package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"
)

func TestTesseractEngineRejectsEmptyLanguages(t *testing.T) {
	if _, _, _, err := (TesseractEngine{}).Recognize(context.Background(), "unused.png", nil); !errors.Is(err, ErrOCRNoLanguages) {
		t.Fatalf("error = %v", err)
	}
}

func TestRegenerateRejectsDuplicateActiveJobAndListsIt(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{{id: "doc-a", project: "project-a", pages: []string{"texto"}}})
	service.config.OCR.Enabled = true
	service.config.OCR.CandidateLanguages = []string{"spa", "eng"}
	service.config.OCR.FallbackLanguages = []string{"spa"}
	service.installedLanguages = func(context.Context) ([]string, error) { return []string{"eng", "spa"}, nil }

	start := make(chan struct{})
	var wait sync.WaitGroup
	var resultMu sync.Mutex
	jobs := make([]*OCRJob, 0, 1)
	errorsSeen := make([]error, 0, 7)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			job, err := service.Regenerate("doc-a", CreateOCRJobRequest{Mode: "ocr_only", LanguageMode: "manual", Languages: []string{"spa"}})
			resultMu.Lock()
			defer resultMu.Unlock()
			if err != nil {
				errorsSeen = append(errorsSeen, err)
			} else {
				jobs = append(jobs, job)
			}
		}()
	}
	close(start)
	wait.Wait()
	if len(jobs) != 1 || len(errorsSeen) != 7 {
		t.Fatalf("jobs=%d errors=%d", len(jobs), len(errorsSeen))
	}
	for _, err := range errorsSeen {
		if !errors.Is(err, ErrOCRActiveJob) {
			t.Fatalf("duplicate error = %v", err)
		}
	}
	job := jobs[0]
	if !job.Regeneration || job.DocumentName != "doc-a.pdf" || job.Message != "Esperando en cola" {
		t.Fatalf("job = %#v", job)
	}
	list := service.ListJobs(true, "doc-a")
	if list.Total != 1 || len(list.Jobs) != 1 || list.Jobs[0].ID != job.ID {
		t.Fatalf("active jobs = %#v", list)
	}
}

func TestAutomaticLanguageSelectionSpanishEnglishAndMixed(t *testing.T) {
	detectors := linguaLanguages([]string{"spa", "eng"})
	fallback := []string{"spa"}
	cases := []struct {
		name   string
		text   string
		wanted map[string]bool
	}{
		{name: "spanish", text: strings.Repeat("Este documento contiene información histórica en español sobre cultura y patrimonio. ", 8), wanted: map[string]bool{"spa": true}},
		{name: "english", text: strings.Repeat("This document contains historical information in English about culture and heritage. ", 8), wanted: map[string]bool{"eng": true}},
		{name: "mixed", text: strings.Repeat("Este documento contiene información histórica en español. This document also contains historical information in English. ", 12), wanted: map[string]bool{"spa": true, "eng": true}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := selectLanguagesFromSample(test.text, detectors, fallback, 0.70, 2)
			if len(got) != len(test.wanted) {
				t.Fatalf("languages = %#v", got)
			}
			for _, language := range got {
				if !test.wanted[language] {
					t.Fatalf("languages = %#v", got)
				}
			}
		})
	}
}

func TestOCRRegenerationIntegrationProducesWordsBBoxConfidenceAndEnglish(t *testing.T) {
	if testing.Short() {
		t.Skip("prueba OCR real")
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("Tesseract no está instalado")
	}
	pdf, err := os.ReadFile(filepath.Join("testdata", "without_toc.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cfg := &config.Config{}
	cfg.ApplyDefaults()
	cfg.Storage.DataPath = filepath.Join(root, "artifacts")
	cfg.PDF.TempPath = filepath.Join(root, "temp")
	cfg.OCR.Enabled = true
	cfg.OCR.Workers = 1
	cfg.OCR.CandidateLanguages = []string{"spa", "eng"}
	cfg.OCR.FallbackLanguages = []string{"eng"}
	cfg.OCR.LanguageDetection.Enabled = true
	cfg.OCR.LanguageDetection.SamplePages = 2
	cfg.OCR.LanguageDetection.MinSampleChars = 10
	cfg.OCR.LanguageDetection.MinimumConfidence = 0.60
	if err := os.MkdirAll(cfg.PDF.TempPath, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := storage.NewFileStorage(filepath.Join(root, "metadata"))
	document := &models.PDFDocument{ID: "english-fixture", Name: "english-fixture.pdf", Status: "completed", TotalPages: 2, ConvertedPages: 2, UploadDate: time.Now().UTC()}
	if err := metadata.SaveDocument(document); err != nil {
		t.Fatal(err)
	}
	store := &ocrIntegrationStorage{Storage: metadata, pdf: pdf}
	service, err := NewOCRService(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Regenerate(document.ID, CreateOCRJobRequest{Mode: "ocr_only", LanguageMode: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		job, err = service.GetJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !isActiveOCRStatus(job.Status) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if job.Status != "completed" {
		t.Fatalf("job status=%s error=%s", job.Status, job.Error)
	}
	if strings.Join(job.Languages, "+") != "eng" {
		t.Fatalf("tesseract languages = %#v", job.Languages)
	}
	page, err := service.GetPage(document.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Text == "" || page.Confidence <= 0 || len(page.Words) == 0 || page.GeometryStatus != "word" {
		t.Fatalf("OCR page missing current geometry: text=%q confidence=%v words=%d geometry=%s", page.Text, page.Confidence, len(page.Words), page.GeometryStatus)
	}
	for _, word := range page.Words {
		if word.Confidence <= 0 || word.BBox.X1 <= word.BBox.X0 || word.BBox.Y1 <= word.BBox.Y0 {
			t.Fatalf("invalid OCR word: %#v", word)
		}
	}
}

type ocrIntegrationStorage struct {
	storage.Storage
	pdf []byte
}

func (s *ocrIntegrationStorage) GetDocumentPDFData(string) (*models.BinaryAsset, error) {
	return &models.BinaryAsset{ID: "english-fixture", Data: s.pdf, MediaType: "application/pdf", ByteSize: int64(len(s.pdf))}, nil
}

func TestParseTesseractTSV(t *testing.T) {
	input := strings.Join([]string{
		"",
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
		"1\t1\t0\t0\t0\t0\t0\t0\t2000\t3000\t-1\t",
		"5\t1\t1\t1\t1\t1\t940\t1543\t76\t14\t95.20067596435548\tSÁNCHEZ,",
		"5\t1\t1\t1\t1\t2\t1020\t1543\t80\t14\t91.5\tartículo",
		"5\t1\t1\t1\t1\t3\t1110\t1543\t42\t14\t90.25\tNIÑO",
		"5\t1\t1\t1\t1\t4\t1160\t1543\t38\t14\t88.75\t62-C",
		"5\t1\t1\t1\t1\t5\t1200\t1543\t40\t14\t-1\tnegativa",
		"5\t1\t1\t1\t1\t6\t1240\t1543\t40\t14\t80.0\t",
		"malformada",
		"5\t1\t1\t1\t1\t7\tno-numero\t1543\t40\t14\t80.0\tignorada",
	}, "\n")
	text, words, confidence, err := parseTesseractTSV([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if text != "SÁNCHEZ, artículo NIÑO 62-C" {
		t.Fatalf("text = %q", text)
	}
	if len(words) != 4 {
		t.Fatalf("words = %#v", words)
	}
	first := words[0]
	if first.Text != "SÁNCHEZ," || first.Confidence != 95.20067596435548 {
		t.Fatalf("first word = %#v", first)
	}
	wantBox := (OCRBoundingBox{X0: 940, X1: 1016, Y0: 1543, Y1: 1557})
	if first.BBox != wantBox {
		t.Fatalf("bbox = %#v, want %#v", first.BBox, wantBox)
	}
	if confidence <= 0 || confidence >= 100 {
		t.Fatalf("confidence promedio = %v", confidence)
	}
}

func TestFilterOCRWordsMatchesAccentsCaseAndOuterPunctuation(t *testing.T) {
	words := []OCRWord{
		{Text: "SÁNCHEZ,", Confidence: 95, BBox: OCRBoundingBox{X0: 10, X1: 30, Y0: 20, Y1: 40}},
		{Text: "sanchez", Confidence: 90, BBox: OCRBoundingBox{X0: 40, X1: 60, Y0: 20, Y1: 40}},
		{Text: "otro", Confidence: 99, BBox: OCRBoundingBox{X0: 70, X1: 90, Y0: 20, Y1: 40}},
	}
	items := filterOCRWords(words, normalizeWordLookup("sánchez"), 1)
	if len(items) != 1 || items[0].Text != "SÁNCHEZ," || items[0].BBox.X1 != 30 {
		t.Fatalf("resultado inesperado: %+v", items)
	}
}

func TestParseTesseractTSVRejectsMissingHeader(t *testing.T) {
	if _, _, _, err := parseTesseractTSV([]byte("línea\tmalformada\n")); err == nil {
		t.Fatal("se esperaba error para TSV sin encabezado")
	}
}

func TestOCRWordReadsLegacyCoordinatesAndWritesBBox(t *testing.T) {
	var word OCRWord
	if err := json.Unmarshal([]byte(`{"text":"SÁNCHEZ,","confidence":95.2,"left":940,"top":1543,"width":76,"height":14}`), &word); err != nil {
		t.Fatal(err)
	}
	if word.BBox != (OCRBoundingBox{X0: 940, X1: 1016, Y0: 1543, Y1: 1557}) {
		t.Fatalf("legacy bbox = %#v", word.BBox)
	}
	encoded, err := json.Marshal(word)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"left"`) || !strings.Contains(string(encoded), `"bbox"`) {
		t.Fatalf("JSON nuevo inesperado: %s", encoded)
	}
}

func TestScaleOCRWordsToCanvasCoordinates(t *testing.T) {
	words := []OCRWord{{Text: "SÁNCHEZ,", Confidence: 95.2, BBox: OCRBoundingBox{X0: 940, X1: 1016, Y0: 1543, Y1: 1557}}}
	scaled := scaleOCRWords(words, 2000, 3000, 1000, 1500)
	want := OCRBoundingBox{X0: 470, X1: 508, Y0: 772, Y1: 779}
	if len(scaled) != 1 || scaled[0].BBox != want {
		t.Fatalf("scaled = %#v, want %#v", scaled, want)
	}
}

func TestOCRPagePersistsWordGeometry(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{{id: "doc-a", project: "project-a", pages: []string{"SÁNCHEZ,"}}})
	page := &OCRPage{SchemaVersion: ocrSchemaVersion, DocumentID: "doc-a", Generation: "generation-1", PageNumber: 1, Status: "indexed", Source: "ocr", Width: 1000, Height: 1500, OCRImageWidth: 2000, OCRImageHeight: 3000, Text: "SÁNCHEZ,", GeometryStatus: "word", GeometrySpace: "canvas", Words: []OCRWord{{Text: "SÁNCHEZ,", Confidence: 95.2, BBox: OCRBoundingBox{X0: 470, X1: 508, Y0: 772, Y1: 779}}}}
	if err := service.savePage(page); err != nil {
		t.Fatal(err)
	}
	result, err := service.readPage("doc-a", "generation-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.GeometrySpace != "canvas" || result.Width != 1000 || result.Height != 1500 || result.OCRImageWidth != 2000 || result.OCRImageHeight != 3000 {
		t.Fatalf("page geometry = %#v", result)
	}
	if len(result.Words) != 1 || result.Words[0].BBox != (OCRBoundingBox{X0: 470, X1: 508, Y0: 772, Y1: 779}) {
		t.Fatalf("page words = %#v", result.Words)
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
