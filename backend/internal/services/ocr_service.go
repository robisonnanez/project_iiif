package services

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
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

const ocrSchemaVersion = 1

type OCRWord struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Left       int     `json:"left"`
	Top        int     `json:"top"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
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
	Text           string    `json:"text"`
	NativeText     string    `json:"native_text,omitempty"`
	OCRText        string    `json:"ocr_text,omitempty"`
	SearchText     string    `json:"-"`
	GeometryStatus string    `json:"geometry_status"`
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
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	first := true
	words := make([]OCRWord, 0)
	var text strings.Builder
	confidenceTotal := 0.0
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.SplitN(scanner.Text(), "\t", 12)
		if len(fields) < 12 || strings.TrimSpace(fields[11]) == "" {
			continue
		}
		confidence, err := strconv.ParseFloat(fields[10], 64)
		if err != nil || confidence < 0 {
			continue
		}
		left, _ := strconv.Atoi(fields[6])
		top, _ := strconv.Atoi(fields[7])
		width, _ := strconv.Atoi(fields[8])
		height, _ := strconv.Atoi(fields[9])
		word := OCRWord{Text: fields[11], Confidence: confidence, Left: left, Top: top, Width: width, Height: height}
		words = append(words, word)
		if text.Len() > 0 {
			text.WriteByte(' ')
		}
		text.WriteString(word.Text)
		confidenceTotal += confidence
	}
	if err := scanner.Err(); err != nil {
		return "", nil, 0, err
	}
	confidence := 0.0
	if len(words) > 0 {
		confidence = confidenceTotal / float64(len(words))
	}
	return strings.TrimSpace(text.String()), words, confidence, nil
}

type OCRService struct {
	config  *config.Config
	storage storage.Storage
	engine  OCREngine
	root    string
	queue   chan string
	mu      sync.RWMutex
	jobs    map[string]*OCRJob
	cancels map[string]context.CancelFunc
}

func NewOCRService(cfg *config.Config, store storage.Storage) (*OCRService, error) {
	service := &OCRService{config: cfg, storage: store, engine: TesseractEngine{}, root: filepath.Join(cfg.Storage.DataPath, "ocr"), jobs: map[string]*OCRJob{}, cancels: map[string]context.CancelFunc{}}
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
	if err := s.saveSummary(summary); err != nil {
		s.failJob(id, err)
		return
	}
	s.updateJob(id, func(j *OCRJob) { j.Status = status; now := time.Now().UTC(); j.FinishedAt = &now; j.CurrentPage = 0 })
}

func (s *OCRService) processPage(parent context.Context, document *fitz.Document, job *OCRJob, pageIndex int, languages []string) (*OCRPage, error) {
	nativeText, _ := document.Text(pageIndex)
	nativeText = cleanText(nativeText)
	image, err := document.ImageDPI(pageIndex, float64(s.config.OCR.RenderDPI))
	if err != nil {
		return nil, fmt.Errorf("render página %d: %w", pageIndex+1, err)
	}
	bounds := image.Bounds()
	imageID, iiifImage := s.imageReference(job.DocumentID, pageIndex+1)
	page := &OCRPage{SchemaVersion: ocrSchemaVersion, DocumentID: job.DocumentID, Generation: job.Generation, PageNumber: pageIndex + 1, CanvasV2: fmt.Sprintf("%s/api/iiif/%s/canvases/%s_%04d", strings.TrimRight(s.config.IIIF.BaseURL, "/"), job.DocumentID, job.DocumentID, pageIndex+1), CanvasV3: fmt.Sprintf("%s/api/iiif/%s/canvas/%d", strings.TrimRight(s.config.IIIF.BaseURL, "/"), job.DocumentID, pageIndex+1), ImageID: imageID, IIIFImage: iiifImage, Status: "indexed", Language: strings.Join(languages, "+"), Width: bounds.Dx(), Height: bounds.Dy(), NativeText: nativeText, Engine: "mupdf+tesseract-cli", CreatedAt: time.Now().UTC()}
	needsOCR := job.Mode == "exhaustive" || job.Mode == "ocr_only" || usefulRunes(nativeText) < s.config.OCR.MinTextChars
	if !needsOCR {
		page.Source = "text_layer"
		page.Text = nativeText
		page.GeometryStatus = "page_only"
		page.SearchText = normalizeSearch(nativeText)
		if page.Text == "" {
			page.Status = "blank"
			page.Source = "blank"
		}
		return page, nil
	}
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
	for attempt := 0; attempt <= s.config.OCR.RetriesPerPage; attempt++ {
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
		return nil, err
	}
	page.OCRText = cleanText(ocrText)
	page.Words = words
	page.Confidence = confidence
	page.GeometryStatus = "word"
	switch job.Mode {
	case "exhaustive":
		page.Source = "mixed"
		page.Text = strings.TrimSpace(nativeText + "\n" + page.OCRText)
	default:
		page.Source = "ocr"
		page.Text = page.OCRText
	}
	if page.Text == "" {
		page.Status = "blank"
		page.Source = "blank"
	}
	page.SearchText = normalizeSearch(page.Text)
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
	detector := lingua.NewLanguageDetectorBuilder().FromLanguages(lingua.Spanish, lingua.English, lingua.French, lingua.Portuguese).Build()
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
	switch language {
	case lingua.English:
		return "eng"
	case lingua.French:
		return "fra"
	case lingua.Portuguese:
		return "por"
	default:
		return "spa"
	}
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
	if err == nil && result.ImageID == "" {
		result.ImageID, result.IIIFImage = s.imageReference(documentID, page)
	}
	return result, err
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

func (s *OCRService) imageReference(documentID string, page int) (string, string) {
	image, err := s.storage.GetDocumentImageByPage(documentID, page)
	if err != nil || image == nil || image.ID == "" {
		return "", ""
	}
	base := strings.TrimRight(s.config.IIIF.BaseURL, "/")
	return image.ID, fmt.Sprintf("%s/iiif/%s/%s/full/max/0/default.jpg", base, s.config.IIIF.APIVersion, image.ID)
}

func (s *OCRService) Delete(documentID string) error {
	if strings.TrimSpace(documentID) == "" || filepath.Base(documentID) != documentID {
		return errors.New("identificador de documento inválido")
	}
	if err := os.Remove(filepath.Join(s.root, "documents", documentID+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(filepath.Join(s.root, "pages", documentID))
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
