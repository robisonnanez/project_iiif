package handlers

import (
	"reflect"
	"testing"

	"iiif-pdf-server/internal/config"
)

func TestSecretOrCurrentPreservesExistingSecret(t *testing.T) {
	current := "secret"

	for name, value := range map[string]string{
		"empty":  "",
		"spaces": "   ",
		"masked": maskedSecret(current),
	} {
		t.Run(name, func(t *testing.T) {
			if got := secretOrCurrent(value, current); got != current {
				t.Fatalf("secretOrCurrent() = %q, want existing secret", got)
			}
		})
	}
}

func TestSecretOrCurrentAcceptsReplacement(t *testing.T) {
	if got := secretOrCurrent("new-secret", "old-secret"); got != "new-secret" {
		t.Fatalf("secretOrCurrent() = %q, want replacement", got)
	}
}

func TestValidateCORSOrigins(t *testing.T) {
	valid := []string{"https://app.example.com", "http://localhost:5173", "https://*.example.org"}
	if err := validateCORSOrigins(valid); err != nil {
		t.Fatalf("validateCORSOrigins() error = %v", err)
	}
	for _, invalid := range [][]string{{"app.example.com"}, {"https://example.com/path"}, {"https://foo.*.example.com"}} {
		if err := validateCORSOrigins(invalid); err == nil {
			t.Fatalf("validateCORSOrigins(%v) expected error", invalid)
		}
	}
}

func TestValidateOCRLanguages(t *testing.T) {
	if err := validateOCRLanguages([]string{"spa", "eng"}, []string{"spa"}); err != nil {
		t.Fatalf("validateOCRLanguages() error = %v", err)
	}
	if err := validateOCRLanguages([]string{"spa"}, []string{"eng"}); err == nil {
		t.Fatal("expected fallback outside candidates to fail")
	}
	if err := validateOCRLanguages([]string{"deu"}, []string{"deu"}); err == nil {
		t.Fatal("expected unsupported language to fail")
	}
}

func TestExtractTenantNamesSupportsCommonResponses(t *testing.T) {
	payload := map[string]any{"data": []any{map[string]any{"slug": "sunat"}, map[string]any{"id": "demo"}, "uniguajira", "../invalid", "SUNAT"}}
	got := extractTenantNames(payload)
	want := []string{"demo", "sunat", "uniguajira"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractTenantNames() = %#v, want %#v", got, want)
	}
}

func TestValidateProjectsRequiresSafeUniqueKeysAndDefault(t *testing.T) {
	valid := []config.ProjectConfig{{Key: "default", Name: "Default"}, {Key: "metavisor", Name: "Metavisor", Multitenant: true, Tenants: []string{"sunat"}, TenantsEndpoint: "https://example.test/tenants"}}
	if err := validateProjects(valid, "default"); err != nil {
		t.Fatalf("validateProjects() error = %v", err)
	}
	if err := validateProjects([]config.ProjectConfig{{Key: "../escape"}}, "../escape"); err == nil {
		t.Fatal("expected unsafe key to fail")
	}
	if err := validateProjects(valid, "missing"); err == nil {
		t.Fatal("expected missing default to fail")
	}
}
