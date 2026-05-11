package services

import (
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"
)

type DocumentService struct {
	storage storage.Storage
}

func NewDocumentService(storage storage.Storage) *DocumentService {
	return &DocumentService{
		storage: storage,
	}
}

func (s *DocumentService) GetAllDocuments() ([]*models.PDFDocument, error) {
	return s.storage.GetAllDocuments()
}

func (s *DocumentService) GetDocumentsByScope(projectKey, tenantKey string) ([]*models.PDFDocument, error) {
	return s.storage.GetDocumentsByScope(projectKey, tenantKey)
}

func (s *DocumentService) GetDocument(id string) (*models.PDFDocument, error) {
	return s.storage.GetDocument(id)
}

func (s *DocumentService) GetDocumentImages(id string) ([]*models.DocumentImage, error) {
	return s.storage.GetDocumentImages(id)
}

func (s *DocumentService) DeleteDocument(id string) error {
	return s.storage.DeleteDocument(id)
}
