package storage

import (
	"errors"
	"strings"
	"testing"

	"iiif-pdf-server/internal/models"

	"github.com/aws/smithy-go"
)

func TestS3ObjectKeysAreScopedAndSanitized(t *testing.T) {
	doc := &models.PDFDocument{ID: "../doc/one", ProjectKey: "../project", TenantKey: "tenant\\one"}
	key := documentObjectKey(doc)
	if strings.Contains(key, "..") || strings.Contains(key, "\\") || strings.HasPrefix(key, "/") {
		t.Fatalf("unsafe document key: %q", key)
	}
	if !strings.HasSuffix(key, "/documents/_doc_one/document.pdf") || !strings.Contains(key, "/tenants/tenant_one/") {
		t.Fatalf("unexpected document key: %q", key)
	}

	imageKey := imageObjectKey(&models.DocumentImage{ID: "../image", DocumentID: doc.ID, ProjectKey: doc.ProjectKey, TenantKey: doc.TenantKey, PageNumber: 2, Format: "jpg"})
	if strings.Contains(imageKey, "..") || strings.Contains(imageKey, "\\") {
		t.Fatalf("unsafe image key: %q", imageKey)
	}
}

func TestS3ReferenceOnlyAcceptsConfiguredBucket(t *testing.T) {
	store := &S3Storage{bucket: "expected"}
	key, err := store.keyFromReference("s3://expected/projects/default/document.pdf")
	if err != nil || key != "projects/default/document.pdf" {
		t.Fatalf("key=%q err=%v", key, err)
	}
	if _, err := store.keyFromReference("s3://other/private.pdf"); err == nil {
		t.Fatal("expected foreign bucket reference to fail")
	}
}

func TestIsS3NotFoundDoesNotHidePermissionErrors(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}
	if !isS3NotFound(notFound) {
		t.Fatal("NoSuchKey should be classified as not found")
	}
	forbidden := &smithy.GenericAPIError{Code: "AccessDenied", Message: "denied"}
	if isS3NotFound(forbidden) {
		t.Fatal("AccessDenied must not be classified as not found")
	}
	if isS3NotFound(errors.New("network failure")) {
		t.Fatal("network failures must not be classified as not found")
	}
}
