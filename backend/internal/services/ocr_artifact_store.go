package services

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"iiif-pdf-server/internal/storage"
)

type ocrArtifactStore interface {
	Put(string, []byte, string) error
	Get(string) ([]byte, error)
	List(string) ([]string, error)
	DeletePrefix(string) error
}

type localOCRArtifactStore struct{ root string }

func (s *localOCRArtifactStore) clean(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimLeft(key, "/")))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("clave de artefacto OCR inválida")
	}
	return filepath.Join(s.root, clean), nil
}

func (s *localOCRArtifactStore) Put(key string, data []byte, _ string) error {
	name, err := s.clean(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(name), ".ocr-artifact-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempName, name)
}

func (s *localOCRArtifactStore) Get(key string) ([]byte, error) {
	name, err := s.clean(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(name)
}

func (s *localOCRArtifactStore) List(prefix string) ([]string, error) {
	name, err := s.clean(prefix)
	if err != nil {
		return nil, err
	}
	items := make([]string, 0)
	err = filepath.WalkDir(name, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return relErr
		}
		items = append(items, filepath.ToSlash(relative))
		return nil
	})
	if os.IsNotExist(err) {
		return items, nil
	}
	return items, err
}

func (s *localOCRArtifactStore) DeletePrefix(prefix string) error {
	name, err := s.clean(prefix)
	if err != nil {
		return err
	}
	return os.RemoveAll(name)
}

type configuredOCRArtifactStore struct {
	primary  storage.OCRArtifactStorage
	fallback *localOCRArtifactStore
}

func newOCRArtifactStore(root string, store storage.Storage) ocrArtifactStore {
	local := &localOCRArtifactStore{root: root}
	if remote, ok := store.(storage.OCRArtifactStorage); ok {
		return &configuredOCRArtifactStore{primary: remote, fallback: local}
	}
	return local
}

func (s *configuredOCRArtifactStore) Put(key string, data []byte, mediaType string) error {
	return s.primary.PutOCRArtifact(key, data, mediaType)
}
func (s *configuredOCRArtifactStore) Get(key string) ([]byte, error) {
	data, err := s.primary.GetOCRArtifact(key)
	if err == nil {
		return data, nil
	}
	legacy, legacyErr := s.fallback.Get(key)
	if legacyErr == nil {
		return legacy, nil
	}
	return nil, err
}
func (s *configuredOCRArtifactStore) List(prefix string) ([]string, error) {
	return s.primary.ListOCRArtifacts(prefix)
}
func (s *configuredOCRArtifactStore) DeletePrefix(prefix string) error {
	if err := s.primary.DeleteOCRArtifacts(prefix); err != nil {
		return err
	}
	return s.fallback.DeletePrefix(prefix)
}

func marshalJSON(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func marshalGzipJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func unmarshalGzipJSON(data []byte, target any) error {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer reader.Close()
	decoder := json.NewDecoder(io.LimitReader(reader, 64<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decodificar artefacto OCR: %w", err)
	}
	return nil
}
