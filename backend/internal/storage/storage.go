package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iiif-pdf-server/internal/models"
)

type Storage interface {
	SaveDocument(doc *models.PDFDocument) error
	GetDocument(id string) (*models.PDFDocument, error)
	GetAllDocuments() ([]*models.PDFDocument, error)
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

type FileStorage struct {
	basePath string
}

func NewFileStorage(basePath string) *FileStorage {
	return &FileStorage{
		basePath: basePath,
	}
}

func (fs *FileStorage) SaveDocument(doc *models.PDFDocument) error {
	docPath := filepath.Join(fs.basePath, "documents", doc.ID+".json")

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling document: %w", err)
	}

	return os.WriteFile(docPath, data, 0664)
}

func (fs *FileStorage) GetDocument(id string) (*models.PDFDocument, error) {
	docPath := filepath.Join(fs.basePath, "documents", id+".json")
	data, err := os.ReadFile(docPath)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	var doc models.PDFDocument
	err = json.Unmarshal(data, &doc)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling document: %w", err)
	}

	return &doc, nil
}

func (fs *FileStorage) GetAllDocuments() ([]*models.PDFDocument, error) {
	docsDir := filepath.Join(fs.basePath, "documents")

	files, err := os.ReadDir(docsDir)
	if err != nil {
		return nil, fmt.Errorf("error reading documents directory: %w", err)
	}

	var documents []*models.PDFDocument
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			id := file.Name()[:len(file.Name())-5] // Remove .json extension
			doc, err := fs.GetDocument(id)
			if err != nil {
				continue // Skip invalid documents
			}
			documents = append(documents, doc)
		}
	}

	return documents, nil
}

func (fs *FileStorage) DeleteDocument(id string) error {
	// Delete document metadata
	docPath := filepath.Join(fs.basePath, "documents", id+".json")
	if err := os.Remove(docPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting document metadata: %w", err)
	}

	// Delete associated files
	imagePath := filepath.Join(fs.basePath, "images", id)
	if err := os.RemoveAll(imagePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting images: %w", err)
	}

	thumbnailPath := filepath.Join(fs.basePath, "thumbnails", id+".jpg")
	if err := os.Remove(thumbnailPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting thumbnail: %w", err)
	}

	manifestPath := filepath.Join(fs.basePath, "manifests", id+".json")
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
	imageDir := filepath.Join(fs.basePath, "images", image.DocumentID)
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
		PageNumber: page,
		ImagePath:  imagePath,
		Format:     normalizeFormat(filepath.Ext(imagePath)),
	}, nil
}

func (fs *FileStorage) GetDocumentImages(documentID string) ([]*models.DocumentImage, error) {
	// Lista las paginas convertidas para que el dashboard pueda construir URLs IIIF sin tocar rutas internas.
	imagesDir := filepath.Join(fs.basePath, "images", documentID)
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

	doc, err := fs.GetDocument(documentID)
	if err != nil {
		return nil, err
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
	imagesDir := filepath.Join(fs.basePath, "images")
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
