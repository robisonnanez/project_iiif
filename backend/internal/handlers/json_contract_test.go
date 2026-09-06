package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWriteJSONInitializesNestedNilSlices(t *testing.T) {
	type nested struct {
		Items []string `json:"items"`
	}
	type response struct {
		Installed []string `json:"installed"`
		Nested    nested   `json:"nested"`
		Optional  *string  `json:"optional"`
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	writeJSON(context, 200, response{})

	body := recorder.Body.String()
	if !strings.Contains(body, `"installed":[]`) || !strings.Contains(body, `"items":[]`) {
		t.Fatalf("las colecciones deben ser arreglos: %s", body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["optional"] != nil {
		t.Fatalf("los punteros nulos deben conservarse: %#v", decoded)
	}
}

func TestWriteJSONNormalizesSlicesInsideMaps(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	writeJSON(context, 200, gin.H{"status": "ok", "items": []string(nil)})
	if body := recorder.Body.String(); !strings.Contains(body, `"status":"ok"`) || !strings.Contains(body, `"items":[]`) {
		t.Fatalf("respuesta inesperada: %s", body)
	}
}
