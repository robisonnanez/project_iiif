package services

import (
	"path/filepath"
	"testing"

	"github.com/gen2brain/go-fitz"
)

func TestExtractPDFOutlineFromFixtures(t *testing.T) {
	t.Run("without TOC", func(t *testing.T) {
		doc, err := fitz.New(filepath.Join("testdata", "without_toc.pdf"))
		if err != nil {
			t.Fatal(err)
		}
		defer doc.Close()
		if outline := extractPDFOutline(doc, doc.NumPage()); len(outline) != 0 {
			t.Fatalf("outline=%v, want none", outline)
		}
	})

	t.Run("hierarchical TOC", func(t *testing.T) {
		doc, err := fitz.New(filepath.Join("testdata", "hierarchical_toc.pdf"))
		if err != nil {
			t.Fatal(err)
		}
		defer doc.Close()
		outline := extractPDFOutline(doc, doc.NumPage())
		if len(outline) != 6 {
			t.Fatalf("outline=%v", outline)
		}
		checks := []struct {
			index int
			level int
			title string
			page  int
		}{
			{0, 1, "Chapter 1", 1},
			{1, 2, "Introduction", 2},
			{4, 1, "Chapter 2", 5},
			{5, 2, "Results", 6},
		}
		for _, check := range checks {
			item := outline[check.index]
			if item.Level != check.level || item.Title != check.title || item.PageNumber != check.page {
				t.Fatalf("outline[%d]=%#v", check.index, item)
			}
		}
	})
}
