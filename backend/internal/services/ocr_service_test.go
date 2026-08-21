package services

import (
	"strings"
	"testing"
)

func TestParseTesseractTSV(t *testing.T) {
	input := strings.Join([]string{
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
		"5\t1\t1\t1\t1\t1\t10\t20\t30\t12\t95.5\tHola",
		"5\t1\t1\t1\t1\t2\t45\t20\t50\t12\t84.5\tmundo",
	}, "\n")
	text, words, confidence, err := parseTesseractTSV([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hola mundo" {
		t.Fatalf("text = %q", text)
	}
	if len(words) != 2 || words[0].Left != 10 || words[1].Width != 50 {
		t.Fatalf("words = %#v", words)
	}
	if confidence != 90 {
		t.Fatalf("confidence = %v", confidence)
	}
}

func TestNormalizeSearchPreservesOriginalSemantics(t *testing.T) {
	got := normalizeSearch("  Índices   del\nCAFÉ  ")
	if got != "indices del cafe" {
		t.Fatalf("normalizeSearch() = %q", got)
	}
	if usefulRunes(" -- á1 -- ") != 2 {
		t.Fatalf("usefulRunes() returned unexpected count")
	}
}

func TestSanitizeLanguages(t *testing.T) {
	got := sanitizeLanguages([]string{"SPA", "eng", "spa", "deu"}, []string{"spa", "eng", "fra", "por"})
	if strings.Join(got, ",") != "spa,eng" {
		t.Fatalf("languages = %#v", got)
	}
}
