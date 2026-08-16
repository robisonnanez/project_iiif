package services

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
)

func TestManifestPagesSupportsRangesAndRemovesDuplicates(t *testing.T) {
	pages, normalized, err := manifestPages("5, 1-3, 3", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 5}
	if normalized != "1,2,3,5" || len(pages) != len(want) {
		t.Fatalf("pages=%v normalized=%q", pages, normalized)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Fatalf("pages=%v want=%v", pages, want)
		}
	}
}

func TestManifestPagesDefaultsToAll(t *testing.T) {
	pages, normalized, err := manifestPages("all", 3)
	if err != nil || normalized != "all" || len(pages) != 3 {
		t.Fatalf("pages=%v normalized=%q err=%v", pages, normalized, err)
	}
}

func TestManifestPagesRejectsInvalidSelections(t *testing.T) {
	for _, selection := range []string{"0", "4", "3-2", "a", "1,,2"} {
		if _, _, err := manifestPages(selection, 3); err == nil {
			t.Fatalf("expected %q to fail", selection)
		}
	}
}

func TestManifestV2WithoutTOCOmitsStructures(t *testing.T) {
	store := newManifestTestStorage(testDocument(nil))
	manifest, err := testIIIFService(store).GetManifest("doc-1", "all")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(data)
	for _, required := range []string{`"@context":"http://iiif.io/api/presentation/2/context.json"`, `"@type":"sc:Manifest"`, `"sequences"`, `"canvases"`, `"images"`} {
		if !strings.Contains(jsonText, required) {
			t.Fatalf("manifest is missing %s: %s", required, jsonText)
		}
	}
	if strings.Contains(jsonText, `"structures"`) {
		t.Fatalf("manifest without TOC must omit structures: %s", jsonText)
	}
	if strings.Contains(jsonText, "s3://") {
		t.Fatalf("internal S3 paths leaked into manifest: %s", jsonText)
	}
}

func TestManifestV2HierarchicalTOCAndPartialSelection(t *testing.T) {
	outline := []models.PDFOutlineItem{
		{Level: 1, Title: "Chapter 1", PageNumber: 1},
		{Level: 2, Title: "Introduction", PageNumber: 2},
		{Level: 2, Title: "Background", PageNumber: 3},
		{Level: 1, Title: "Chapter 2", PageNumber: 5},
	}
	store := newManifestTestStorage(testDocument(outline))
	service := testIIIFService(store)
	manifest, err := service.GetManifest("doc-1", "2,5")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Sequences) != 1 || len(manifest.Sequences[0].Canvases) != 2 {
		t.Fatalf("partial canvases=%v", manifest.Sequences)
	}
	if len(manifest.Structures) != 2 {
		t.Fatalf("structures=%v", manifest.Structures)
	}
	first := manifest.Structures[0]
	if first.Label != "Chapter 1" || len(first.Ranges) != 1 || first.Ranges[0].Label != "Introduction" {
		t.Fatalf("unexpected filtered hierarchy: %#v", first)
	}
	if first.Ranges[0].Within != first.ID {
		t.Fatalf("child within=%q want=%q", first.Ranges[0].Within, first.ID)
	}
	data, _ := json.Marshal(manifest)
	jsonText := string(data)
	for _, excluded := range []string{"canvases/doc-1_0001", "canvases/doc-1_0003", "canvases/doc-1_0004", "canvases/doc-1_0006", "Background"} {
		if strings.Contains(jsonText, excluded) {
			t.Fatalf("partial manifest references excluded content %q: %s", excluded, jsonText)
		}
	}
	for _, included := range []string{"canvases/doc-1_0002", "canvases/doc-1_0005", "Introduction", "Chapter 2"} {
		if !strings.Contains(jsonText, included) {
			t.Fatalf("partial manifest missing %q: %s", included, jsonText)
		}
	}
}

func TestManifestV2IsDeterministicForMigratedS3Document(t *testing.T) {
	doc := testDocument([]models.PDFOutlineItem{{Level: 1, Title: "Contents", PageNumber: 1}})
	doc.MigratedFromLocal = true
	doc.FilePath = "s3://private-bucket/documents/doc-1/original.pdf"
	store := newManifestTestStorage(doc)
	service := testIIIFService(store)
	first, err := service.GetManifest("doc-1", "1-3")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.GetManifest("doc-1", "1,2,3")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic requests differ:\n%#v\n%#v", first, second)
	}
	data, _ := json.Marshal(first)
	if strings.Contains(string(data), "s3://") {
		t.Fatalf("internal S3 path leaked: %s", data)
	}
}

func testDocument(outline []models.PDFOutlineItem) *models.PDFDocument {
	return &models.PDFDocument{
		ID:             "doc-1",
		Name:           "Real document.pdf",
		Status:         "completed",
		TotalPages:     6,
		ConvertedPages: 6,
		Outline:        outline,
		FilePath:       "s3://private-bucket/documents/doc-1/original.pdf",
	}
}

func testIIIFService(store *manifestTestStorage) *IIIFService {
	cfg := config.Default()
	cfg.IIIF.BaseURL = "https://iiif.example.test"
	cfg.IIIF.CacheEnabled = false
	return NewIIIFService(cfg, store)
}

type manifestTestStorage struct {
	doc    *models.PDFDocument
	images map[int]*models.DocumentImage
}

func newManifestTestStorage(doc *models.PDFDocument) *manifestTestStorage {
	images := make(map[int]*models.DocumentImage, doc.TotalPages)
	for page := 1; page <= doc.TotalPages; page++ {
		images[page] = &models.DocumentImage{
			ID:         "doc-1_page_" + strconv.Itoa(page),
			DocumentID: doc.ID,
			PageNumber: page,
			ImagePath:  "s3://private-bucket/images/page.jpg",
			Width:      1241,
			Height:     1754,
			Format:     "jpg",
			MediaType:  "image/jpeg",
		}
	}
	return &manifestTestStorage{doc: doc, images: images}
}

func (s *manifestTestStorage) SaveDocument(*models.PDFDocument) error { return nil }
func (s *manifestTestStorage) GetDocument(id string) (*models.PDFDocument, error) {
	if id != s.doc.ID {
		return nil, errors.New("not found")
	}
	return s.doc, nil
}
func (s *manifestTestStorage) GetAllDocuments() ([]*models.PDFDocument, error) {
	return []*models.PDFDocument{s.doc}, nil
}
func (s *manifestTestStorage) GetDocumentsByScope(string, string) ([]*models.PDFDocument, error) {
	return []*models.PDFDocument{s.doc}, nil
}
func (s *manifestTestStorage) DeleteDocument(string) error                   { return nil }
func (s *manifestTestStorage) UpdateDocument(*models.PDFDocument) error      { return nil }
func (s *manifestTestStorage) SaveDocumentPDF(string, []byte, string) error  { return nil }
func (s *manifestTestStorage) SaveDocumentImage(*models.DocumentImage) error { return nil }
func (s *manifestTestStorage) SaveDocumentImageData(string, []byte, string) error {
	return nil
}
func (s *manifestTestStorage) GetDocumentImage(id string) (*models.DocumentImage, error) {
	for _, image := range s.images {
		if image.ID == id {
			return image, nil
		}
	}
	return nil, errors.New("not found")
}
func (s *manifestTestStorage) GetDocumentImageByPage(_ string, page int) (*models.DocumentImage, error) {
	image, ok := s.images[page]
	if !ok {
		return nil, errors.New("not found")
	}
	return image, nil
}
func (s *manifestTestStorage) GetDocumentImages(string) ([]*models.DocumentImage, error) {
	result := make([]*models.DocumentImage, 0, len(s.images))
	for page := 1; page <= len(s.images); page++ {
		result = append(result, s.images[page])
	}
	return result, nil
}
func (s *manifestTestStorage) GetDocumentImageData(string) (*models.BinaryAsset, error) {
	return nil, errors.New("not found")
}
