package main

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"iiif-pdf-server/internal/models"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestIsMissingImageMetadata(t *testing.T) {
	for name, err := range map[string]error{
		"mysql": fmt.Errorf("image not found: %w", sql.ErrNoRows),
		"mongo": fmt.Errorf("image not found: %w", mongo.ErrNoDocuments),
	} {
		t.Run(name, func(t *testing.T) {
			if !isMissingImageMetadata(err) {
				t.Fatalf("expected %v to be recognized as missing metadata", err)
			}
		})
	}
}

func TestMigrateDocumentDoesNotCompletePartialMigration(t *testing.T) {
	store := newMigrationTestStorage()
	item := sourceDocument{
		Doc:          &models.PDFDocument{ID: "doc", Name: "doc.pdf", Status: "completed"},
		FromDatabase: true,
	}
	err := migrateDocument(item, store, &stats{})
	if err == nil {
		t.Fatal("expected incomplete migration to fail")
	}
	if stored := store.documents["doc"]; stored == nil || stored.Status != "error" {
		t.Fatalf("stored document=%#v, want status error", stored)
	}
}

func TestMigrateDocumentCompletesAndKeepsS3Reference(t *testing.T) {
	store := newMigrationTestStorage()
	image := &models.DocumentImage{ID: "image-1", DocumentID: "doc", PageNumber: 1, Width: 10, Height: 10, Format: "jpg", MediaType: "image/jpeg"}
	item := sourceDocument{
		Doc:          &models.PDFDocument{ID: "doc", Name: "doc.pdf", Status: "completed", ProjectKey: "default"},
		PDFBytes:     []byte("pdf"),
		Images:       []*models.DocumentImage{image},
		ImageData:    map[string][]byte{"image-1": {1, 2, 3}},
		FromDatabase: true,
	}
	var result stats
	if err := migrateDocument(item, store, &result); err != nil {
		t.Fatal(err)
	}
	stored := store.documents["doc"]
	if stored.Status != "completed" || stored.FilePath != "s3://bucket/doc.pdf" || stored.ConvertedPages != 1 {
		t.Fatalf("stored document=%#v", stored)
	}
	if result.MigratedDocs != 1 || result.MigratedPDFs != 1 || result.MigratedImgs != 1 {
		t.Fatalf("stats=%+v", result)
	}
}

type migrationTestStorage struct {
	documents map[string]*models.PDFDocument
	images    map[string]*models.DocumentImage
}

func newMigrationTestStorage() *migrationTestStorage {
	return &migrationTestStorage{documents: map[string]*models.PDFDocument{}, images: map[string]*models.DocumentImage{}}
}

func (s *migrationTestStorage) SaveDocument(doc *models.PDFDocument) error {
	return s.UpdateDocument(doc)
}
func (s *migrationTestStorage) UpdateDocument(doc *models.PDFDocument) error {
	copy := *doc
	s.documents[doc.ID] = &copy
	return nil
}
func (s *migrationTestStorage) GetDocument(id string) (*models.PDFDocument, error) {
	doc, ok := s.documents[id]
	if !ok {
		return nil, errors.New("not found")
	}
	copy := *doc
	return &copy, nil
}
func (s *migrationTestStorage) GetAllDocuments() ([]*models.PDFDocument, error) { return nil, nil }
func (s *migrationTestStorage) GetDocumentsByScope(string, string) ([]*models.PDFDocument, error) {
	return nil, nil
}
func (s *migrationTestStorage) DeleteDocument(string) error { return nil }
func (s *migrationTestStorage) SaveDocumentPDF(id string, _ []byte, _ string) error {
	doc := s.documents[id]
	doc.FilePath = "s3://bucket/doc.pdf"
	return nil
}
func (s *migrationTestStorage) SaveDocumentImage(image *models.DocumentImage) error {
	copy := *image
	s.images[image.ID] = &copy
	return nil
}
func (s *migrationTestStorage) SaveDocumentImageData(id string, _ []byte, _ string) error {
	image, ok := s.images[id]
	if !ok {
		return errors.New("image metadata not found")
	}
	image.ImagePath = "s3://bucket/image.jpg"
	return nil
}
func (s *migrationTestStorage) GetDocumentImage(id string) (*models.DocumentImage, error) {
	image, ok := s.images[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return image, nil
}
func (s *migrationTestStorage) GetDocumentImageByPage(string, int) (*models.DocumentImage, error) {
	return nil, errors.New("not found")
}
func (s *migrationTestStorage) GetDocumentImages(string) ([]*models.DocumentImage, error) {
	return nil, nil
}
func (s *migrationTestStorage) GetDocumentImageData(string) (*models.BinaryAsset, error) {
	return nil, errors.New("not found")
}

func TestIsMissingImageMetadataRejectsOtherErrors(t *testing.T) {
	if isMissingImageMetadata(fmt.Errorf("connection refused")) {
		t.Fatal("unexpectedly classified a connection error as missing metadata")
	}
}
