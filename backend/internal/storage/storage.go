package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"iiif-pdf-server/internal/models"
)

type Storage interface {
	SaveDocument(doc *models.PDFDocument) error
	GetDocument(id string) (*models.PDFDocument, error)
	GetAllDocuments() ([]*models.PDFDocument, error)
	DeleteDocument(id string) error
	UpdateDocument(doc *models.PDFDocument) error
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

	manifiestPath := filepath.Join(fs.basePath, "manifiest", id+".json")
	if err := os.Remove(manifiestPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting manifiest: %w", err)
	}

	return nil
}

func (fs *FileStorage) UpdateDocument(doc *models.PDFDocument) error {
	return fs.SaveDocument(doc)
}
