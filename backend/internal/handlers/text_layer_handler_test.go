package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	contract "iiif-pdf-server/internal/api"

	"github.com/gin-gonic/gin"
)

type fakeTextLayerService struct{}

func (fakeTextLayerService) GetTextLayerStatus(id string) (*contract.TextLayerStatus, error) {
	return &contract.TextLayerStatus{SchemaVersion: 1, DocumentID: id, Available: true, Status: "completed", Geometry: "word"}, nil
}
func (fakeTextLayerService) GetTextLayerPage(id, generation string, page int) (*contract.TextLayerPage, error) {
	return &contract.TextLayerPage{SchemaVersion: 1, DocumentID: id, Generation: generation, PageNumber: page, GeometrySpace: "canvas", GeometryStatus: "word", Words: []contract.TextLayerWord{}}, nil
}
func (fakeTextLayerService) ListOCRGenerations(id string) (*contract.OCRGenerationList, error) {
	return &contract.OCRGenerationList{SchemaVersion: 1, DocumentID: id, Generations: []contract.OCRGeneration{}}, nil
}

func TestTextLayerRejectsInvalidDocumentUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/documents/:id/text-layer/status", NewTextLayerHandler(fakeTextLayerService{}).Status)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/documents/not-a-uuid/text-layer/status", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTextLayerPageContractAndGenerationValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/documents/:id/text-layer/pages/:page", NewTextLayerHandler(fakeTextLayerService{}).Page)
	id := "00000000-0000-4000-8000-000000000001"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+id+"/text-layer/pages/1?generation=bad", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	generation := "00000000-0000-4000-8000-000000000002"
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+id+"/text-layer/pages/1?generation="+generation, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"geometry_space":"canvas"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
