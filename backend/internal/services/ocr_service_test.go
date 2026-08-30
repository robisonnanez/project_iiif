package services

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"
)

func TestParseTesseractTSV(t *testing.T) {
	input := strings.Join([]string{
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
		"5\t1\t1\t1\t1\t1\t10\t20\t30\t12\t95.5\tHola",
		"5\t1\t1\t1\t1\t2\t45\t20\t50\t12\t84.5\tmundo",
	}, "\n")
	text, words, confidence, err := parseTesseractTSV([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hola mundo" {
		t.Fatalf("text = %q", text)
	}
	if len(words) != 2 || words[0].Left != 10 || words[1].Width != 50 {
		t.Fatalf("words = %#v", words)
	}
	if confidence != 90 {
		t.Fatalf("confidence = %v", confidence)
	}
}

func TestNormalizeSearchPreservesOriginalSemantics(t *testing.T) {
	got := normalizeSearch("  Índices   del\nCAFÉ  ")
	if got != "indices del cafe" {
		t.Fatalf("normalizeSearch() = %q", got)
	}
	if usefulRunes(" -- á1 -- ") != 2 {
		t.Fatalf("usefulRunes() returned unexpected count")
	}
}

func TestSanitizeLanguages(t *testing.T) {
	got := sanitizeLanguages([]string{"SPA", "eng", "spa", "deu"}, []string{"spa", "eng", "fra", "por"})
	if strings.Join(got, ",") != "spa,eng" {
		t.Fatalf("languages = %#v", got)
	}
}

func TestOCRAutocompletePrefixDeduplicationAccentsAndRanking(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{{
		id: "doc-a", project: "project-a", tenant: "tenant-a",
		pages: []string{
			"funciones funcionalidad funcionamiento función microfuncionalidad",
			"FUNCIONES Funciones funciones información",
		},
	}})

	items, err := service.Autocomplete("func", "project-a", "tenant-a", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"funciones", "función", "funcionalidad", "funcionamiento"}
	if len(items) != len(want) {
		t.Fatalf("items = %#v", items)
	}
	for index, text := range want {
		if items[index].Text != text {
			t.Fatalf("items[%d] = %#v, want %q", index, items[index], text)
		}
	}
	if items[0].Frequency != 4 {
		t.Fatalf("frecuencia de funciones = %d", items[0].Frequency)
	}
	for _, item := range items {
		if item.Text == "microfuncionalidad" {
			t.Fatal("autocomplete incluyó una coincidencia por substring")
		}
	}

	accented, err := service.Autocomplete("informacion", "project-a", "tenant-a", "", 10)
	if err != nil || len(accented) != 1 || accented[0].Text != "información" {
		t.Fatalf("autocomplete sin acento = %#v, %v", accented, err)
	}
}

func TestOCRAutocompleteExactMatchLimitAndMinimumPrefix(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{{
		id: "doc-a", project: "project-a", pages: []string{"funcion funcionalidad funciones funcionamiento funcional"},
	}})

	items, err := service.Autocomplete("funcion", "project-a", "", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Text != "funcion" {
		t.Fatalf("exact match/limit = %#v", items)
	}
	if _, err := service.Autocomplete("f", "project-a", "", "", 10); err == nil {
		t.Fatal("se esperaba error para un prefijo de un carácter")
	}
}

func TestOCRAutocompleteRespectsProjectTenantAndDocumentScope(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{
		{id: "doc-a", project: "project-a", tenant: "tenant-a", pages: []string{"astronomía"}},
		{id: "doc-a2", project: "project-a", tenant: "tenant-b", pages: []string{"astrolabio"}},
		{id: "doc-b", project: "project-b", tenant: "tenant-a", pages: []string{"astrología"}},
	})

	items, err := service.Autocomplete("ast", "project-a", "tenant-a", "", 10)
	if err != nil || len(items) != 1 || items[0].Text != "astronomía" {
		t.Fatalf("scope de proyecto/tenant = %#v, %v", items, err)
	}
	items, err = service.Autocomplete("ast", "", "", "doc-b", 10)
	if err != nil || len(items) != 1 || items[0].Text != "astrología" {
		t.Fatalf("scope de documento = %#v, %v", items, err)
	}
}

func TestOCRAutocompleteBackfillsAndDeletesVocabulary(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{{
		id: "doc-a", project: "project-a", pages: []string{"funciones funciones"},
	}})
	items, err := service.Autocomplete("fun", "project-a", "", "", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("backfill = %#v, %v", items, err)
	}
	vocabularyPath := filepath.Join(service.root, "vocabularies", "doc-a", "generation-1.json.gz")
	if _, err := os.Stat(vocabularyPath); err != nil {
		t.Fatalf("no se creó el vocabulario durable: %v", err)
	}
	if err := service.Delete("doc-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(vocabularyPath); !os.IsNotExist(err) {
		t.Fatalf("el vocabulario no fue eliminado: %v", err)
	}
}

func BenchmarkOCRAutocomplete(b *testing.B) {
	words := make([]string, 0, 20000)
	for index := 0; index < 20000; index++ {
		words = append(words, "palabra"+strconv.Itoa(index))
	}
	words = append(words, "función", "funciones", "funcionalidad", "funcionamiento")
	service := newAutocompleteBenchmarkService(b, strings.Join(words, " "))
	if _, err := service.Autocomplete("func", "project-a", "", "", 10); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := service.Autocomplete("func", "project-a", "", "", 10); err != nil {
			b.Fatal(err)
		}
	}
}

type autocompleteTestDocument struct {
	id      string
	project string
	tenant  string
	pages   []string
}

func newAutocompleteTestService(t *testing.T, documents []autocompleteTestDocument) *OCRService {
	t.Helper()
	return newAutocompleteService(t, documents)
}

type autocompleteTesting interface {
	Helper()
	TempDir() string
	Fatal(...any)
}

func newAutocompleteBenchmarkService(b *testing.B, text string) *OCRService {
	b.Helper()
	return newAutocompleteService(b, []autocompleteTestDocument{{id: "benchmark", project: "project-a", pages: []string{text}}})
}

func newAutocompleteService(tb autocompleteTesting, documents []autocompleteTestDocument) *OCRService {
	tb.Helper()
	root := tb.TempDir()
	store := storage.NewFileStorage(filepath.Join(root, "metadata"))
	cfg := &config.Config{}
	cfg.Storage.DataPath = filepath.Join(root, "artifacts")
	cfg.OCR.Workers = 1
	service, err := NewOCRService(cfg, store)
	if err != nil {
		tb.Fatal(err)
	}
	for _, item := range documents {
		document := &models.PDFDocument{ID: item.id, Name: item.id + ".pdf", ProjectKey: item.project, TenantKey: item.tenant, Status: "completed", TotalPages: len(item.pages), UploadDate: time.Now().UTC()}
		if err := store.SaveDocument(document); err != nil {
			tb.Fatal(err)
		}
		for index, text := range item.pages {
			page := &OCRPage{SchemaVersion: ocrSchemaVersion, DocumentID: item.id, Generation: "generation-1", PageNumber: index + 1, Status: "indexed", Source: "ocr", Text: text, CreatedAt: time.Now().UTC()}
			if err := service.savePage(page); err != nil {
				tb.Fatal(err)
			}
		}
		summary := &OCRDocumentSummary{DocumentID: item.id, ProjectKey: item.project, TenantKey: item.tenant, ActiveGeneration: "generation-1", Status: "completed", TotalPages: len(item.pages), IndexedPages: len(item.pages), UpdatedAt: time.Now().UTC()}
		if err := service.saveSummary(summary); err != nil {
			tb.Fatal(err)
		}
	}
	return service
}
