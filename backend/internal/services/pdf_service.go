package services

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/go-fitz"
	"github.com/google/uuid"
)

type PDFService struct {
	config      *config.Config
	storage     storage.Storage
	onCompleted func(string)
	uploadSlots chan struct{}
}

type stagedDocumentImage struct {
	image     *models.DocumentImage
	path      string
	mediaType string
}

type stagedUploadResult struct {
	item stagedDocumentImage
	err  error
}

func NewPDFService(config *config.Config, storage storage.Storage) *PDFService {
	maxConcurrentUploads := config.Security.MaxConcurrentUploads
	if maxConcurrentUploads < 1 {
		maxConcurrentUploads = 5
	}
	if maxConcurrentUploads > 100 {
		maxConcurrentUploads = 100
	}
	return &PDFService{
		config:      config,
		storage:     storage,
		uploadSlots: make(chan struct{}, maxConcurrentUploads),
	}
}

func (s *PDFService) SetCompletionHook(hook func(string)) { s.onCompleted = hook }

func (s *PDFService) ProcessPDF(sourcePath string, filename string, settings models.ConversionSettings, scope *models.Scope) (*models.PDFDocument, error) {
	if settings.Format == "" {
		settings.Format = s.config.Conversion.DefaultFormat
	}
	if settings.Quality == 0 {
		settings.Quality = s.config.Conversion.DefaultQuality
	}
	if settings.DPI == 0 {
		settings.DPI = s.config.Conversion.DPI
	}
	if settings.DPI == 0 {
		settings.DPI = 150
	}

	documentID := uuid.New().String()
	if scope == nil {
		scope = &models.Scope{ProjectKey: s.config.Projects.DefaultProject}
	}
	pdfPath := ""
	pdfData, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("error reading uploaded PDF: %w", err)
	}

	if !s.usesDatabaseBlobs() {
		pdfPath = filepath.Join(s.scopePath(s.config.Storage.PDFsPath, scope), documentID+filepath.Ext(filename))
		if err := copyFile(sourcePath, pdfPath); err != nil {
			return nil, fmt.Errorf("error storing original PDF: %w", err)
		}
	}

	doc := &models.PDFDocument{
		ID:                documentID,
		Name:              filename,
		ProjectKey:        scope.ProjectKey,
		TenantKey:         scope.TenantKey,
		MigratedFromLocal: false,
		UploadDate:        time.Now(),
		Status:            "processing",
		FilePath:          pdfPath,
		ConvertedPages:    0,
		ConversionWidth:   settings.MaxWidth,
		ConversionHeight:  settings.MaxHeight,
		ConversionDPI:     settings.DPI,
		ConversionFormat:  settings.Format,
		ConversionQuality: settings.Quality,
	}

	if err := s.storage.SaveDocument(doc); err != nil {
		return nil, fmt.Errorf("error saving document: %w", err)
	}
	if s.usesDatabaseBlobs() {
		if err := s.storage.SaveDocumentPDF(doc.ID, pdfData, "application/pdf"); err != nil {
			return nil, fmt.Errorf("error saving PDF blob: %w", err)
		}
		storedDoc, err := s.storage.GetDocument(doc.ID)
		if err != nil {
			return nil, fmt.Errorf("error reloading stored PDF metadata: %w", err)
		}
		// S3 assigns pdf_path while saving the object. Keep it in the worker copy
		// so progress updates do not overwrite the persisted reference.
		doc.FilePath = storedDoc.FilePath
	}

	workerPath := sourcePath
	if s.usesDatabaseBlobs() {
		workerPath, err = createProcessingCopy(s.config.PDF.TempPath, pdfData)
		if err != nil {
			doc.Status = "error"
			_ = s.storage.UpdateDocument(doc)
			return nil, fmt.Errorf("error preparing PDF conversion: %w", err)
		}
	}

	go s.convertPDFToImages(doc, workerPath, settings)

	return doc, nil
}

func (s *PDFService) convertPDFToImages(doc *models.PDFDocument, sourcePath string, settings models.ConversionSettings) {
	startedAt := time.Now()
	var renderDuration, resizeDuration, encodeDuration, stageDuration, uploadDuration time.Duration
	defer func() {
		if s.usesDatabaseBlobs() {
			_ = os.Remove(sourcePath)
		}
		if r := recover(); r != nil {
			doc.Status = "error"
			_ = s.storage.UpdateDocument(doc)
			log.Printf("PDF conversion failed document=%s stage=panic error=%v", doc.ID, r)
		}
	}()

	openPath := doc.FilePath
	if s.usesDatabaseBlobs() {
		openPath = sourcePath
	}

	pdf, err := fitz.New(openPath)
	if err != nil {
		doc.Status = "error"
		_ = s.storage.UpdateDocument(doc)
		log.Printf("PDF conversion failed document=%s stage=open error=%v", doc.ID, err)
		return
	}
	pdfClosed := false
	defer func() {
		if !pdfClosed {
			pdf.Close()
		}
	}()

	doc.TotalPages = pdf.NumPage()
	doc.Outline = extractPDFOutline(pdf, doc.TotalPages)
	_ = s.storage.UpdateDocument(doc)

	bulkUpload := s.bulkUploadEnabled(doc.ProjectKey)
	stagingDir := ""
	if bulkUpload {
		if err := os.MkdirAll(s.config.BinaryStorage.TempPath, 0o750); err != nil {
			doc.Status = "error"
			_ = s.storage.UpdateDocument(doc)
			log.Printf("PDF conversion failed document=%s stage=prepare_bulk_temp error=%v", doc.ID, err)
			return
		}
		stagingDir, err = os.MkdirTemp(s.config.BinaryStorage.TempPath, "bulk-"+doc.ID+"-")
		if err != nil {
			doc.Status = "error"
			_ = s.storage.UpdateDocument(doc)
			log.Printf("PDF conversion failed document=%s stage=create_bulk_temp error=%v", doc.ID, err)
			return
		}
		defer os.RemoveAll(stagingDir)
	}

	imageDir := ""
	if !s.usesDatabaseBlobs() {
		imageDir = filepath.Join(s.scopePath(s.config.Storage.ImagesPath, &models.Scope{ProjectKey: doc.ProjectKey, TenantKey: doc.TenantKey}), doc.ID)
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			doc.Status = "error"
			_ = s.storage.UpdateDocument(doc)
			log.Printf("PDF conversion failed document=%s stage=create_image_directory error=%v", doc.ID, err)
			return
		}
	}

	var imagePaths []string
	stagedImages := make([]stagedDocumentImage, 0, doc.TotalPages)
	failedPages := 0

	for i := 0; i < pdf.NumPage(); i++ {
		pageNumber := i + 1
		stageStarted := time.Now()
		img, err := pdf.ImageDPI(i, float64(settings.DPI))
		renderDuration += time.Since(stageStarted)
		if err != nil {
			failedPages++
			log.Printf("PDF page failed document=%s page=%d stage=render error=%v", doc.ID, pageNumber, err)
			continue
		}
		var pageImage image.Image = img

		if settings.MaxWidth > 0 || settings.MaxHeight > 0 {
			stageStarted = time.Now()
			pageImage = imaging.Fit(pageImage, settings.MaxWidth, settings.MaxHeight, imaging.Lanczos)
			resizeDuration += time.Since(stageStarted)
		}

		stageStarted = time.Now()
		imageData, mediaType, err := s.encodeImage(pageImage, settings)
		encodeDuration += time.Since(stageStarted)
		if err != nil {
			failedPages++
			log.Printf("PDF page failed document=%s page=%d stage=encode error=%v", doc.ID, pageNumber, err)
			continue
		}

		imagePath := ""
		if !s.usesDatabaseBlobs() {
			stageStarted = time.Now()
			imagePath = filepath.Join(imageDir, fmt.Sprintf("page_%d.%s", pageNumber, settings.Format))
			if err := os.WriteFile(imagePath, imageData, 0664); err != nil {
				failedPages++
				log.Printf("PDF page failed document=%s page=%d stage=write_local error=%v", doc.ID, pageNumber, err)
				continue
			}
			stageDuration += time.Since(stageStarted)
		}

		bounds := pageImage.Bounds()
		image := &models.DocumentImage{
			ID:                uuid.New().String(),
			DocumentID:        doc.ID,
			ProjectKey:        doc.ProjectKey,
			TenantKey:         doc.TenantKey,
			MigratedFromLocal: false,
			PageNumber:        pageNumber,
			ImagePath:         imagePath,
			Width:             bounds.Dx(),
			Height:            bounds.Dy(),
			Format:            settings.Format,
			MediaType:         mediaType,
			ByteSize:          int64(len(imageData)),
			CreatedAt:         time.Now(),
		}

		if bulkUpload {
			stageStarted = time.Now()
			stagedPath := filepath.Join(stagingDir, fmt.Sprintf("page_%06d.%s", pageNumber, settings.Format))
			if err := os.WriteFile(stagedPath, imageData, 0o600); err != nil {
				failedPages++
				log.Printf("PDF page failed document=%s page=%d stage=stage_bulk error=%v", doc.ID, pageNumber, err)
				continue
			}
			stageDuration += time.Since(stageStarted)
			stagedImages = append(stagedImages, stagedDocumentImage{image: image, path: stagedPath, mediaType: mediaType})
			continue
		}

		stageStarted = time.Now()
		if err := s.persistDocumentImage(image, imageData, mediaType); err != nil {
			failedPages++
			log.Printf("PDF page failed document=%s page=%d stage=store error=%v", doc.ID, pageNumber, err)
			continue
		}
		uploadDuration += time.Since(stageStarted)

		if imagePath != "" {
			imagePaths = append(imagePaths, imagePath)
		}
		doc.ConvertedPages++
		if doc.ConvertedPages%5 == 0 || doc.ConvertedPages == doc.TotalPages {
			s.storage.UpdateDocument(doc)
		}

		if i == 0 && !s.usesDatabaseBlobs() {
			thumbnailPath := filepath.Join(s.scopePath(s.config.Storage.ThumbnailsPath, &models.Scope{ProjectKey: doc.ProjectKey, TenantKey: doc.TenantKey}), doc.ID+".jpg")
			thumbnail := imaging.Resize(pageImage, 200, 0, imaging.Lanczos)
			if err := s.saveImage(thumbnail, thumbnailPath, models.ConversionSettings{Format: "jpg", Quality: 80}); err == nil {
				doc.ThumbnailURL = fmt.Sprintf("%s/static/thumbnails/%s.jpg", s.config.IIIF.BaseURL, doc.ID)
				s.storage.UpdateDocument(doc)
			}
		}
	}

	if bulkUpload {
		pdf.Close()
		pdfClosed = true
		uploadStarted := time.Now()
		uploadErrors := s.uploadStagedImages(stagedImages, func(item stagedDocumentImage) {
			doc.ConvertedPages++
			if doc.ConvertedPages%5 == 0 || doc.ConvertedPages == doc.TotalPages {
				_ = s.storage.UpdateDocument(doc)
			}
			_ = os.Remove(item.path)
		})
		uploadDuration += time.Since(uploadStarted)
		failedPages += len(uploadErrors)
		for _, uploadErr := range uploadErrors {
			log.Printf("PDF page failed document=%s stage=bulk_upload error=%v", doc.ID, uploadErr)
		}
	}

	doc.ImagePaths = imagePaths
	if failedPages == 0 && doc.ConvertedPages == doc.TotalPages {
		doc.Status = "completed"
		doc.ManifestURL = fmt.Sprintf("%s/api/iiif/%s/manifest", s.config.IIIF.BaseURL, doc.ID)
	} else {
		doc.Status = "error"
	}

	_ = s.storage.UpdateDocument(doc)
	if doc.Status == "completed" && s.onCompleted != nil {
		s.onCompleted(doc.ID)
	}
	log.Printf("PDF conversion finished document=%s status=%s pages=%d/%d failed=%d bulk_upload=%t total=%s render=%s resize=%s encode=%s stage=%s upload=%s dpi=%d max=%dx%d format=%s",
		doc.ID, doc.Status, doc.ConvertedPages, doc.TotalPages, failedPages, bulkUpload, time.Since(startedAt), renderDuration, resizeDuration, encodeDuration, stageDuration, uploadDuration,
		settings.DPI, settings.MaxWidth, settings.MaxHeight, settings.Format)
}

func (s *PDFService) bulkUploadEnabled(projectKey string) bool {
	if !s.usesS3() {
		return false
	}
	project, ok := s.config.ProjectByKey(projectKey)
	return ok && project.BulkUpload
}

func (s *PDFService) usesS3() bool {
	return strings.EqualFold(s.config.FilesystemDisk, "s3") || strings.EqualFold(s.config.BinaryStorage.Mode, "s3")
}

func (s *PDFService) persistDocumentImage(image *models.DocumentImage, data []byte, mediaType string) error {
	if writer, ok := s.storage.(storage.DocumentImageAssetWriter); ok {
		s.uploadSlots <- struct{}{}
		defer func() { <-s.uploadSlots }()
		return writer.SaveDocumentImageAsset(image, data, mediaType)
	}
	if err := s.storage.SaveDocumentImage(image); err != nil {
		return err
	}
	if s.usesDatabaseBlobs() {
		return s.storage.SaveDocumentImageData(image.ID, data, mediaType)
	}
	return nil
}

func (s *PDFService) uploadStagedImages(items []stagedDocumentImage, onSuccess func(stagedDocumentImage)) []error {
	if len(items) == 0 {
		return nil
	}
	workerCount := cap(s.uploadSlots)
	if workerCount > len(items) {
		workerCount = len(items)
	}
	jobs := make(chan stagedDocumentImage)
	results := make(chan stagedUploadResult, len(items))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for item := range jobs {
				data, err := os.ReadFile(item.path)
				if err == nil {
					err = s.persistDocumentImage(item.image, data, item.mediaType)
				}
				results <- stagedUploadResult{item: item, err: err}
			}
		}()
	}
	go func() {
		for _, item := range items {
			jobs <- item
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	errors := make([]error, 0)
	for result := range results {
		if result.err != nil {
			errors = append(errors, fmt.Errorf("page=%d: %w", result.item.image.PageNumber, result.err))
			continue
		}
		onSuccess(result.item)
	}
	return errors
}

func extractPDFOutline(pdf *fitz.Document, totalPages int) []models.PDFOutlineItem {
	outline, err := pdf.ToC()
	if err != nil {
		return nil
	}
	return normalizePDFOutline(outline, totalPages)
}

func normalizePDFOutline(source []fitz.Outline, totalPages int) []models.PDFOutlineItem {
	result := make([]models.PDFOutlineItem, 0, len(source))
	for _, item := range source {
		title := strings.TrimSpace(item.Title)
		page := item.Page + 1 // go-fitz exposes MuPDF's zero-based page index.
		if title == "" || page < 1 || page > totalPages {
			continue
		}
		level := item.Level
		if level < 1 {
			level = 1
		}
		result = append(result, models.PDFOutlineItem{
			Level:      level,
			Title:      title,
			PageNumber: page,
		})
	}
	return result
}

func (s *PDFService) scopePath(base string, scope *models.Scope) string {
	if !s.config.Projects.Enabled || scope == nil || strings.TrimSpace(scope.ProjectKey) == "" {
		return base
	}
	root := s.config.Storage.DataPath
	rel, err := filepath.Rel(root, base)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(base)
	}
	if strings.TrimSpace(scope.TenantKey) != "" {
		return filepath.Join(root, "projects", scope.ProjectKey, "tenants", scope.TenantKey, rel)
	}
	return filepath.Join(root, "projects", scope.ProjectKey, rel)
}

func (s *PDFService) encodeImage(img image.Image, settings models.ConversionSettings) ([]byte, string, error) {
	var buffer bytes.Buffer
	switch settings.Format {
	case "png":
		if err := png.Encode(&buffer, img); err != nil {
			return nil, "", err
		}
		return buffer.Bytes(), "image/png", nil
	default:
		if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: settings.Quality}); err != nil {
			return nil, "", err
		}
		return buffer.Bytes(), "image/jpeg", nil
	}
}

func (s *PDFService) saveImage(img image.Image, path string, settings models.ConversionSettings) error {
	data, _, err := s.encodeImage(img, settings)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0664)
}

func (s *PDFService) usesDatabaseBlobs() bool {
	backend := strings.ToLower(s.config.Storage.Backend)
	mode := strings.ToLower(s.config.BinaryStorage.Mode)
	return mode == "s3" || (backend != "local" && mode == "database")
}

func copyFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func createProcessingCopy(tempPath string, data []byte) (string, error) {
	if err := os.MkdirAll(tempPath, 0o750); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(tempPath, "processing-*.pdf")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
