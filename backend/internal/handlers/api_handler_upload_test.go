package handlers

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"iiif-pdf-server/internal/config"

	"github.com/gin-gonic/gin"
)

func TestConversionSettingsDefaults(t *testing.T) {
	handler := uploadSettingsTestHandler()
	context := uploadSettingsTestContext(url.Values{})

	settings, err := handler.conversionSettings(context)
	if err != nil {
		t.Fatalf("conversionSettings() error = %v", err)
	}
	if settings.MaxWidth != 1241 || settings.MaxHeight != 1754 || settings.DPI != 150 {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
	if settings.Format != "jpg" || settings.Quality != 85 {
		t.Fatalf("unexpected image defaults: %+v", settings)
	}
}

func TestConversionSettingsReadsMultipartFields(t *testing.T) {
	handler := uploadSettingsTestHandler()
	context := uploadSettingsTestContext(url.Values{
		"max_width":  {"1600"},
		"max_height": {"2200"},
		"dpi":        {"200"},
		"format":     {"png"},
		"quality":    {"90"},
	})

	settings, err := handler.conversionSettings(context)
	if err != nil {
		t.Fatalf("conversionSettings() error = %v", err)
	}
	if settings.MaxWidth != 1600 || settings.MaxHeight != 2200 || settings.DPI != 200 || settings.Format != "png" || settings.Quality != 90 {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestConversionSettingsUsesConfiguredDefaults(t *testing.T) {
	handler := uploadSettingsTestHandler()
	handler.config.Conversion.DefaultWidth = 1600
	handler.config.Conversion.DefaultHeight = 2200

	settings, err := handler.conversionSettings(uploadSettingsTestContext(url.Values{}))
	if err != nil {
		t.Fatal(err)
	}
	if settings.MaxWidth != 1600 || settings.MaxHeight != 2200 {
		t.Fatalf("unexpected configured defaults: %+v", settings)
	}
}

func TestConversionSettingsRejectsUnsafeDPI(t *testing.T) {
	handler := uploadSettingsTestHandler()
	context := uploadSettingsTestContext(url.Values{"dpi": {"1200"}})
	if _, err := handler.conversionSettings(context); err == nil {
		t.Fatal("conversionSettings() expected DPI validation error")
	}
}

func uploadSettingsTestHandler() *APIHandler {
	cfg := &config.Config{}
	cfg.Conversion.DefaultFormat = "jpg"
	cfg.Conversion.DefaultQuality = 85
	cfg.Conversion.DPI = 150
	return &APIHandler{config: cfg}
}

func uploadSettingsTestContext(values url.Values) *gin.Context {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := httptest.NewRequest("POST", "/upload", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	context.Request = request
	return context
}
