package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateProcessingCopyOwnsIndependentPDF(t *testing.T) {
	tempPath := t.TempDir()
	source := filepath.Join(tempPath, "upload.pdf")
	want := []byte("%PDF-independent-worker-copy")
	if err := os.WriteFile(source, want, 0o600); err != nil {
		t.Fatal(err)
	}

	workerPath, err := createProcessingCopy(tempPath, want)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(workerPath)
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("worker copy disappeared with upload temp file: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("worker copy=%q, want %q", got, want)
	}
}
