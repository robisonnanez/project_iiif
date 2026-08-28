package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"
)

type bulkPDFTestStorage struct {
	storage.Storage
	mu                    sync.Mutex
	documents             map[string]*models.PDFDocument
	images                map[string]*models.DocumentImage
	tempPath              string
	failPage              int
	activeUploads         int
	maxActiveUploads      int
	stagedAtFirstUpload   bool
	checkedFirstUpload    bool
	terminalDocumentState chan *models.PDFDocument
}

func newBulkPDFTestStorage(tempPath string) *bulkPDFTestStorage {
	return &bulkPDFTestStorage{
		documents:             map[string]*models.PDFDocument{},
		images:                map[string]*models.DocumentImage{},
		tempPath:              tempPath,
		terminalDocumentState: make(chan *models.PDFDocument, 1),
	}
}

func clonePDFDocument(doc *models.PDFDocument) *models.PDFDocument {
	copy := *doc
	copy.ImagePaths = append([]string(nil), doc.ImagePaths...)
	copy.Outline = append([]models.PDFOutlineItem(nil), doc.Outline...)
	return &copy
}

func (s *bulkPDFTestStorage) SaveDocument(doc *models.PDFDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[doc.ID] = clonePDFDocument(doc)
	return nil
}

func (s *bulkPDFTestStorage) UpdateDocument(doc *models.PDFDocument) error {
	s.mu.Lock()
	copy := clonePDFDocument(doc)
	s.documents[doc.ID] = copy
	s.mu.Unlock()
	if copy.Status == "completed" || copy.Status == "error" {
		select {
		case s.terminalDocumentState <- copy:
		default:
		}
	}
	return nil
}

func (s *bulkPDFTestStorage) GetDocument(id string) (*models.PDFDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.documents[id]
	if !ok {
		return nil, errors.New("documento no encontrado")
	}
	return clonePDFDocument(doc), nil
}

func (s *bulkPDFTestStorage) SaveDocumentPDF(string, []byte, string) error { return nil }

func (s *bulkPDFTestStorage) SaveDocumentImageAsset(image *models.DocumentImage, _ []byte, _ string) error {
	s.mu.Lock()
	if !s.checkedFirstUpload {
		s.checkedFirstUpload = true
		doc := s.documents[image.DocumentID]
		files, _ := filepath.Glob(filepath.Join(s.tempPath, "bulk-"+image.DocumentID+"-*", "page_*"))
		s.stagedAtFirstUpload = doc != nil && doc.TotalPages > 0 && len(files) == doc.TotalPages
	}
	s.activeUploads++
	if s.activeUploads > s.maxActiveUploads {
		s.maxActiveUploads = s.activeUploads
	}
	s.mu.Unlock()

	time.Sleep(20 * time.Millisecond)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeUploads--
	if image.PageNumber == s.failPage {
		return errors.New("fallo S3 simulado")
	}
	copy := *image
	s.images[image.ID] = &copy
	return nil
}

func bulkPDFTestConfig(tempPath string) *config.Config {
	cfg := config.Default()
	cfg.BinaryStorage.Mode = "s3"
	cfg.BinaryStorage.TempPath = tempPath
	cfg.FilesystemDisk = "s3"
	cfg.PDF.TempPath = tempPath
	cfg.Projects.Enabled = true
	cfg.Projects.DefaultProject = "bulk"
	cfg.Projects.Items = []config.ProjectConfig{{Key: "bulk", Name: "Carga masiva", BulkUpload: true}}
	cfg.Security.MaxConcurrentUploads = 2
	return cfg
}

func waitForTerminalDocument(t *testing.T, states <-chan *models.PDFDocument) *models.PDFDocument {
	t.Helper()
	select {
	case doc := <-states:
		return doc
	case <-time.After(15 * time.Second):
		t.Fatal("la conversión no alcanzó un estado final")
		return nil
	}
}

func waitForBulkCleanup(t *testing.T, tempPath string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(tempPath, "bulk-*"))
		if len(matches) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("quedaron directorios temporales bulk en %s", tempPath)
}

func TestBulkUploadStagesEveryPageBeforeUploading(t *testing.T) {
	tempPath := t.TempDir()
	store := newBulkPDFTestStorage(tempPath)
	service := NewPDFService(bulkPDFTestConfig(tempPath), store)
	completed := make(chan string, 1)
	service.SetCompletionHook(func(documentID string) { completed <- documentID })

	doc, err := service.ProcessPDF(filepath.Join("testdata", "hierarchical_toc.pdf"), "outline.pdf", models.ConversionSettings{DPI: 72, MaxWidth: 256, MaxHeight: 256, Format: "jpg", Quality: 70}, &models.Scope{ProjectKey: "bulk"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalDocument(t, store.terminalDocumentState)
	if final.Status != "completed" || final.TotalPages == 0 || final.ConvertedPages != final.TotalPages {
		t.Fatalf("estado final = status=%s pages=%d/%d", final.Status, final.ConvertedPages, final.TotalPages)
	}
	store.mu.Lock()
	stagedAtFirstUpload := store.stagedAtFirstUpload
	maxActiveUploads := store.maxActiveUploads
	store.mu.Unlock()
	if !stagedAtFirstUpload {
		t.Fatal("la primera carga comenzó antes de que todas las páginas estuvieran preparadas")
	}
	if maxActiveUploads > 2 {
		t.Fatalf("cargas simultáneas=%d, máximo configurado=2", maxActiveUploads)
	}
	select {
	case completedID := <-completed:
		if completedID != doc.ID {
			t.Fatalf("completion hook id=%s, want %s", completedID, doc.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no se ejecutó el hook de finalización")
	}
	waitForBulkCleanup(t, tempPath)
}

func TestBulkUploadFailureMarksDocumentAsError(t *testing.T) {
	tempPath := t.TempDir()
	store := newBulkPDFTestStorage(tempPath)
	store.failPage = 1
	service := NewPDFService(bulkPDFTestConfig(tempPath), store)
	completed := make(chan string, 1)
	service.SetCompletionHook(func(documentID string) { completed <- documentID })

	_, err := service.ProcessPDF(filepath.Join("testdata", "hierarchical_toc.pdf"), "outline.pdf", models.ConversionSettings{DPI: 72, MaxWidth: 256, MaxHeight: 256, Format: "jpg", Quality: 70}, &models.Scope{ProjectKey: "bulk"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalDocument(t, store.terminalDocumentState)
	if final.Status != "error" {
		t.Fatalf("status=%s, want error", final.Status)
	}
	if final.ConvertedPages != final.TotalPages-1 {
		t.Fatalf("converted_pages=%d, want %d", final.ConvertedPages, final.TotalPages-1)
	}
	select {
	case documentID := <-completed:
		t.Fatalf("se ejecutó el hook para el documento fallido %s", documentID)
	case <-time.After(100 * time.Millisecond):
	}
	waitForBulkCleanup(t, tempPath)
}

func TestBulkUploadTemporaryDirectoryFailureMarksDocumentAsError(t *testing.T) {
	tempPath := t.TempDir()
	blockedPath := filepath.Join(tempPath, "not-a-directory")
	if err := os.WriteFile(blockedPath, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newBulkPDFTestStorage(tempPath)
	cfg := bulkPDFTestConfig(tempPath)
	cfg.BinaryStorage.TempPath = blockedPath
	service := NewPDFService(cfg, store)
	completed := make(chan string, 1)
	service.SetCompletionHook(func(documentID string) { completed <- documentID })

	_, err := service.ProcessPDF(filepath.Join("testdata", "hierarchical_toc.pdf"), "outline.pdf", models.ConversionSettings{DPI: 72, MaxWidth: 256, MaxHeight: 256, Format: "jpg", Quality: 70}, &models.Scope{ProjectKey: "bulk"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalDocument(t, store.terminalDocumentState)
	if final.Status != "error" || final.ConvertedPages != 0 {
		t.Fatalf("estado final = status=%s pages=%d", final.Status, final.ConvertedPages)
	}
	select {
	case documentID := <-completed:
		t.Fatalf("se ejecutó el hook para el documento fallido %s", documentID)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDisabledBulkUploadKeepsSequentialS3Writes(t *testing.T) {
	tempPath := t.TempDir()
	store := newBulkPDFTestStorage(tempPath)
	cfg := bulkPDFTestConfig(tempPath)
	cfg.Projects.Items[0].BulkUpload = false
	service := NewPDFService(cfg, store)

	_, err := service.ProcessPDF(filepath.Join("testdata", "hierarchical_toc.pdf"), "outline.pdf", models.ConversionSettings{DPI: 72, MaxWidth: 256, MaxHeight: 256, Format: "jpg", Quality: 70}, &models.Scope{ProjectKey: "bulk"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalDocument(t, store.terminalDocumentState)
	if final.Status != "completed" || final.ConvertedPages != final.TotalPages {
		t.Fatalf("estado final = status=%s pages=%d/%d", final.Status, final.ConvertedPages, final.TotalPages)
	}
	store.mu.Lock()
	maxActiveUploads := store.maxActiveUploads
	store.mu.Unlock()
	if maxActiveUploads != 1 {
		t.Fatalf("cargas simultáneas=%d, want 1", maxActiveUploads)
	}
}

func TestBulkUploadUsesOneGlobalConcurrencyLimit(t *testing.T) {
	tempPath := t.TempDir()
	store := newBulkPDFTestStorage(tempPath)
	service := NewPDFService(bulkPDFTestConfig(tempPath), store)
	makeItems := func(documentID string, count int) []stagedDocumentImage {
		items := make([]stagedDocumentImage, 0, count)
		for page := 1; page <= count; page++ {
			path := filepath.Join(tempPath, fmt.Sprintf("%s-%d.jpg", documentID, page))
			if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
				t.Fatal(err)
			}
			items = append(items, stagedDocumentImage{image: &models.DocumentImage{ID: fmt.Sprintf("%s-%d", documentID, page), DocumentID: documentID, PageNumber: page}, path: path, mediaType: "image/jpeg"})
		}
		return items
	}

	var group sync.WaitGroup
	group.Add(2)
	for _, documentID := range []string{"doc-a", "doc-b"} {
		items := makeItems(documentID, 5)
		go func() {
			defer group.Done()
			if errs := service.uploadStagedImages(items, func(stagedDocumentImage) {}); len(errs) != 0 {
				t.Errorf("uploadStagedImages() errors=%v", errs)
			}
		}()
	}
	group.Wait()
	store.mu.Lock()
	maxActiveUploads := store.maxActiveUploads
	store.mu.Unlock()
	if maxActiveUploads != 2 {
		t.Fatalf("máximo observado=%d, want 2", maxActiveUploads)
	}
}
