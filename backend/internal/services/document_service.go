package services

import (
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"
)

type DocumentService struct {
	storage storage.Storage
}

// NewDocumentService crea e inicializa la dependencia con una configuración válida.
func NewDocumentService(storage storage.Storage) *DocumentService {
	return &DocumentService{
		storage: storage,
	}
}

// GetAllDocuments obtiene la información solicitada sin modificar el estado persistido.
func (s *DocumentService) GetAllDocuments() ([]*models.PDFDocument, error) {
	return s.storage.GetAllDocuments()
}

// GetDocumentsByScope obtiene la información solicitada sin modificar el estado persistido.
func (s *DocumentService) GetDocumentsByScope(projectKey, tenantKey string) ([]*models.PDFDocument, error) {
	return s.storage.GetDocumentsByScope(projectKey, tenantKey)
}

// GetDocument obtiene la información solicitada sin modificar el estado persistido.
func (s *DocumentService) GetDocument(id string) (*models.PDFDocument, error) {
	return s.storage.GetDocument(id)
}

// GetDocumentImages obtiene la información solicitada sin modificar el estado persistido.
func (s *DocumentService) GetDocumentImages(id string) ([]*models.DocumentImage, error) {
	return s.storage.GetDocumentImages(id)
}

// DeleteDocument elimina o libera de forma controlada los recursos asociados.
func (s *DocumentService) DeleteDocument(id string) error {
	return s.storage.DeleteDocument(id)
}
