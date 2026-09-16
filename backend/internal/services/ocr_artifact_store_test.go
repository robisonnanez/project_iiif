package services

import (
	"errors"
	"path/filepath"
	"testing"

	"iiif-pdf-server/internal/storage"
)

type failingOCRArtifactStorage struct{ storage.Storage }

func (failingOCRArtifactStorage) PutOCRArtifact(string, []byte, string) error {
	return errors.New("backend unavailable")
}
func (failingOCRArtifactStorage) GetOCRArtifact(string) ([]byte, error) {
	return nil, errors.New("NoSuchKey")
}
func (failingOCRArtifactStorage) ListOCRArtifacts(string) ([]string, error) {
	return nil, errors.New("backend unavailable")
}
func (failingOCRArtifactStorage) DeleteOCRArtifacts(string) error {
	return errors.New("backend unavailable")
}

func TestConfiguredOCRArtifactStoreFallsBackToLegacyLocalReads(t *testing.T) {
	root := t.TempDir()
	local := &localOCRArtifactStore{root: root}
	if err := local.Put("pages/doc/g1/000001.json.gz", []byte("legacy"), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	store := newOCRArtifactStore(root, failingOCRArtifactStorage{Storage: storage.NewFileStorage(filepath.Join(root, "metadata"))})
	data, err := store.Get("pages/doc/g1/000001.json.gz")
	if err != nil || string(data) != "legacy" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if err := store.Put("pages/doc/g2/000001.json.gz", []byte("new"), "application/gzip"); err == nil {
		t.Fatal("write must not silently fall back when S3 is unavailable")
	}
}

func TestUnmarshalGzipJSONRejectsCorruptArtifact(t *testing.T) {
	var page OCRPage
	if err := unmarshalGzipJSON([]byte("not-gzip"), &page); err == nil {
		t.Fatal("expected corrupt gzip error")
	}
}
