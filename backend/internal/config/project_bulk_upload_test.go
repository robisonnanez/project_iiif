package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyProjectDefaultsBulkUploadToFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	legacy := []byte("projects:\n  enabled: true\n  default_project: legacy\n  items:\n    - key: legacy\n      name: Legacy\n")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Projects.Items[0].BulkUpload {
		t.Fatal("bulk_upload debe ser false cuando no existe en una configuración antigua")
	}
	if cfg.Security.MaxConcurrentUploads != 5 {
		t.Fatalf("max_concurrent_uploads=%d, want 5", cfg.Security.MaxConcurrentUploads)
	}
}

func TestProjectBulkUploadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Default()
	cfg.Projects.Enabled = true
	cfg.Projects.DefaultProject = "bulk"
	cfg.Projects.Items = []ProjectConfig{{Key: "bulk", Name: "Carga masiva", BulkUpload: true}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	project, ok := reloaded.ProjectByKey("bulk")
	if !ok || !project.BulkUpload {
		t.Fatalf("bulk_upload no se conservó: project=%+v exists=%t", project, ok)
	}
}

func TestMaxConcurrentUploadsIsCappedAtOneHundred(t *testing.T) {
	cfg := Default()
	cfg.Security.MaxConcurrentUploads = 1000
	cfg.ApplyDefaults()
	if cfg.Security.MaxConcurrentUploads != 100 {
		t.Fatalf("max_concurrent_uploads=%d, want 100", cfg.Security.MaxConcurrentUploads)
	}
}
