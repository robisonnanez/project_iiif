package storage

import (
	"context"
	"os"
	"testing"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
)

// TestAnnotationMySQLMigration is opt-in because it creates schema objects and
// rows. CI or a developer enables it with ANNOTATION_DB_CONFIG pointing to an
// isolated MySQL configuration.
func TestAnnotationMySQLMigration(t *testing.T) {
	configPath := os.Getenv("ANNOTATION_DB_CONFIG")
	if configPath == "" {
		t.Skip("ANNOTATION_DB_CONFIG no configurado")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Backend != "mysql" {
		t.Fatalf("backend=%s, se esperaba mysql", cfg.Storage.Backend)
	}
	first, err := RunDBMigrations(cfg, filepathFromStorageTests())
	if err != nil {
		t.Fatalf("forward migration: %v (%#v)", err, first)
	}
	second, err := RunDBMigrations(cfg, filepathFromStorageTests())
	if err != nil || second.Applied != 0 {
		t.Fatalf("second migration applied=%d err=%v", second.Applied, err)
	}

	db, err := openMigrationDB(cfg, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, index := range []string{"PRIMARY", "uq_document_annotations_tenant_client", "idx_document_annotations_scope_page", "idx_document_annotations_generation", "idx_document_annotations_deleted_at"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='document_annotations' AND index_name=?`, index).Scan(&count); err != nil || count == 0 {
			t.Fatalf("index %s count=%d err=%v", index, count, err)
		}
	}

	documentID, pageID := "00000000-0000-4000-8000-000000000101", "stable-page-integration-1"
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM document_annotations WHERE document_id=?`, documentID)
		_, _ = db.ExecContext(ctx, `DELETE FROM document_images WHERE id=?`, pageID)
		_, _ = db.ExecContext(ctx, `DELETE FROM documents WHERE id=?`, documentID)
	}()
	if _, err := db.ExecContext(ctx, `INSERT INTO documents (id, original_name, project_key, tenant_key, status, total_pages, converted_pages, pdf_path) VALUES (?,?,?,?,?,?,?,?)`, documentID, "test.pdf", "project-a", "tenant-a", "completed", 1, 1, "/tmp/test.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO document_images (id, document_id, project_key, tenant_key, page_number, image_path, width, height, format) VALUES (?,?,?,?,?,?,?,?,?)`, pageID, documentID, "project-a", "tenant-a", 1, "/tmp/test.jpg", 1241, 1754, "jpg"); err != nil {
		t.Fatal(err)
	}

	repository := &MySQLStorage{db: db}
	annotation := integrationAnnotation(documentID, pageID, "tenant-a", "0199b252-3c6f-7f6a-9a13-6b7c8d9e0101")
	created, replayed, err := repository.CreateAnnotation(ctx, annotation)
	if err != nil || replayed || created.ID != annotation.ID {
		t.Fatalf("create replayed=%v err=%v", replayed, err)
	}
	found, err := repository.GetAnnotation(ctx, annotation.ID, "tenant-a", "project-a", false)
	if err != nil || len(found.Selector.Rectangles) != 1 {
		t.Fatalf("JSON selector not persisted: %#v err=%v", found, err)
	}
	if _, replayed, err := repository.CreateAnnotation(ctx, annotation); err != nil || !replayed {
		t.Fatalf("idempotency replayed=%v err=%v", replayed, err)
	}

	otherTenant := integrationAnnotation(documentID, pageID, "tenant-b", "0199b252-3c6f-7f6a-9a13-6b7c8d9e0101")
	otherTenant.ID = "0199b252-3c6f-7f6a-9a13-6b7c8d9e0102"
	if _, replayed, err := repository.CreateAnnotation(ctx, otherTenant); err != nil || replayed {
		t.Fatalf("same client reference in another tenant: replayed=%v err=%v", replayed, err)
	}

	badPage := integrationAnnotation(documentID, "missing-page", "tenant-a", "0199b252-3c6f-7f6a-9a13-6b7c8d9e0103")
	badPage.ID = "0199b252-3c6f-7f6a-9a13-6b7c8d9e0103"
	if _, _, err := repository.CreateAnnotation(ctx, badPage); err == nil {
		t.Fatal("page FK accepted a missing page")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM documents WHERE id=?`, documentID); err == nil {
		t.Fatal("ON DELETE RESTRICT did not protect annotations")
	}
	if err := repository.SoftDeleteAnnotation(ctx, annotation.ID, "tenant-a", "project-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetAnnotation(ctx, annotation.ID, "tenant-a", "project-a", false); err != ErrAnnotationNotFound {
		t.Fatalf("soft deleted annotation visible: %v", err)
	}
	if deleted, err := repository.GetAnnotation(ctx, annotation.ID, "tenant-a", "project-a", true); err != nil || deleted.DeletedAt == nil {
		t.Fatalf("deleted_at missing: %#v err=%v", deleted, err)
	}
}

func filepathFromStorageTests() string { return "../.." }

func integrationAnnotation(documentID, pageID, tenant, clientReference string) *models.DocumentAnnotation {
	note, color := "nota", "yellow"
	return &models.DocumentAnnotation{
		ID: "0199b252-3c6f-7f6a-9a13-6b7c8d9e0100", ProjectKey: "project-a", TenantID: tenant,
		DocumentID: documentID, PageID: pageID, PageNumber: 1,
		OCRGenerationID:   "00000000-0000-4000-8000-000000000102",
		LayerSHA256:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ClientReferenceID: clientReference, RequestFingerprint: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		SelectedText: "texto", Note: &note, Color: &color, Version: 1,
		Selector: models.AnnotationSelector{Type: "MultiRectangleSelector", CoordinateSpace: "canvas_normalized",
			Canvas:     models.AnnotationCanvas{ID: "https://iiif.test/canvas/1", Width: 1241, Height: 1754},
			Rectangles: []models.AnnotationRectangle{{X: .1, Y: .1, Width: .2, Height: .1}},
			Quote:      models.AnnotationQuote{Exact: "texto"}},
	}
}
