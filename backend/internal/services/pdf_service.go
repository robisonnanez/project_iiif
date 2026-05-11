package services

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/go-fitz"
	"github.com/google/uuid"
)

type PDFService struct {
	config  *config.Config
	storage storage.Storage
}

func NewPDFService(config *config.Config, storage storage.Storage) *PDFService {
	return &PDFService{
		config:  config,
		storage: storage,
	}
}

func (s *PDFService) ProcessPDF(sourcePath string, filename string, settings models.ConversionSettings, scope *models.Scope) (*models.PDFDocument, error) {
	if settings.Format == "" {
		settings.Format = s.config.Conversion.DefaultFormat
	}
	if settings.Quality == 0 {
		settings.Quality = s.config.Conversion.DefaultQuality
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
		ID:             documentID,
		Name:           filename,
		ProjectKey:     scope.ProjectKey,
		TenantKey:      scope.TenantKey,
		UploadDate:     time.Now(),
		Status:         "processing",
		FilePath:       pdfPath,
		ConvertedPages: 0,
	}

	if err := s.storage.SaveDocument(doc); err != nil {
		return nil, fmt.Errorf("error saving document: %w", err)
	}
	if s.usesDatabaseBlobs() {
		if err := s.storage.SaveDocumentPDF(doc.ID, pdfData, "application/pdf"); err != nil {
			return nil, fmt.Errorf("error saving PDF blob: %w", err)
		}
	}

	go s.convertPDFToImages(doc, sourcePath, settings)

	return doc, nil
}

func (s *PDFService) convertPDFToImages(doc *models.PDFDocument, sourcePath string, settings models.ConversionSettings) {
	defer func() {
		if s.usesDatabaseBlobs() {
			_ = os.Remove(sourcePath)
		}
		if r := recover(); r != nil {
			doc.Status = "error"
			s.storage.UpdateDocument(doc)
		}
	}()

	openPath := doc.FilePath
	if s.usesDatabaseBlobs() {
		openPath = sourcePath
	}

	pdf, err := fitz.New(openPath)
	if err != nil {
		doc.Status = "error"
		s.storage.UpdateDocument(doc)
		return
	}
	defer pdf.Close()

	doc.TotalPages = pdf.NumPage()
	s.storage.UpdateDocument(doc)

	imageDir := ""
	if !s.usesDatabaseBlobs() {
		imageDir = filepath.Join(s.scopePath(s.config.Storage.ImagesPath, &models.Scope{ProjectKey: doc.ProjectKey, TenantKey: doc.TenantKey}), doc.ID)
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			doc.Status = "error"
			s.storage.UpdateDocument(doc)
			return
		}
	}

	var imagePaths []string

	for i := 0; i < pdf.NumPage(); i++ {
		img, err := pdf.Image(i)
		if err != nil {
			continue
		}
		var pageImage image.Image = img

		if settings.MaxWidth > 0 || settings.MaxHeight > 0 {
			pageImage = imaging.Fit(pageImage, settings.MaxWidth, settings.MaxHeight, imaging.Lanczos)
		}

		imageData, mediaType, err := s.encodeImage(pageImage, settings)
		if err != nil {
			continue
		}

		imagePath := ""
		if !s.usesDatabaseBlobs() {
			imagePath = filepath.Join(imageDir, fmt.Sprintf("page_%d.%s", i+1, settings.Format))
			if err := os.WriteFile(imagePath, imageData, 0664); err != nil {
				continue
			}
		}

		bounds := pageImage.Bounds()
		image := &models.DocumentImage{
			ID:         uuid.New().String(),
			DocumentID: doc.ID,
			ProjectKey: doc.ProjectKey,
			TenantKey:  doc.TenantKey,
			PageNumber: i + 1,
			ImagePath:  imagePath,
			Width:      bounds.Dx(),
			Height:     bounds.Dy(),
			Format:     settings.Format,
			MediaType:  mediaType,
			ByteSize:   int64(len(imageData)),
			CreatedAt:  time.Now(),
		}
		if err := s.storage.SaveDocumentImage(image); err != nil {
			continue
		}
		if s.usesDatabaseBlobs() {
			if err := s.storage.SaveDocumentImageData(image.ID, imageData, mediaType); err != nil {
				continue
			}
		}

		if imagePath != "" {
			imagePaths = append(imagePaths, imagePath)
		}
		doc.ConvertedPages++
		s.storage.UpdateDocument(doc)

		if i == 0 && !s.usesDatabaseBlobs() {
			thumbnailPath := filepath.Join(s.scopePath(s.config.Storage.ThumbnailsPath, &models.Scope{ProjectKey: doc.ProjectKey, TenantKey: doc.TenantKey}), doc.ID+".jpg")
			thumbnail := imaging.Resize(pageImage, 200, 0, imaging.Lanczos)
			if err := s.saveImage(thumbnail, thumbnailPath, models.ConversionSettings{Format: "jpg", Quality: 80}); err == nil {
				doc.ThumbnailURL = fmt.Sprintf("%s/static/thumbnails/%s.jpg", s.config.IIIF.BaseURL, doc.ID)
				s.storage.UpdateDocument(doc)
			}
		}
	}

	doc.ImagePaths = imagePaths
	doc.Status = "completed"
	doc.ManifestURL = fmt.Sprintf("%s/api/iiif/%s/manifest", s.config.IIIF.BaseURL, doc.ID)

	s.storage.UpdateDocument(doc)
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
	return backend != "local" && mode == "database"
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
