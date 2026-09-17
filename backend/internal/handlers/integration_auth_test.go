package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

	"github.com/gin-gonic/gin"
)

func newIntegrationAuthTest(t *testing.T) (*IntegrationAuth, string) {
	t.Helper()
	cfg := &config.Config{}
	cfg.ApplyDefaults()
	cfg.Security.IntegrationAuth.Enabled = true
	cfg.Security.IntegrationAuth.HMACSecret = "0123456789abcdef0123456789abcdef"
	cfg.Frontend.RequireAuth = true
	cfg.Security.IntegrationAuth.RatePerMinute = 1
	cfg.Security.IntegrationAuth.Burst = 0
	store := storage.NewFileStorage(filepath.Join(t.TempDir(), "data"))
	id := "00000000-0000-4000-8000-000000000001"
	if err := store.SaveDocument(&models.PDFDocument{ID: id, Name: "doc.pdf", ProjectKey: "project-a", TenantKey: "tenant-a", Status: "completed", UploadDate: time.Now()}); err != nil {
		t.Fatal(err)
	}
	auth, err := NewIntegrationAuth(cfg, store, NewAuthHandler(cfg))
	if err != nil {
		t.Fatal(err)
	}
	return auth, id
}

func TestIntegrationAuthRejectsShortSecret(t *testing.T) {
	cfg := &config.Config{}
	cfg.ApplyDefaults()
	cfg.Security.IntegrationAuth.Enabled = true
	cfg.Security.IntegrationAuth.HMACSecret = "short"
	cfg.Frontend.RequireAuth = true
	if _, err := NewIntegrationAuth(cfg, storage.NewFileStorage(t.TempDir()), NewAuthHandler(cfg)); err == nil {
		t.Fatal("expected short secret error")
	}
}

func TestIntegrationAuthRequiresProtectedAdminSession(t *testing.T) {
	cfg := &config.Config{}
	cfg.ApplyDefaults()
	cfg.Security.IntegrationAuth.Enabled = true
	cfg.Security.IntegrationAuth.HMACSecret = "0123456789abcdef0123456789abcdef"
	cfg.Frontend.RequireAuth = false
	if _, err := NewIntegrationAuth(cfg, storage.NewFileStorage(t.TempDir()), NewAuthHandler(cfg)); err == nil {
		t.Fatal("expected require_auth validation error")
	}
}

func TestIntegrationAuthScopesDocumentAndRateLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth, id := newIntegrationAuthTest(t)
	now := time.Now()
	token, err := auth.sign(integrationClaims{Issuer: auth.config.Issuer, Subject: "consumer-a", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Scope: textLayerReadScope, Project: "project-a", Tenant: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/documents/:id", auth.RequireRead(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/documents/"+id, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/documents/"+id, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestIntegrationAuthRejectsCrossTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth, id := newIntegrationAuthTest(t)
	now := time.Now()
	token, _ := auth.sign(integrationClaims{Issuer: auth.config.Issuer, Subject: "consumer-b", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Scope: textLayerReadScope, Project: "project-a", Tenant: "tenant-b"})
	router := gin.New()
	router.GET("/documents/:id", auth.RequireRead(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/documents/"+id, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestIntegrationAuthAnnotationScopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth, _ := newIntegrationAuthTest(t)
	now := time.Now()
	tests := []struct {
		name, tokenScope, required string
		expires                    time.Time
		want                       int
	}{
		{"read accepted", annotationsReadScope, annotationsReadScope, now.Add(time.Minute), http.StatusNoContent},
		{"write cannot read", annotationsWriteScope, annotationsReadScope, now.Add(time.Minute), http.StatusForbidden},
		{"read cannot write", annotationsReadScope, annotationsWriteScope, now.Add(time.Minute), http.StatusForbidden},
		{"expired", annotationsReadScope, annotationsReadScope, now.Add(-time.Second), http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, err := auth.sign(integrationClaims{Issuer: auth.config.Issuer, Subject: "consumer", IssuedAt: now.Add(-time.Minute).Unix(), ExpiresAt: test.expires.Unix(), Scope: test.tokenScope, Project: "project-a", Tenant: "tenant-a"})
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.GET("/annotations", auth.RequireScope(test.required), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/annotations", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestIntegrationAuthAnnotationRequiresTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth, _ := newIntegrationAuthTest(t)
	now := time.Now()
	token, _ := auth.sign(integrationClaims{Issuer: auth.config.Issuer, Subject: "consumer", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Scope: annotationsReadScope, Project: "project-a"})
	router := gin.New()
	router.GET("/annotations", auth.RequireScope(annotationsReadScope), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/annotations", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
