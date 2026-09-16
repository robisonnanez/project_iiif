package api

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestTextLayerPageContract(t *testing.T) {
	wantBytes, err := os.ReadFile("testdata/text_layer_page.json")
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}

	page := TextLayerPage{
		SchemaVersion: TextLayerSchemaVersion, DocumentID: "00000000-0000-4000-8000-000000000001",
		Generation: "00000000-0000-4000-8000-000000000002", PageNumber: 1,
		Canvas:        Canvas{ID: "https://iiif.example/canvas/1", Width: 1241, Height: 1754, ImageID: "image-1", ImageServiceID: "https://iiif.example/iiif/3/image-1"},
		GeometrySpace: "canvas", GeometryStatus: "word", Source: "ocr", Language: "spa", Confidence: 96.1,
		Text: "Texto", LayerSHA256: "abc123", Words: []TextLayerWord{{Order: 0, BlockIndex: 1, ParagraphIndex: 1, LineIndex: 1, WordIndex: 1, Text: "Texto", Confidence: 96.1, BBox: BoundingBox{X0: 104, Y0: 88, X1: 176, Y1: 111}}},
	}
	gotBytes, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(gotBytes, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contract changed\nwant=%s\ngot=%s", wantBytes, gotBytes)
	}
}

func TestGenerationListUsesNonNullArray(t *testing.T) {
	value := OCRGenerationList{SchemaVersion: 1, DocumentID: "doc", Generations: []OCRGeneration{}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"schema_version":1,"document_id":"doc","generations":[]}` {
		t.Fatalf("unexpected JSON: %s", data)
	}
	_ = time.Time{}
}
