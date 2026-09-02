package services

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/storage"

	"github.com/gen2brain/go-fitz"
	"github.com/google/uuid"
	"github.com/pemistahl/lingua-go"
	"golang.org/x/text/unicode/norm"
)

const ocrSchemaVersion = 2

const ocrVocabularySchemaVersion = 1

type OCRBoundingBox struct {
	X0 int `json:"x0" example:"940"`
	X1 int `json:"x1" example:"1016"`
	Y0 int `json:"y0" example:"1543"`
	Y1 int `json:"y1" example:"1557"`
}

type OCRWord struct {
	Text       string         `json:"text" example:"SÁNCHEZ,"`
	Confidence float64        `json:"confidence" example:"95.20067596435548"`
	BBox       OCRBoundingBox `json:"bbox"`
}

func (word *OCRWord) UnmarshalJSON(data []byte) error {
	var value struct {
		Text       string          `json:"text"`
		Confidence float64         `json:"confidence"`
		BBox       *OCRBoundingBox `json:"bbox"`
		Left       *int            `json:"left"`
		Top        *int            `json:"top"`
		Width      *int            `json:"width"`
		Height     *int            `json:"height"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	word.Text = value.Text
	word.Confidence = value.Confidence
	if value.BBox != nil {
		word.BBox = *value.BBox
		return nil
	}
	if value.Left != nil && value.Top != nil && value.Width != nil && value.Height != nil {
		word.BBox = OCRBoundingBox{X0: *value.Left, X1: *value.Left + *value.Width, Y0: *value.Top, Y1: *value.Top + *value.Height}
	}
	return nil
}

type OCRPage struct {
	SchemaVersion  int       `json:"schema_version"`
	DocumentID     string    `json:"document_id"`
	Generation     string    `json:"generation"`
	PageNumber     int       `json:"page_number"`
	CanvasV2       string    `json:"canvas_v2"`
	CanvasV3       string    `json:"canvas_v3"`
	ImageID        string    `json:"image_id,omitempty"`
	IIIFImage      string    `json:"iiif_image,omitempty"`
	Status         string    `json:"status"`
	Source         string    `json:"source"`
	Language       string    `json:"language"`
	Confidence     float64   `json:"confidence"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	OCRImageWidth  int       `json:"ocr_image_width,omitempty"`
	OCRImageHeight int       `json:"ocr_image_height,omitempty"`
	Text           string    `json:"text"`
	NativeText     string    `json:"native_text,omitempty"`
	OCRText        string    `json:"ocr_text,omitempty"`
	SearchText     string    `json:"-"`
	GeometryStatus string    `json:"geometry_status"`
	GeometrySpace  string    `json:"geometry_space,omitempty"`
	GeometryError  string    `json:"geometry_error,omitempty"`
	Words          []OCRWord `json:"words,omitempty"`
	Engine         string    `json:"engine"`
	CreatedAt      time.Time `json:"created_at"`
	Error          string    `json:"error,omitempty"`
}

type OCRJob struct {
	ID              string     `json:"id"`
	DocumentID      string     `json:"document_id"`
	ProjectKey      string     `json:"project_key"`
	TenantKey       string     `json:"tenant_key,omitempty"`
	Generation      string     `json:"generation"`
	Mode            string     `json:"mode"`
	LanguageMode    string     `json:"language_mode"`
	Languages       []string   `json:"languages"`
	Status          string     `json:"status"`
	TotalPages      int        `json:"total_pages"`
	ProcessedPages  int        `json:"processed_pages"`
	FailedPages     int        `json:"failed_pages"`
	CurrentPage     int        `json:"current_page,omitempty"`
	CancelRequested bool       `json:"cancel_requested"`
	Error           string     `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type OCRDocumentSummary struct {
	DocumentID       string    `json:"document_id"`
	ProjectKey       string    `json:"project_key"`
	TenantKey        string    `json:"tenant_key,omitempty"`
	ActiveGeneration string    `json:"active_generation"`
	Status           string    `json:"status"`
	Languages        []string  `json:"languages"`
	TotalPages       int       `json:"total_pages"`
	IndexedPages     int       `json:"indexed_pages"`
	FailedPages      int       `json:"failed_pages"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type OCRSearchResult struct {
	DocumentID string  `json:"document_id"`
	PageNumber int     `json:"page_number"`
	CanvasV2   string  `json:"canvas_v2"`
	CanvasV3   string  `json:"canvas_v3"`
	ImageID    string  `json:"image_id,omitempty"`
	IIIFImage  string  `json:"iiif_image,omitempty"`
	Source     string  `json:"source"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
	Matches    int     `json:"matches"`
}

type OCRAutocompleteItem struct {
	Text      string `json:"text"`
	Frequency int    `json:"frequency"`
}

type OCRAutocompleteResponse struct {
	Query string                `json:"query"`
	Items []OCRAutocompleteItem `json:"items"`
}

type ocrVocabularyEntry struct {
	Text       string `json:"text"`
	Normalized string `json:"normalized"`
	Frequency  int    `json:"frequency"`
}

type ocrVocabulary struct {
	SchemaVersion int                  `json:"schema_version"`
	DocumentID    string               `json:"document_id"`
	Generation    string               `json:"generation"`
	Entries       []ocrVocabularyEntry `json:"entries"`
}

type CreateOCRJobRequest struct {
	Mode         string   `json:"mode"`
	LanguageMode string   `json:"language_mode"`
	Languages    []string `json:"languages"`
	Force        bool     `json:"force"`
}

type OCREngine interface {
	Recognize(context.Context, string, []string) (string, []OCRWord, float64, error)
}

type TesseractEngine struct{}

func (TesseractEngine) Recognize(ctx context.Context, imagePath string, languages []string) (string, []OCRWord, float64, error) {
	if len(languages) == 0 {
		languages = []string{"spa"}
	}
	cmd := exec.CommandContext(ctx, "tesseract", imagePath, "stdout", "-l", strings.Join(languages, "+"), "--psm", "3", "tsv")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, 0, ctx.Err()
		}
		return "", nil, 0, fmt.Errorf("tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseTesseractTSV(out)
}

func parseTesseractTSV(data []byte) (string, []OCRWord, float64, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	header, err := readTesseractTSVHeader(reader)
	if err != nil {
		return "", nil, 0, err
	}
	words := make([]OCRWord, 0)
	var text strings.Builder
	confidenceTotal := 0.0
	for {
		fields, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil || !recordHasColumns(fields, header) {
			continue
		}
		level, parseErr := strconv.Atoi(fields[header["level"]])
		if parseErr != nil || level != 5 {
			continue
		}
		wordText := fields[header["text"]]
		if strings.TrimSpace(wordText) == "" {
			continue
		}
		confidence, parseErr := strconv.ParseFloat(fields[header["conf"]], 64)
		if parseErr != nil || confidence < 0 {
			continue
		}
		left, leftErr := strconv.Atoi(fields[header["left"]])
		top, topErr := strconv.Atoi(fields[header["top"]])
		width, widthErr := strconv.Atoi(fields[header["width"]])
		height, heightErr := strconv.Atoi(fields[header["height"]])
		if leftErr != nil || topErr != nil || widthErr != nil || heightErr != nil || left < 0 || top < 0 || width <= 0 || height <= 0 {
			continue
		}
		word := OCRWord{Text: wordText, Confidence: confidence, BBox: OCRBoundingBox{X0: left, X1: left + width, Y0: top, Y1: top + height}}
		words = append(words, word)
		if text.Len() > 0 {
			text.WriteByte(' ')
		}
		text.WriteString(wordText)
		confidenceTotal += confidence
	}
	confidence := 0.0
	if len(words) > 0 {
		confidence = confidenceTotal / float64(len(words))
	}
	return strings.TrimSpace(text.String()), words, confidence, nil
}

func readTesseractTSVHeader(reader *csv.Reader) (map[string]int, error) {
	required := []string{"level", "left", "top", "width", "height", "conf", "text"}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return nil, errors.New("TSV de Tesseract sin encabezado")
		}
		if err != nil || len(record) == 0 {
			continue
		}
		header := make(map[string]int, len(record))
		for index, field := range record {
			header[strings.TrimSpace(strings.TrimPrefix(field, "\ufeff"))] = index
		}
		valid := true
		for _, field := range required {
			if _, ok := header[field]; !ok {
				valid = false
				break
			}
		}
		if valid {
			return header, nil
		}
	}
}

func recordHasColumns(record []string, header map[string]int) bool {
	for _, index := range header {
		if index >= len(record) {
			return false
		}
	}
	return true
}

type OCRService struct {
	config       *config.Config
	storage      storage.Storage
	engine       OCREngine
	root         string
	queue        chan string
	mu           sync.RWMutex
	jobs         map[string]*OCRJob
	cancels      map[string]context.CancelFunc
	vocabularyMu sync.RWMutex
	vocabularies map[string][]ocrVocabularyEntry
}

func NewOCRService(cfg *config.Config, store storage.Storage) (*OCRService, error) {
	service := &OCRService{config: cfg, storage: store, engine: TesseractEngine{}, root: filepath.Join(cfg.Storage.DataPath, "ocr"), jobs: map[string]*OCRJob{}, cancels: map[string]context.CancelFunc{}, vocabularies: map[string][]ocrVocabularyEntry{}}
	if err := os.MkdirAll(filepath.Join(service.root, "jobs"), 0755); err != nil {
		return nil, err
	}
	service.queue = make(chan string, maxInt(16, cfg.OCR.Workers*4))
	if err := service.loadJobs(); err != nil {
		return nil, err
	}
	if cfg.OCR.Enabled {
		for worker := 0; worker < cfg.OCR.Workers; worker++ {
			go service.worker()
		}
		for id, job := range service.jobs {
			if job.Status == "queued" || job.Status == "processing" || job.Status == "detecting_language" || job.Status == "indexing" {
				job.Status = "queued"
				job.Error = ""
				_ = service.saveJob(job)
				service.queue <- id
			}
		}
	}
	return service, nil
}

func (s *OCRService) Enabled() bool { return s.config.OCR.Enabled }

func (s *OCRService) CreateJob(documentID string, request CreateOCRJobRequest) (*OCRJob, error) {
	if !s.Enabled() {
		return nil, errors.New("OCR está desactivado en config.yaml")
	}
	doc, err := s.storage.GetDocument(documentID)
	if err != nil {
		return nil, err
	}
	if doc.Status != "completed" {
		return nil, errors.New("el documento debe terminar su conversión antes de ejecutar OCR")
	}
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode == "" {
		mode = s.config.OCR.DefaultMode
	}
	if mode != "hybrid" && mode != "exhaustive" && mode != "ocr_only" {
		return nil, errors.New("mode debe ser hybrid, exhaustive u ocr_only")
	}
	languageMode := strings.ToLower(strings.TrimSpace(request.LanguageMode))
	if languageMode == "" {
		languageMode = "auto"
	}
	if languageMode != "auto" && languageMode != "manual" {
		return nil, errors.New("language_mode debe ser auto o manual")
	}
	languages := sanitizeLanguages(request.Languages, s.config.OCR.CandidateLanguages)
	if languageMode == "manual" && len(languages) == 0 {
		return nil, errors.New("seleccione al menos un idioma")
	}
	if !request.Force {
		if summary, err := s.GetSummary(documentID); err == nil && summary.Status == "completed" {
			return nil, errors.New("el documento ya tiene OCR activo; use force para crear una nueva generación")
		}
	}
	now := time.Now().UTC()
	job := &OCRJob{ID: uuid.NewString(), DocumentID: documentID, ProjectKey: doc.ProjectKey, TenantKey: doc.TenantKey, Generation: uuid.NewString(), Mode: mode, LanguageMode: languageMode, Languages: languages, Status: "queued", TotalPages: doc.TotalPages, CreatedAt: now}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	if err := s.saveJob(job); err != nil {
		return nil, err
	}
	s.queue <- job.ID
	return cloneJob(job), nil
}

func (s *OCRService) GetJob(id string) (*OCRJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, errors.New("trabajo OCR no encontrado")
	}
	return cloneJob(job), nil
}

func (s *OCRService) CancelJob(id string) (*OCRJob, error) {
	s.mu.Lock()
	job, ok := s.jobs[id]
	if !ok {
		s.mu.Unlock()
		return nil, errors.New("trabajo OCR no encontrado")
	}
	if job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled" {
		result := cloneJob(job)
		s.mu.Unlock()
		return result, nil
	}
	job.CancelRequested = true
	job.Status = "cancelling"
	cancel := s.cancels[id]
	result := cloneJob(job)
	s.mu.Unlock()
	_ = s.saveJob(result)
	if cancel != nil {
		cancel()
	}
	return result, nil
}

func (s *OCRService) worker() {
	for id := range s.queue {
		s.processJob(id)
	}
}

func (s *OCRService) processJob(id string) {
	s.mu.Lock()
	job, ok := s.jobs[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	if job.CancelRequested {
		job.Status = "cancelled"
		now := time.Now().UTC()
		job.FinishedAt = &now
		copy := cloneJob(job)
		s.mu.Unlock()
		_ = s.saveJob(copy)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancels[id] = cancel
	now := time.Now().UTC()
	job.StartedAt = &now
	job.Status = "detecting_language"
	copy := cloneJob(job)
	s.mu.Unlock()
	_ = s.saveJob(copy)
	defer func() { cancel(); s.mu.Lock(); delete(s.cancels, id); s.mu.Unlock() }()
	reader, ok := s.storage.(storage.DocumentPDFReader)
	if !ok {
		s.failJob(id, errors.New("el almacenamiento no permite leer el PDF original"))
		return
	}
	asset, err := reader.GetDocumentPDFData(job.DocumentID)
	if err != nil {
		s.failJob(id, err)
		return
	}
	document, err := fitz.NewFromMemory(asset.Data)
	if err != nil {
		s.failJob(id, fmt.Errorf("abrir PDF: %w", err))
		return
	}
	defer document.Close()
	pageCount := document.NumPage()
	s.updateJob(id, func(j *OCRJob) { j.TotalPages = pageCount })
	languages := job.Languages
	if job.LanguageMode == "auto" {
		languages = s.detectLanguages(document)
		s.updateJob(id, func(j *OCRJob) { j.Languages = languages })
	}
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		if ctx.Err() != nil || s.cancelRequested(id) {
			s.finishCancelled(id)
			return
		}
		s.updateJob(id, func(j *OCRJob) { j.Status = "processing"; j.CurrentPage = pageIndex + 1 })
		page, pageErr := s.processPage(ctx, document, job, pageIndex, languages)
		if pageErr != nil {
			page = &OCRPage{SchemaVersion: ocrSchemaVersion, DocumentID: job.DocumentID, Generation: job.Generation, PageNumber: pageIndex + 1, Status: "failed", Source: "blank", Language: strings.Join(languages, "+"), Engine: "tesseract-cli", CreatedAt: time.Now().UTC(), Error: pageErr.Error()}
		}
		if err := s.savePage(page); err != nil {
			pageErr = err
		}
		s.updateJob(id, func(j *OCRJob) {
			j.ProcessedPages++
			if pageErr != nil {
				j.FailedPages++
			}
		})
	}
	s.updateJob(id, func(j *OCRJob) { j.Status = "indexing" })
	current, _ := s.GetJob(id)
	status := "completed"
	if current.FailedPages > 0 {
		status = "completed_with_errors"
	}
	summary := &OCRDocumentSummary{DocumentID: job.DocumentID, ProjectKey: job.ProjectKey, TenantKey: job.TenantKey, ActiveGeneration: job.Generation, Status: status, Languages: languages, TotalPages: pageCount, IndexedPages: pageCount - current.FailedPages, FailedPages: current.FailedPages, UpdatedAt: time.Now().UTC()}
	vocabulary, err := s.buildVocabulary(job.DocumentID, job.Generation, pageCount)
	if err != nil {
		s.failJob(id, err)
		return
	}
	if err := s.saveVocabulary(vocabulary); err != nil {
		s.failJob(id, err)
		return
	}
	if err := s.saveSummary(summary); err != nil {
		s.failJob(id, err)
		return
	}
	s.updateJob(id, func(j *OCRJob) { j.Status = status; now := time.Now().UTC(); j.FinishedAt = &now; j.CurrentPage = 0 })
}

func (s *OCRService) processPage(parent context.Context, document *fitz.Document, job *OCRJob, pageIndex int, languages []string) (*OCRPage, error) {
	startedAt := time.Now()
	nativeText, _ := document.Text(pageIndex)
	nativeText = cleanText(nativeText)
	image, err := document.ImageDPI(pageIndex, float64(s.config.OCR.RenderDPI))
	if err != nil {
		return nil, fmt.Errorf("render página %d: %w", pageIndex+1, err)
	}
	bounds := image.Bounds()
	ocrWidth, ocrHeight := bounds.Dx(), bounds.Dy()
	imageID, iiifImage, canvasWidth, canvasHeight := s.imageDetails(job.DocumentID, pageIndex+1)
	if canvasWidth <= 0 || canvasHeight <= 0 {
		canvasWidth, canvasHeight = ocrWidth, ocrHeight
	}
	page := &OCRPage{SchemaVersion: ocrSchemaVersion, DocumentID: job.DocumentID, Generation: job.Generation, PageNumber: pageIndex + 1, CanvasV2: fmt.Sprintf("%s/api/iiif/%s/canvases/%s_%04d", strings.TrimRight(s.config.IIIF.BaseURL, "/"), job.DocumentID, job.DocumentID, pageIndex+1), CanvasV3: fmt.Sprintf("%s/api/iiif/%s/canvas/%d", strings.TrimRight(s.config.IIIF.BaseURL, "/"), job.DocumentID, pageIndex+1), ImageID: imageID, IIIFImage: iiifImage, Status: "indexed", Language: strings.Join(languages, "+"), Width: canvasWidth, Height: canvasHeight, OCRImageWidth: ocrWidth, OCRImageHeight: ocrHeight, NativeText: nativeText, GeometryStatus: "page_only", Engine: "mupdf+tesseract-cli", CreatedAt: time.Now().UTC()}
	hasNativeText := usefulRunes(nativeText) >= s.config.OCR.MinTextChars
	temp, err := os.CreateTemp(s.config.PDF.TempPath, fmt.Sprintf("ocr-%s-%06d-*.png", job.DocumentID, pageIndex+1))
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := png.Encode(temp, image); err != nil {
		temp.Close()
		return nil, err
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	var ocrText string
	var words []OCRWord
	var confidence float64
	attempts := 0
	for attempt := 0; attempt <= s.config.OCR.RetriesPerPage; attempt++ {
		attempts++
		ctx, cancel := context.WithTimeout(parent, time.Duration(s.config.OCR.PageTimeoutSeconds)*time.Second)
		ocrText, words, confidence, err = s.engine.Recognize(ctx, tempPath, languages)
		cancel()
		if err == nil {
			break
		}
		if parent.Err() != nil {
			return nil, parent.Err()
		}
	}
	if err != nil {
		if job.Mode == "hybrid" && hasNativeText {
			page.Source = "text_layer"
			page.Text = nativeText
			page.GeometryError = err.Error()
			page.SearchText = normalizeSearch(page.Text)
			log.Printf("OCR page document=%s page=%d language=%s words=0 geometry=%s duration=%s tesseract_calls=%d error=%q", job.DocumentID, pageIndex+1, page.Language, page.GeometryStatus, time.Since(startedAt).Round(time.Millisecond), attempts, err.Error())
			return page, nil
		}
		return nil, err
	}
	page.OCRText = cleanText(ocrText)
	page.Words = scaleOCRWords(words, ocrWidth, ocrHeight, canvasWidth, canvasHeight)
	page.Confidence = confidence
	if len(page.Words) > 0 {
		page.GeometryStatus = "word"
		page.GeometrySpace = "canvas"
	}
	switch job.Mode {
	case "exhaustive":
		page.Source = "mixed"
		page.Text = strings.TrimSpace(nativeText + "\n" + page.OCRText)
	case "hybrid":
		if hasNativeText {
			page.Source = "text_layer"
			page.Text = nativeText
		} else {
			page.Source = "ocr"
			page.Text = page.OCRText
		}
	default:
		page.Source = "ocr"
		page.Text = page.OCRText
	}
	if page.Text == "" {
		page.Status = "blank"
		page.Source = "blank"
	}
	page.SearchText = normalizeSearch(page.Text)
	log.Printf("OCR page document=%s page=%d language=%s words=%d geometry=%s duration=%s tesseract_calls=%d ocr_image=%dx%d canvas=%dx%d", job.DocumentID, pageIndex+1, page.Language, len(page.Words), page.GeometryStatus, time.Since(startedAt).Round(time.Millisecond), attempts, ocrWidth, ocrHeight, canvasWidth, canvasHeight)
	return page, nil
}

func (s *OCRService) detectLanguages(document *fitz.Document) []string {
	var sample strings.Builder
	limit := minInt(document.NumPage(), s.config.OCR.LanguageDetection.SamplePages)
	for page := 0; page < limit; page++ {
		text, _ := document.Text(page)
		if usefulRunes(text) > 0 {
			sample.WriteString(text)
			sample.WriteByte('\n')
		}
	}
	if usefulRunes(sample.String()) < s.config.OCR.LanguageDetection.MinSampleChars {
		// Scanned documents do not have a text layer. A short preliminary pass
		// with all installed candidates gives Lingua enough text to decide.
		for page := 0; page < minInt(document.NumPage(), s.config.OCR.LanguageDetection.SamplePages); page++ {
			image, err := document.ImageDPI(page, float64(s.config.OCR.RenderDPI))
			if err != nil {
				continue
			}
			temp, err := os.CreateTemp(s.config.PDF.TempPath, "ocr-language-*.png")
			if err != nil {
				continue
			}
			path := temp.Name()
			if png.Encode(temp, image) == nil && temp.Close() == nil {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.OCR.PageTimeoutSeconds)*time.Second)
				text, _, _, recognizeErr := s.engine.Recognize(ctx, path, s.config.OCR.CandidateLanguages)
				cancel()
				if recognizeErr == nil {
					sample.WriteString(text)
					sample.WriteByte('\n')
				}
			} else {
				_ = temp.Close()
			}
			_ = os.Remove(path)
			if usefulRunes(sample.String()) >= s.config.OCR.LanguageDetection.MinSampleChars {
				break
			}
		}
	}
	if usefulRunes(sample.String()) < s.config.OCR.LanguageDetection.MinSampleChars {
		return append([]string(nil), s.config.OCR.FallbackLanguages...)
	}
	detectorLanguages := linguaLanguages(s.config.OCR.CandidateLanguages)
	if len(detectorLanguages) == 0 {
		return append([]string(nil), s.config.OCR.FallbackLanguages...)
	}
	if len(detectorLanguages) == 1 {
		return []string{tesseractLanguage(detectorLanguages[0])}
	}
	detector := lingua.NewLanguageDetectorBuilder().FromLanguages(detectorLanguages...).Build()
	values := detector.ComputeLanguageConfidenceValues(sample.String())
	if len(values) == 0 || values[0].Value() < s.config.OCR.LanguageDetection.MinimumConfidence {
		return append([]string(nil), s.config.OCR.FallbackLanguages...)
	}
	result := []string{tesseractLanguage(values[0].Language())}
	if s.config.OCR.LanguageDetection.MaxLanguages > 1 && len(values) > 1 && values[1].Value() >= 0.25 && values[0].Value()-values[1].Value() <= 0.40 {
		result = append(result, tesseractLanguage(values[1].Language()))
	}
	return result
}

func tesseractLanguage(language lingua.Language) string {
	return strings.ToLower(language.IsoCode639_3().String())
}

func linguaLanguages(codes []string) []lingua.Language {
	requested := stringSet(codes)
	result := make([]lingua.Language, 0, len(requested))
	for _, language := range lingua.AllLanguages() {
		if requested[tesseractLanguage(language)] {
			result = append(result, language)
		}
	}
	return result
}

func (s *OCRService) GetSummary(documentID string) (*OCRDocumentSummary, error) {
	data, err := os.ReadFile(filepath.Join(s.root, "documents", documentID+".json"))
	if err != nil {
		return nil, errors.New("el documento aún no tiene OCR")
	}
	var summary OCRDocumentSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *OCRService) GetPage(documentID string, page int) (*OCRPage, error) {
	summary, err := s.GetSummary(documentID)
	if err != nil {
		return nil, err
	}
	result, err := s.readPage(documentID, summary.ActiveGeneration, page)
	if err != nil {
		return nil, err
	}
	imageID, iiifImage, canvasWidth, canvasHeight := s.imageDetails(documentID, page)
	if result.ImageID == "" {
		result.ImageID, result.IIIFImage = imageID, iiifImage
	}
	if len(result.Words) > 0 && result.GeometrySpace == "" && result.Width > 0 && result.Height > 0 && canvasWidth > 0 && canvasHeight > 0 {
		result.OCRImageWidth, result.OCRImageHeight = result.Width, result.Height
		result.Words = scaleOCRWords(result.Words, result.Width, result.Height, canvasWidth, canvasHeight)
		result.Width, result.Height = canvasWidth, canvasHeight
		result.GeometrySpace = "canvas"
	}
	return result, nil
}

func (s *OCRService) Search(query, project, tenant, documentID string, limit, offset int) ([]OCRSearchResult, int, error) {
	needle := normalizeSearch(query)
	if utf8.RuneCountInString(needle) < 2 {
		return nil, 0, errors.New("la consulta debe tener al menos 2 caracteres")
	}
	docs, err := s.storage.GetDocumentsByScope(project, tenant)
	if err != nil {
		return nil, 0, err
	}
	results := make([]OCRSearchResult, 0)
	for _, doc := range docs {
		if documentID != "" && doc.ID != documentID {
			continue
		}
		summary, err := s.GetSummary(doc.ID)
		if err != nil {
			continue
		}
		for page := 1; page <= summary.TotalPages; page++ {
			item, err := s.readPage(doc.ID, summary.ActiveGeneration, page)
			if err != nil {
				continue
			}
			haystack := normalizeSearch(item.Text)
			matches := strings.Count(haystack, needle)
			if matches == 0 {
				continue
			}
			imageID, iiifImage := item.ImageID, item.IIIFImage
			if imageID == "" {
				imageID, iiifImage = s.imageReference(doc.ID, page)
			}
			results = append(results, OCRSearchResult{DocumentID: doc.ID, PageNumber: page, CanvasV2: item.CanvasV2, CanvasV3: item.CanvasV3, ImageID: imageID, IIIFImage: iiifImage, Source: item.Source, Snippet: makeSnippet(item.Text, needle), Score: float64(matches), Matches: matches})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].DocumentID == results[j].DocumentID {
				return results[i].PageNumber < results[j].PageNumber
			}
			return results[i].DocumentID < results[j].DocumentID
		}
		return results[i].Score > results[j].Score
	})
	maxScore := 0.0
	if len(results) > 0 {
		maxScore = results[0].Score
	}
	if maxScore > 0 {
		for index := range results {
			results[index].Score /= maxScore
		}
	}
	total := len(results)
	if offset > total {
		offset = total
	}
	end := minInt(total, offset+limit)
	return results[offset:end], total, nil
}

func (s *OCRService) Autocomplete(query, project, tenant, documentID string, limit int) ([]OCRAutocompleteItem, error) {
	prefix := normalizeSearch(query)
	if utf8.RuneCountInString(prefix) < 2 {
		return nil, errors.New("la consulta debe tener al menos 2 caracteres")
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	docs, err := s.storage.GetDocumentsByScope(project, tenant)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		text        string
		frequency   int
		displayFreq int
	}
	candidates := map[string]candidate{}
	for _, doc := range docs {
		if documentID != "" && doc.ID != documentID {
			continue
		}
		summary, err := s.GetSummary(doc.ID)
		if err != nil {
			continue
		}
		entries, err := s.vocabularyForDocument(summary)
		if err != nil {
			continue
		}
		start := sort.Search(len(entries), func(index int) bool { return entries[index].Normalized >= prefix })
		for index := start; index < len(entries) && strings.HasPrefix(entries[index].Normalized, prefix); index++ {
			entry := entries[index]
			current := candidates[entry.Normalized]
			current.frequency += entry.Frequency
			if current.text == "" || entry.Frequency > current.displayFreq || (entry.Frequency == current.displayFreq && betterWordDisplay(entry.Text, current.text)) {
				current.text = entry.Text
				current.displayFreq = entry.Frequency
			}
			candidates[entry.Normalized] = current
		}
	}
	type rankedCandidate struct {
		normalized string
		candidate
	}
	ranked := make([]rankedCandidate, 0, len(candidates))
	for normalized, item := range candidates {
		ranked = append(ranked, rankedCandidate{normalized: normalized, candidate: item})
	}
	sort.Slice(ranked, func(i, j int) bool {
		iExact, jExact := ranked[i].normalized == prefix, ranked[j].normalized == prefix
		if iExact != jExact {
			return iExact
		}
		if ranked[i].frequency != ranked[j].frequency {
			return ranked[i].frequency > ranked[j].frequency
		}
		iLength, jLength := utf8.RuneCountInString(ranked[i].normalized), utf8.RuneCountInString(ranked[j].normalized)
		if iLength != jLength {
			return iLength < jLength
		}
		if ranked[i].normalized != ranked[j].normalized {
			return ranked[i].normalized < ranked[j].normalized
		}
		return ranked[i].text < ranked[j].text
	})
	if limit > len(ranked) {
		limit = len(ranked)
	}
	items := make([]OCRAutocompleteItem, 0, limit)
	for _, item := range ranked[:limit] {
		items = append(items, OCRAutocompleteItem{Text: item.text, Frequency: item.frequency})
	}
	return items, nil
}

func (s *OCRService) imageReference(documentID string, page int) (string, string) {
	imageID, iiifImage, _, _ := s.imageDetails(documentID, page)
	return imageID, iiifImage
}

func (s *OCRService) imageDetails(documentID string, page int) (string, string, int, int) {
	image, err := s.storage.GetDocumentImageByPage(documentID, page)
	if err != nil || image == nil || image.ID == "" {
		return "", "", 0, 0
	}
	base := strings.TrimRight(s.config.IIIF.BaseURL, "/")
	return image.ID, fmt.Sprintf("%s/iiif/%s/%s/full/max/0/default.jpg", base, s.config.IIIF.APIVersion, image.ID), image.Width, image.Height
}

func scaleOCRWords(words []OCRWord, sourceWidth, sourceHeight, targetWidth, targetHeight int) []OCRWord {
	if sourceWidth <= 0 || sourceHeight <= 0 || targetWidth <= 0 || targetHeight <= 0 {
		return append([]OCRWord(nil), words...)
	}
	scaled := make([]OCRWord, 0, len(words))
	for _, word := range words {
		box := word.BBox
		if box.X0 < 0 || box.Y0 < 0 || box.X1 <= box.X0 || box.Y1 <= box.Y0 {
			continue
		}
		x0 := scaleCoordinate(box.X0, sourceWidth, targetWidth)
		x1 := scaleCoordinate(box.X1, sourceWidth, targetWidth)
		y0 := scaleCoordinate(box.Y0, sourceHeight, targetHeight)
		y1 := scaleCoordinate(box.Y1, sourceHeight, targetHeight)
		if x0 >= targetWidth {
			x0 = targetWidth - 1
		}
		if y0 >= targetHeight {
			y0 = targetHeight - 1
		}
		if x1 <= x0 {
			x1 = x0 + 1
		}
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if x1 > targetWidth {
			x1 = targetWidth
		}
		if y1 > targetHeight {
			y1 = targetHeight
		}
		word.BBox = OCRBoundingBox{X0: x0, X1: x1, Y0: y0, Y1: y1}
		scaled = append(scaled, word)
	}
	return scaled
}

func scaleCoordinate(value, sourceSize, targetSize int) int {
	return int(math.Round(float64(value) * float64(targetSize) / float64(sourceSize)))
}

func (s *OCRService) Delete(documentID string) error {
	if strings.TrimSpace(documentID) == "" || filepath.Base(documentID) != documentID {
		return errors.New("identificador de documento inválido")
	}
	if err := os.Remove(filepath.Join(s.root, "documents", documentID+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.root, "pages", documentID)); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.root, "vocabularies", documentID)); err != nil {
		return err
	}
	s.vocabularyMu.Lock()
	for key := range s.vocabularies {
		if strings.HasPrefix(key, documentID+"\x00") {
			delete(s.vocabularies, key)
		}
	}
	s.vocabularyMu.Unlock()
	return nil
}

func (s *OCRService) vocabularyForDocument(summary *OCRDocumentSummary) ([]ocrVocabularyEntry, error) {
	key := vocabularyCacheKey(summary.DocumentID, summary.ActiveGeneration)
	s.vocabularyMu.RLock()
	entries, ok := s.vocabularies[key]
	s.vocabularyMu.RUnlock()
	if ok {
		return entries, nil
	}
	s.vocabularyMu.Lock()
	defer s.vocabularyMu.Unlock()
	if entries, ok = s.vocabularies[key]; ok {
		return entries, nil
	}
	vocabulary, err := s.readVocabulary(summary.DocumentID, summary.ActiveGeneration)
	if os.IsNotExist(err) {
		vocabulary, err = s.buildVocabulary(summary.DocumentID, summary.ActiveGeneration, summary.TotalPages)
		if err == nil {
			err = s.writeVocabulary(vocabulary)
		}
	}
	if err != nil {
		return nil, err
	}
	s.vocabularies[key] = vocabulary.Entries
	return vocabulary.Entries, nil
}

func (s *OCRService) buildVocabulary(documentID, generation string, totalPages int) (*ocrVocabulary, error) {
	type wordCount struct {
		total    int
		variants map[string]int
	}
	words := map[string]*wordCount{}
	for page := 1; page <= totalPages; page++ {
		item, err := s.readPage(documentID, generation, page)
		if err != nil {
			continue
		}
		for _, word := range splitOCRWords(item.Text) {
			normalized := normalizeSearch(word)
			if normalized == "" {
				continue
			}
			count := words[normalized]
			if count == nil {
				count = &wordCount{variants: map[string]int{}}
				words[normalized] = count
			}
			count.total++
			count.variants[word]++
		}
	}
	entries := make([]ocrVocabularyEntry, 0, len(words))
	for normalized, count := range words {
		text, frequency := "", 0
		for variant, variantFrequency := range count.variants {
			if text == "" || variantFrequency > frequency || (variantFrequency == frequency && betterWordDisplay(variant, text)) {
				text, frequency = variant, variantFrequency
			}
		}
		entries = append(entries, ocrVocabularyEntry{Text: text, Normalized: normalized, Frequency: count.total})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Normalized == entries[j].Normalized {
			return entries[i].Text < entries[j].Text
		}
		return entries[i].Normalized < entries[j].Normalized
	})
	return &ocrVocabulary{SchemaVersion: ocrVocabularySchemaVersion, DocumentID: documentID, Generation: generation, Entries: entries}, nil
}

func (s *OCRService) saveVocabulary(vocabulary *ocrVocabulary) error {
	s.vocabularyMu.Lock()
	defer s.vocabularyMu.Unlock()
	if err := s.writeVocabulary(vocabulary); err != nil {
		return err
	}
	s.vocabularies[vocabularyCacheKey(vocabulary.DocumentID, vocabulary.Generation)] = vocabulary.Entries
	return nil
}

func (s *OCRService) writeVocabulary(vocabulary *ocrVocabulary) error {
	path := filepath.Join(s.root, "vocabularies", vocabulary.DocumentID, vocabulary.Generation+".json.gz")
	return writeGzipJSONAtomic(path, vocabulary)
}

func (s *OCRService) readVocabulary(documentID, generation string) (*ocrVocabulary, error) {
	file, err := os.Open(filepath.Join(s.root, "vocabularies", documentID, generation+".json.gz"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var result ocrVocabulary
	if err := json.NewDecoder(reader).Decode(&result); err != nil {
		return nil, err
	}
	if result.SchemaVersion != ocrVocabularySchemaVersion || result.DocumentID != documentID || result.Generation != generation {
		return nil, errors.New("vocabulario OCR incompatible")
	}
	return &result, nil
}

func writeGzipJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".ocr-vocabulary-*.json.gz")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	writer := gzip.NewWriter(temp)
	err = json.NewEncoder(writer).Encode(value)
	closeErr := writer.Close()
	fileErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if fileErr != nil {
		return fileErr
	}
	return os.Rename(name, path)
}

func vocabularyCacheKey(documentID, generation string) string {
	return documentID + "\x00" + generation
}

func splitOCRWords(value string) []string {
	words := make([]string, 0)
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, character := range strings.ToValidUTF8(value, "") {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			current.WriteRune(character)
		} else {
			flush()
		}
	}
	flush()
	return words
}

func betterWordDisplay(candidate, current string) bool {
	candidateLower := candidate == strings.ToLower(candidate)
	currentLower := current == strings.ToLower(current)
	if candidateLower != currentLower {
		return candidateLower
	}
	return candidate < current
}

func (s *OCRService) savePage(page *OCRPage) error {
	path := filepath.Join(s.root, "pages", page.DocumentID, page.Generation, fmt.Sprintf("%06d.json.gz", page.PageNumber))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := gzip.NewWriter(file)
	err = json.NewEncoder(writer).Encode(page)
	closeErr := writer.Close()
	fileErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return fileErr
}
func (s *OCRService) readPage(documentID, generation string, page int) (*OCRPage, error) {
	file, err := os.Open(filepath.Join(s.root, "pages", documentID, generation, fmt.Sprintf("%06d.json.gz", page)))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var result OCRPage
	if err := json.NewDecoder(reader).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (s *OCRService) saveSummary(summary *OCRDocumentSummary) error {
	path := filepath.Join(s.root, "documents", summary.DocumentID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeJSONAtomic(path, summary)
}
func (s *OCRService) saveJob(job *OCRJob) error {
	return writeJSONAtomic(filepath.Join(s.root, "jobs", job.ID+".json"), job)
}
func (s *OCRService) loadJobs() error {
	entries, err := os.ReadDir(filepath.Join(s.root, "jobs"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.root, "jobs", entry.Name()))
		if err != nil {
			continue
		}
		var job OCRJob
		if json.Unmarshal(data, &job) == nil {
			s.jobs[job.ID] = &job
		}
	}
	return nil
}
func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".ocr-*.json")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (s *OCRService) updateJob(id string, change func(*OCRJob)) {
	s.mu.Lock()
	job := s.jobs[id]
	if job == nil {
		s.mu.Unlock()
		return
	}
	change(job)
	copy := cloneJob(job)
	s.mu.Unlock()
	_ = s.saveJob(copy)
}
func (s *OCRService) failJob(id string, err error) {
	s.updateJob(id, func(job *OCRJob) {
		job.Status = "failed"
		job.Error = err.Error()
		now := time.Now().UTC()
		job.FinishedAt = &now
	})
}
func (s *OCRService) finishCancelled(id string) {
	s.updateJob(id, func(job *OCRJob) { job.Status = "cancelled"; now := time.Now().UTC(); job.FinishedAt = &now })
}
func (s *OCRService) cancelRequested(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[id] != nil && s.jobs[id].CancelRequested
}
func cloneJob(job *OCRJob) *OCRJob {
	if job == nil {
		return nil
	}
	copy := *job
	copy.Languages = append([]string(nil), job.Languages...)
	return &copy
}
func sanitizeLanguages(values, allowed []string) []string {
	permitted := map[string]bool{}
	for _, value := range allowed {
		permitted[strings.ToLower(strings.TrimSpace(value))] = true
	}
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if permitted[value] && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func cleanText(value string) string {
	return strings.Join(strings.Fields(strings.ToValidUTF8(value, "")), " ")
}
func usefulRunes(value string) int {
	count := 0
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			count++
		}
	}
	return count
}
func normalizeSearch(value string) string {
	decomposed := norm.NFD.String(strings.ToLower(cleanText(value)))
	var builder strings.Builder
	for _, current := range decomposed {
		if unicode.Is(unicode.Mn, current) {
			continue
		}
		builder.WriteRune(current)
	}
	return norm.NFC.String(builder.String())
}
func makeSnippet(original, normalizedNeedle string) string {
	normalized := normalizeSearch(original)
	index := strings.Index(normalized, normalizedNeedle)
	if index < 0 {
		index = 0
	}
	runes := []rune(original)
	normalizedRunes := []rune(normalized[:index])
	start := len(normalizedRunes) - 70
	if start < 0 {
		start = 0
	}
	end := start + 180
	if end > len(runes) {
		end = len(runes)
	}
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(runes) {
		suffix = "…"
	}
	return prefix + string(runes[start:end]) + suffix
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
