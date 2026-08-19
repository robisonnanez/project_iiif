package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

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
	if strings.Contains(body, "config.yaml") {
		t.Fatal("login must not expose config.yaml implementation detail")
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
