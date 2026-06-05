package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
)

type Storage interface {
	SaveDocument(doc *models.PDFDocument) error
	GetDocument(id string) (*models.PDFDocument, error)
	GetAllDocuments() ([]*models.PDFDocument, error)
	GetDocumentsByScope(projectKey, tenantKey string) ([]*models.PDFDocument, error)
	DeleteDocument(id string) error
	UpdateDocument(doc *models.PDFDocument) error
	SaveDocumentPDF(documentID string, data []byte, mediaType string) error
	SaveDocumentImage(image *models.DocumentImage) error
	SaveDocumentImageData(imageID string, data []byte, mediaType string) error
	GetDocumentImage(id string) (*models.DocumentImage, error)
	GetDocumentImageByPage(documentID string, page int) (*models.DocumentImage, error)
	GetDocumentImages(documentID string) ([]*models.DocumentImage, error)
	GetDocumentImageData(id string) (*models.BinaryAsset, error)
}

type DocumentPDFBlobChecker interface {
	HasDocumentPDFBlob(documentID string) (bool, error)
}

type ImageBlobChecker interface {
	HasImageBlob(imageID string) (bool, error)
}

type FileStorage struct {
	basePath        string
	projectsEnabled bool
}

func NewFileStorage(basePath string) *FileStorage {
	return &FileStorage{
		basePath: basePath,
	}
}

func NewFileStorageFromConfig(cfg *config.Config) *FileStorage {
	return &FileStorage{
		basePath:        cfg.Storage.DataPath,
		projectsEnabled: cfg.Projects.Enabled,
	}
}

func (fs *FileStorage) SaveDocument(doc *models.PDFDocument) error {
	docPath := filepath.Join(fs.scopeBase(doc.ProjectKey, doc.TenantKey), "documents", doc.ID+".json")
	if err := os.MkdirAll(filepath.Dir(docPath), 0755); err != nil {
		return fmt.Errorf("error creating document directory: %w", err)
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling document: %w", err)
	}

	return os.WriteFile(docPath, data, 0664)
}

func (fs *FileStorage) GetDocument(id string) (*models.PDFDocument, error) {
	docPath, err := fs.findDocumentPath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(docPath)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	var doc models.PDFDocument
	err = json.Unmarshal(data, &doc)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling document: %w", err)
	}
	if doc.ProjectKey == "" {
		doc.ProjectKey = "default"
	}

	return &doc, nil
}

func (fs *FileStorage) GetAllDocuments() ([]*models.PDFDocument, error) {
	return fs.GetDocumentsByScope("", "")
}

func (fs *FileStorage) GetDocumentsByScope(projectKey, tenantKey string) ([]*models.PDFDocument, error) {
	searchRoots := fs.documentSearchRoots(projectKey, tenantKey)

	var documents []*models.PDFDocument
	seen := map[string]struct{}{}
	for _, docsDir := range searchRoots {
		files, err := os.ReadDir(docsDir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if filepath.Ext(file.Name()) == ".json" {
				id := file.Name()[:len(file.Name())-5]
				doc, err := fs.GetDocument(id)
				if err != nil {
					continue
				}
				if projectKey != "" && doc.ProjectKey != projectKey {
					continue
				}
				if tenantKey != "" && doc.TenantKey != tenantKey {
					continue
				}
				documents = append(documents, doc)
				seen[doc.ID] = struct{}{}
			}
		}
	}

	// Fallback para layout filesystem (pdfs/images) cuando no existe metadata JSON.
	fallback, _ := fs.discoverDocumentsFromFilesystem(projectKey, tenantKey)
	for _, doc := range fallback {
		if _, ok := seen[doc.ID]; ok {
			continue
		}
		documents = append(documents, doc)
	}

	sort.Slice(documents, func(i, j int) bool {
		return documents[i].UploadDate.After(documents[j].UploadDate)
	})
	return documents, nil
}

func (fs *FileStorage) DeleteDocument(id string) error {
	doc, _ := fs.GetDocument(id)
	basePath := fs.basePath
	if doc != nil {
		basePath = fs.scopeBase(doc.ProjectKey, doc.TenantKey)
	}
	// Delete document metadata
	docPath := filepath.Join(basePath, "documents", id+".json")
	if err := os.Remove(docPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting document metadata: %w", err)
	}

	// Delete associated files
	imagePath := filepath.Join(basePath, "images", id)
	if err := os.RemoveAll(imagePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting images: %w", err)
	}

	thumbnailPath := filepath.Join(basePath, "thumbnails", id+".jpg")
	if err := os.Remove(thumbnailPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting thumbnail: %w", err)
	}

	manifestPath := filepath.Join(basePath, "manifests", id+".json")
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting manifest: %w", err)
	}

	return nil
}

func (fs *FileStorage) UpdateDocument(doc *models.PDFDocument) error {
	return fs.SaveDocument(doc)
}

func (fs *FileStorage) SaveDocumentPDF(documentID string, data []byte, mediaType string) error {
	return nil
}

func (fs *FileStorage) SaveDocumentImage(image *models.DocumentImage) error {
	imageDir := filepath.Join(fs.scopeBase(image.ProjectKey, image.TenantKey), "images", image.DocumentID)
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		return fmt.Errorf("error creating image metadata directory: %w", err)
	}

	imagePath := filepath.Join(imageDir, image.ID+".json")
	data, err := json.MarshalIndent(image, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling image metadata: %w", err)
	}

	return os.WriteFile(imagePath, data, 0664)
}

func (fs *FileStorage) SaveDocumentImageData(imageID string, data []byte, mediaType string) error {
	return nil
}

func (fs *FileStorage) GetDocumentImage(id string) (*models.DocumentImage, error) {
	imagePath, err := fs.findImageMetadata(id)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("image metadata not found: %w", err)
	}

	var image models.DocumentImage
	if err := json.Unmarshal(data, &image); err != nil {
		return nil, fmt.Errorf("error unmarshaling image metadata: %w", err)
	}

	return &image, nil
}

func (fs *FileStorage) GetDocumentImageByPage(documentID string, page int) (*models.DocumentImage, error) {
	doc, err := fs.GetDocument(documentID)
	if err != nil {
		return nil, err
	}

	if page < 1 || page > len(doc.ImagePaths) {
		return nil, fmt.Errorf("image page not found")
	}

	imagePath := doc.ImagePaths[page-1]
	return &models.DocumentImage{
		ID:         fmt.Sprintf("%s_page_%d", documentID, page),
		DocumentID: documentID,
		ProjectKey: doc.ProjectKey,
		TenantKey:  doc.TenantKey,
		PageNumber: page,
		ImagePath:  imagePath,
		Format:     normalizeFormat(filepath.Ext(imagePath)),
	}, nil
}

func (fs *FileStorage) GetDocumentImages(documentID string) ([]*models.DocumentImage, error) {
	// Lista las paginas convertidas para que el dashboard pueda construir URLs IIIF sin tocar rutas internas.
	doc, docErr := fs.GetDocument(documentID)
	imagesDir := filepath.Join(fs.basePath, "images", documentID)
	if docErr == nil {
		imagesDir = filepath.Join(fs.scopeBase(doc.ProjectKey, doc.TenantKey), "images", documentID)
	}
	files, err := os.ReadDir(imagesDir)
	if err == nil {
		var images []*models.DocumentImage
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
				continue
			}
			id := strings.TrimSuffix(file.Name(), ".json")
			image, err := fs.GetDocumentImage(id)
			if err == nil {
				images = append(images, image)
			}
		}
		return images, nil
	}

	if docErr != nil {
		return nil, docErr
	}

	images := make([]*models.DocumentImage, 0, len(doc.ImagePaths))
	for index := range doc.ImagePaths {
		image, err := fs.GetDocumentImageByPage(documentID, index+1)
		if err == nil {
			images = append(images, image)
		}
	}
	return images, nil
}

func (fs *FileStorage) GetDocumentImageData(id string) (*models.BinaryAsset, error) {
	image, err := fs.GetDocumentImage(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(image.ImagePath)
	if err != nil {
		return nil, err
	}
	return &models.BinaryAsset{
		ID:        id,
		Data:      data,
		MediaType: mediaTypeForFormat(image.Format),
		ByteSize:  int64(len(data)),
	}, nil
}

func (fs *FileStorage) findImageMetadata(id string) (string, error) {
	imagesDir := fs.basePath
	var found string
	err := filepath.WalkDir(imagesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == id+".json" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error reading image metadata: %w", err)
	}
	if found == "" {
		return "", fmt.Errorf("image metadata not found")
	}
	return found, nil
}

func (fs *FileStorage) findDocumentPath(id string) (string, error) {
	candidates := []string{filepath.Join(fs.basePath, "documents", id+".json")}
	// Compatibilidad con layout por tenant: /documents/{tenant}/{id}.json
	_ = filepath.WalkDir(filepath.Join(fs.basePath, "documents"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if d.Name() == id+".json" {
			candidates = append(candidates, path)
			return filepath.SkipAll
		}
		return nil
	})
	if fs.projectsEnabled {
		projectsRoot := filepath.Join(fs.basePath, "projects")
		_ = filepath.WalkDir(projectsRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != id+".json" {
				return nil
			}
			candidates = append(candidates, path)
			return filepath.SkipAll
		})
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("document not found")
}

func (fs *FileStorage) documentSearchRoots(projectKey, tenantKey string) []string {
	if !fs.projectsEnabled {
		roots := []string{filepath.Join(fs.basePath, "documents")}
		// Compatibilidad con layout por tenant: /documents/{tenant}
		entries, err := os.ReadDir(filepath.Join(fs.basePath, "documents"))
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				roots = append(roots, filepath.Join(fs.basePath, "documents", entry.Name()))
			}
		}
		return roots
	}
	if projectKey != "" {
		return []string{filepath.Join(fs.scopeBase(projectKey, tenantKey), "documents")}
	}
	roots := []string{filepath.Join(fs.basePath, "documents")}
	projectsRoot := filepath.Join(fs.basePath, "projects")
	_ = filepath.WalkDir(projectsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || filepath.Base(path) != "documents" {
			return nil
		}
		roots = append(roots, path)
		return filepath.SkipDir
	})
	return roots
}

func (fs *FileStorage) scopeBase(projectKey, tenantKey string) string {
	if !fs.projectsEnabled || strings.TrimSpace(projectKey) == "" {
		return fs.basePath
	}
	if strings.TrimSpace(tenantKey) != "" {
		return filepath.Join(fs.basePath, "projects", projectKey, "tenants", tenantKey)
	}
	return filepath.Join(fs.basePath, "projects", projectKey)
}

func (fs *FileStorage) discoverDocumentsFromFilesystem(projectKey, tenantKey string) ([]*models.PDFDocument, error) {
	pdfsRoot := filepath.Join(fs.basePath, "pdfs")
	out := []*models.PDFDocument{}
	err := filepath.WalkDir(pdfsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".pdf") {
			return nil
		}

		id := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		tenant := inferTenantFromPDFPath(path, fs.basePath)
		project := "default"
		if strings.TrimSpace(projectKey) != "" && project != projectKey {
			return nil
		}
		if strings.TrimSpace(tenantKey) != "" && tenant != tenantKey {
			return nil
		}

		modTime := time.Now()
		if info, statErr := os.Stat(path); statErr == nil {
			modTime = info.ModTime()
		}
		images := fs.findImagesForLocalDocument(id, tenant)
		status := "uploaded"
		if len(images) > 0 {
			status = "completed"
		}
		out = append(out, &models.PDFDocument{
			ID:             id,
			Name:           d.Name(),
			FilePath:       path,
			Status:         status,
			TotalPages:     len(images),
			ConvertedPages: len(images),
			ImagePaths:     images,
			UploadDate:     modTime,
			ProjectKey:     project,
			TenantKey:      tenant,
		})
		return nil
	})
	return out, err
}

func (fs *FileStorage) findImagesForLocalDocument(documentID, tenant string) []string {
	candidates := []string{
		filepath.Join(fs.basePath, "images", documentID),
	}
	if strings.TrimSpace(tenant) != "" {
		candidates = append(candidates, filepath.Join(fs.basePath, "images", tenant, documentID))
	}

	paths := []string{}
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
				continue
			}
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}

func inferTenantFromPDFPath(pdfPath, basePath string) string {
	normalized := filepath.ToSlash(pdfPath)
	root := filepath.ToSlash(filepath.Join(basePath, "pdfs")) + "/"
	if !strings.HasPrefix(normalized, root) {
		return ""
	}
	relative := strings.TrimPrefix(normalized, root)
	parts := strings.Split(relative, "/")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

func mediaTypeForFormat(format string) string {
	switch normalizeFormat(format) {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func normalizeFormat(format string) string {
	if len(format) > 0 && format[0] == '.' {
		format = format[1:]
	}
	if format == "jpeg" {
		return "jpg"
	}
	return format
}
