package handlers

import "testing"

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
