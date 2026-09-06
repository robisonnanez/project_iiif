package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"iiif-pdf-server/internal/buildinfo"
	"iiif-pdf-server/internal/config"

	"github.com/gin-gonic/gin"
)

func TestWelcomeUsesModernLoginWithoutConfigReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Frontend.Enabled = true
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	NewWelcomeHandler(cfg).Welcome(ctx)
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, required := range []string{"Convierte, organiza y publica en IIIF", "Accede al panel de administración", "@media(max-width:820px)"} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing %q", required)
		}
	}
	for _, required := range []string{`target="_blank"`, `rel="noopener noreferrer"`, "Documentación API", "Iniciar sesión"} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing %q", required)
		}
	}
	if strings.Contains(body, "config.yaml") {
		t.Fatal("login must not expose config.yaml implementation detail")
	}
}

func TestVersionReturnsInjectedBuildMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousVersion, previousCommit, previousDate := buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = previousVersion, previousCommit, previousDate
	})
	buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = "1.2.3", "abc123", "2026-09-06T12:00:00Z"

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	NewWelcomeHandler(config.Default()).Version(ctx)
	var response buildinfo.Info
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || response.Commit != "abc123" || response.BuildDate == "" {
		t.Fatalf("status=%d response=%#v", recorder.Code, response)
	}
}

func TestErrorPageRendersRequestedStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/missing", nil)
	NewWelcomeHandler(config.Default()).ErrorPage(ctx, 404, "Página no encontrada", "No existe")
	if recorder.Code != 404 || !strings.Contains(recorder.Body.String(), "ERROR 404") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
