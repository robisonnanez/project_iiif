package services

import (
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

func TestTextLayerReadsExplicitImmutableGeneration(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{{id: "doc-a", project: "project-a", pages: []string{"versión uno"}}})
	service.config.OCR.Enabled = true
	page := &OCRPage{SchemaVersion: ocrSchemaVersion, DocumentID: "doc-a", Generation: "generation-2", PageNumber: 1, CanvasV3: "https://iiif.example/canvas/1", Width: 100, Height: 200, Status: "indexed", Source: "ocr", Text: "versión dos", GeometryStatus: "word", GeometrySpace: "canvas", Words: []OCRWord{{Text: "versión", BBox: OCRBoundingBox{X0: 1, Y0: 1, X1: 20, Y1: 10}}}}
	finalizeOCRPage(page)
	if err := service.savePage(page); err != nil {
		t.Fatal(err)
	}
	first, err := service.GetTextLayerPage("doc-a", "generation-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.GetTextLayerPage("doc-a", "generation-2", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Text != "versión uno" || second.Text != "versión dos" || first.LayerSHA256 == second.LayerSHA256 {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	again, err := service.GetTextLayerPage("doc-a", "generation-1", 1)
	if err != nil || again.LayerSHA256 != first.LayerSHA256 {
		t.Fatalf("historical generation mutated: %#v err=%v", again, err)
	}
}

func TestTextLayerMoreThan200PagesMeetsLocalLatencyTargets(t *testing.T) {
	service := newAutocompleteTestService(t, []autocompleteTestDocument{{id: "large-doc", project: "project-a", pages: []string{"página 1"}}})
	service.config.OCR.Enabled = true
	document, err := service.storage.GetDocument("large-doc")
	if err != nil {
		t.Fatal(err)
	}
	document.TotalPages = 214
	if err := service.storage.UpdateDocument(document); err != nil {
		t.Fatal(err)
	}
	for number := 2; number <= 214; number++ {
		page := &OCRPage{SchemaVersion: ocrSchemaVersion, DocumentID: "large-doc", Generation: "generation-1", PageNumber: number, CanvasV3: "https://iiif.example/canvas/" + strconv.Itoa(number), Width: 1241, Height: 1754, Status: "indexed", Source: "ocr", Text: "texto canónico", GeometryStatus: "page_only"}
		finalizeOCRPage(page)
		if err := service.savePage(page); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.saveSummary(&OCRDocumentSummary{DocumentID: "large-doc", ProjectKey: "project-a", ActiveGeneration: "generation-1", Status: "completed", TotalPages: 214, IndexedPages: 214, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	uncached := make([]time.Duration, 0, 214)
	for number := 1; number <= 214; number++ {
		started := time.Now()
		if _, err := service.GetTextLayerPage("large-doc", "generation-1", number); err != nil {
			t.Fatal(err)
		}
		uncached = append(uncached, time.Since(started))
	}
	service.textLayerCache = cache.New(time.Minute, time.Minute)
	cached := make([]time.Duration, 0, 214)
	for number := 1; number <= 214; number++ {
		_, _ = service.GetTextLayerPage("large-doc", "generation-1", number)
	}
	for number := 1; number <= 214; number++ {
		started := time.Now()
		if _, err := service.GetTextLayerPage("large-doc", "generation-1", number); err != nil {
			t.Fatal(err)
		}
		cached = append(cached, time.Since(started))
	}
	if percentile95(uncached) >= 800*time.Millisecond {
		t.Fatalf("uncached p95=%s", percentile95(uncached))
	}
	if percentile95(cached) >= 250*time.Millisecond {
		t.Fatalf("cached p95=%s", percentile95(cached))
	}
	t.Logf("214 pages: uncached p95=%s cached p95=%s", percentile95(uncached), percentile95(cached))
}

func percentile95(values []time.Duration) time.Duration {
	copyValues := append([]time.Duration(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return copyValues[(len(copyValues)*95-1)/100]
}
