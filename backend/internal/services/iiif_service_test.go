package services

import "testing"

func TestManifestPagesSupportsRangesAndRemovesDuplicates(t *testing.T) {
	pages, normalized, err := manifestPages("5, 1-3, 3", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 5}
	if normalized != "1,2,3,5" || len(pages) != len(want) {
		t.Fatalf("pages=%v normalized=%q", pages, normalized)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Fatalf("pages=%v want=%v", pages, want)
		}
	}
}

func TestManifestPagesDefaultsToAll(t *testing.T) {
	pages, normalized, err := manifestPages("all", 3)
	if err != nil || normalized != "all" || len(pages) != 3 {
		t.Fatalf("pages=%v normalized=%q err=%v", pages, normalized, err)
	}
}

func TestManifestPagesRejectsInvalidSelections(t *testing.T) {
	for _, selection := range []string{"0", "4", "3-2", "a", "1,,2"} {
		if _, _, err := manifestPages(selection, 3); err == nil {
			t.Fatalf("expected %q to fail", selection)
		}
	}
}
