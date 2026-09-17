package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	input := "CREATE TABLE first_table (id INT);\n\nCREATE TABLE second_table (id INT);\n"
	want := []string{
		"CREATE TABLE first_table (id INT)",
		"CREATE TABLE second_table (id INT)",
	}
	if got := splitSQLStatements(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSQLStatements() = %#v, want %#v", got, want)
	}
}

func TestAnnotationMigrationsContainRequiredConstraints(t *testing.T) {
	files := []string{
		filepath.Join("..", "..", "migrations", "007_create_document_annotations.sql"),
		filepath.Join("..", "..", "migrations", "postgres", "007_create_document_annotations.sql"),
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			contents, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			sql := strings.ToLower(string(contents))
			for _, expected := range []string{"create table document_annotations", "foreign key", "on delete restrict", "tenant_id", "client_reference_id", "selector", "deleted_at", "request_fingerprint"} {
				if !strings.Contains(sql, expected) {
					t.Errorf("missing %q", expected)
				}
			}
			if !strings.Contains(sql, "uq_document_annotations_tenant_client") {
				t.Error("missing idempotency unique constraint")
			}
			for _, index := range []string{"idx_document_annotations_scope_page", "idx_document_annotations_generation", "idx_document_annotations_deleted_at"} {
				if !strings.Contains(sql, index) {
					t.Errorf("missing index %s", index)
				}
			}
		})
	}
}
