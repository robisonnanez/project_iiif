package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"iiif-pdf-server/internal/config"

	"github.com/gin-gonic/gin"
)

func TestAdminOCRAutocompleteRequiresSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Frontend.RequireAuth = true
	auth := NewAuthHandler(cfg)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(auth.RequireSession())
	admin.GET("/ocr/autocomplete", NewOCRHandler(nil).Autocomplete)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ocr/autocomplete?q=func", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
