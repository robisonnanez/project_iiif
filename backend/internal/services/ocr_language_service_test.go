package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"iiif-pdf-server/internal/config"
)

type recordedLanguageCommand struct {
	name string
	args []string
}

type fakeLanguageRunner struct {
	installed string
	available string
	commands  []recordedLanguageCommand
	fail      bool
}

func (f *fakeLanguageRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.commands = append(f.commands, recordedLanguageCommand{name: name, args: append([]string(nil), args...)})
	if name == "tesseract" {
		return []byte(f.installed), nil, nil
	}
	if name == "apt-cache" {
		return []byte(f.available), nil, nil
	}
	if f.fail {
		return nil, []byte("falló el helper"), errors.New("exit 1")
	}
	if name == "sudo" && len(args) == 3 {
		f.installed += "\n" + args[2]
	}
	return []byte("ok"), nil, nil
}

func TestOCRLanguageCatalogUsesSystemSources(t *testing.T) {
	cfg := config.Default()
	cfg.OCR.CandidateLanguages = []string{"spa"}
	runner := &fakeLanguageRunner{
		installed: "List of available languages (3):\neng\nspa\nosd\n",
		available: "tesseract-ocr-spa\ntesseract-ocr-deu\ntesseract-ocr-chi-sim\ntesseract-ocr-all\ntesseract-ocr-dev\n",
	}
	service := NewOCRLanguageService(cfg)
	service.runner = runner
	catalog, err := service.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Installed) != 3 || len(catalog.Available) != 2 {
		t.Fatalf("catálogo inesperado: %+v", catalog)
	}
	if catalog.Available[0].Code != "deu" && catalog.Available[1].Code != "deu" {
		t.Fatalf("no se descubrió alemán: %+v", catalog.Available)
	}
	if !catalog.Installed[2].Enabled && !catalog.Installed[1].Enabled && !catalog.Installed[0].Enabled {
		t.Fatal("spa debería aparecer habilitado")
	}
}

func TestInstallOCRLanguagesUsesRestrictedHelperAndVerifies(t *testing.T) {
	cfg := config.Default()
	cfg.OCR.LanguageInstallation.Enabled = true
	cfg.OCR.LanguageInstallation.HelperPath = "/usr/local/sbin/helper-seguro"
	runner := &fakeLanguageRunner{installed: "eng\n", available: "tesseract-ocr-deu\n"}
	service := NewOCRLanguageService(cfg)
	service.runner = runner
	response, err := service.Install(context.Background(), []string{"deu", "deu"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(response.Installed, []string{"deu"}) {
		t.Fatalf("resultado inesperado: %+v", response)
	}
	var sudo recordedLanguageCommand
	for _, command := range runner.commands {
		if command.name == "sudo" {
			sudo = command
		}
	}
	if sudo.name != "sudo" || !reflect.DeepEqual(sudo.args, []string{"-n", "/usr/local/sbin/helper-seguro", "deu"}) {
		t.Fatalf("comando inseguro o inesperado: %+v", sudo)
	}
}

func TestInstallOCRLanguagesRejectsArbitraryInput(t *testing.T) {
	for _, input := range []string{"deu;id", "../../etc/passwd", "", "DE U"} {
		if _, err := normalizeRequestedLanguages([]string{input}); err == nil {
			t.Fatalf("se aceptó entrada insegura %q", input)
		}
	}
	values := make([]string, 11)
	for index := range values {
		values[index] = "deu"
	}
	if _, err := normalizeRequestedLanguages(values); err == nil || !strings.Contains(err.Error(), "1 y 10") {
		t.Fatalf("se esperaba rechazo por límite, err=%v", err)
	}
}
