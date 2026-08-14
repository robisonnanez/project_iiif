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
