package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"iiif-pdf-server/internal/config"
)

var tesseractLanguageCode = regexp.MustCompile(`^[a-z]{3}(?:_[a-z0-9]+)*$`)

type OCRLanguage struct {
	Code               string `json:"code" example:"deu"`
	Name               string `json:"name" example:"Alemán"`
	Package            string `json:"package,omitempty" example:"tesseract-ocr-deu"`
	Installed          bool   `json:"installed" example:"false"`
	Enabled            bool   `json:"enabled" example:"false"`
	DetectionSupported bool   `json:"detection_supported" example:"true"`
}

type OCRLanguageCatalog struct {
	InstallationEnabled bool          `json:"installation_enabled"`
	Installed           []OCRLanguage `json:"installed"`
	Available           []OCRLanguage `json:"available"`
}

type InstallOCRLanguagesRequest struct {
	Languages []string `json:"languages" binding:"required" example:"deu,ita"`
}

type InstallOCRLanguagesResponse struct {
	Installed []string           `json:"installed"`
	Catalog   OCRLanguageCatalog `json:"catalog"`
}

type externalCommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, []byte, error)
}

type osCommandRunner struct{}

func (osCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type OCRLanguageService struct {
	config *config.Config
	runner externalCommandRunner
	mu     sync.Mutex
}

func NewOCRLanguageService(cfg *config.Config) *OCRLanguageService {
	return &OCRLanguageService{config: cfg, runner: osCommandRunner{}}
}

func (s *OCRLanguageService) Catalog(ctx context.Context) (OCRLanguageCatalog, error) {
	installed, err := s.installed(ctx)
	if err != nil {
		return OCRLanguageCatalog{}, err
	}
	packages, err := s.availablePackages(ctx)
	if err != nil {
		return OCRLanguageCatalog{}, err
	}
	enabled := stringSet(s.config.OCR.CandidateLanguages)
	catalog := OCRLanguageCatalog{InstallationEnabled: s.config.OCR.LanguageInstallation.Enabled}
	for code := range installed {
		catalog.Installed = append(catalog.Installed, languageEntry(code, packages[code], true, enabled[code]))
	}
	for code, packageName := range packages {
		if !installed[code] {
			catalog.Available = append(catalog.Available, languageEntry(code, packageName, false, enabled[code]))
		}
	}
	sortLanguages(catalog.Installed)
	sortLanguages(catalog.Available)
	return catalog, nil
}

func (s *OCRLanguageService) Install(ctx context.Context, requested []string) (InstallOCRLanguagesResponse, error) {
	if !s.config.OCR.LanguageInstallation.Enabled {
		return InstallOCRLanguagesResponse{}, errors.New("la instalación de idiomas OCR está deshabilitada en config.yaml")
	}
	if !s.mu.TryLock() {
		return InstallOCRLanguagesResponse{}, errors.New("ya existe una instalación de idiomas OCR en curso")
	}
	defer s.mu.Unlock()

	codes, err := normalizeRequestedLanguages(requested)
	if err != nil {
		return InstallOCRLanguagesResponse{}, err
	}
	packages, err := s.availablePackages(ctx)
	if err != nil {
		return InstallOCRLanguagesResponse{}, err
	}
	installedBefore, err := s.installed(ctx)
	if err != nil {
		return InstallOCRLanguagesResponse{}, err
	}

	timeout := time.Duration(s.config.OCR.LanguageInstallation.TimeoutSeconds) * time.Second
	for _, code := range codes {
		packageName, available := packages[code]
		if !available {
			return InstallOCRLanguagesResponse{}, fmt.Errorf("el idioma %q no corresponde a un paquete Tesseract disponible", code)
		}
		if installedBefore[code] {
			continue
		}
		installContext, cancel := context.WithTimeout(ctx, timeout)
		_, stderr, runErr := s.runner.Run(installContext, "sudo", "-n", s.config.OCR.LanguageInstallation.HelperPath, code)
		cancel()
		if runErr != nil {
			message := strings.TrimSpace(string(stderr))
			if len(message) > 500 {
				message = message[:500]
			}
			log.Printf("Instalación idioma OCR language=%s package=%s status=error error=%v", code, packageName, runErr)
			return InstallOCRLanguagesResponse{}, fmt.Errorf("no se pudo instalar %s: %s", code, message)
		}
		verified, verifyErr := s.installed(ctx)
		if verifyErr != nil || !verified[code] {
			log.Printf("Instalación idioma OCR language=%s package=%s status=verification_failed", code, packageName)
			return InstallOCRLanguagesResponse{}, fmt.Errorf("Tesseract no reconoció el idioma %s después de instalarlo", code)
		}
		log.Printf("Instalación idioma OCR language=%s package=%s status=installed", code, packageName)
	}

	catalog, err := s.Catalog(ctx)
	if err != nil {
		return InstallOCRLanguagesResponse{}, err
	}
	return InstallOCRLanguagesResponse{Installed: codes, Catalog: catalog}, nil
}

func (s *OCRLanguageService) installed(ctx context.Context) (map[string]bool, error) {
	commandContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	stdout, stderr, err := s.runner.Run(commandContext, "tesseract", "--list-langs")
	if err != nil {
		return nil, fmt.Errorf("no se pudieron consultar los idiomas de Tesseract: %s", strings.TrimSpace(string(stderr)))
	}
	result := map[string]bool{}
	for _, line := range strings.Split(string(stdout), "\n") {
		code := strings.TrimSpace(line)
		if tesseractLanguageCode.MatchString(code) || code == "osd" {
			result[code] = true
		}
	}
	return result, nil
}

func (s *OCRLanguageService) availablePackages(ctx context.Context) (map[string]string, error) {
	commandContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stdout, stderr, err := s.runner.Run(commandContext, "apt-cache", "pkgnames", "tesseract-ocr-")
	if err != nil {
		return nil, fmt.Errorf("no se pudieron consultar los paquetes Tesseract: %s", strings.TrimSpace(string(stderr)))
	}
	result := map[string]string{}
	for _, line := range strings.Split(string(stdout), "\n") {
		packageName := strings.TrimSpace(line)
		if !strings.HasPrefix(packageName, "tesseract-ocr-") {
			continue
		}
		suffix := strings.TrimPrefix(packageName, "tesseract-ocr-")
		code := strings.ReplaceAll(suffix, "-", "_")
		if code == "all" || code == "dev" || !tesseractLanguageCode.MatchString(code) {
			continue
		}
		result[code] = packageName
	}
	return result, nil
}

func normalizeRequestedLanguages(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 10 {
		return nil, errors.New("selecciona entre 1 y 10 idiomas")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		code := strings.ToLower(strings.TrimSpace(value))
		if !tesseractLanguageCode.MatchString(code) {
			return nil, fmt.Errorf("código de idioma inválido: %q", value)
		}
		if !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result, nil
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[strings.ToLower(strings.TrimSpace(value))] = true
	}
	return result
}

func languageEntry(code, packageName string, installed, enabled bool) OCRLanguage {
	return OCRLanguage{Code: code, Name: friendlyTesseractLanguageName(code), Package: packageName, Installed: installed, Enabled: enabled, DetectionSupported: len(linguaLanguages([]string{code})) == 1}
}

func sortLanguages(values []OCRLanguage) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Name == values[j].Name {
			return values[i].Code < values[j].Code
		}
		return values[i].Name < values[j].Name
	})
}

func friendlyTesseractLanguageName(code string) string {
	names := map[string]string{
		"ara": "Árabe", "cat": "Catalán", "deu": "Alemán", "eng": "Inglés",
		"eus": "Euskera", "fra": "Francés", "glg": "Gallego", "ita": "Italiano",
		"jpn": "Japonés", "lat": "Latín", "nld": "Neerlandés", "osd": "Detección de orientación y escritura",
		"por": "Portugués", "rus": "Ruso", "spa": "Español", "ukr": "Ucraniano",
		"chi_sim": "Chino simplificado", "chi_tra": "Chino tradicional",
	}
	if name := names[code]; name != "" {
		return name
	}
	return "Idioma Tesseract"
}
