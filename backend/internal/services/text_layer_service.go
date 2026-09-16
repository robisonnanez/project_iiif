package services

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	contract "iiif-pdf-server/internal/api"
)

var (
	ErrTextLayerUnavailable  = errors.New("text layer no disponible")
	ErrTextLayerNotReady     = errors.New("el documento o la generación todavía no está listo")
	ErrOCRGenerationNotFound = errors.New("generación OCR no encontrada")
)

func (s *OCRService) GetTextLayerStatus(documentID string) (*contract.TextLayerStatus, error) {
	document, err := s.storage.GetDocument(documentID)
	if err != nil {
		return nil, ErrOCRDocumentNotFound
	}
	status := &contract.TextLayerStatus{SchemaVersion: contract.TextLayerSchemaVersion, DocumentID: documentID, Status: "unavailable", PagesTotal: document.TotalPages, Geometry: "page_only", UpdatedAt: document.UploadDate}
	if !s.Enabled() {
		return status, nil
	}
	if summary, summaryErr := s.GetSummary(documentID); summaryErr == nil {
		status.Available = summary.IndexedPages > 0
		status.Status = publicOCRStatus(summary.Status)
		status.ActiveGeneration = summary.ActiveGeneration
		status.PagesTotal = summary.TotalPages
		status.PagesReady = summary.IndexedPages
		status.PagesFailed = summary.FailedPages
		status.UpdatedAt = summary.UpdatedAt
		if index, indexErr := s.GetGenerationIndex(documentID); indexErr == nil {
			for _, generation := range index.Generations {
				if generation.ID == summary.ActiveGeneration && generation.Geometry != "" {
					status.Geometry = generation.Geometry
				}
			}
		}
		return status, nil
	}
	jobs := s.ListJobs(true, documentID)
	if len(jobs.Jobs) > 0 {
		job := jobs.Jobs[0]
		status.Status = publicOCRStatus(job.Status)
		status.PagesReady = job.ProcessedPages - job.FailedPages
		status.PagesFailed = job.FailedPages
		status.UpdatedAt = job.CreatedAt
	}
	return status, nil
}

func publicOCRStatus(status string) string {
	switch status {
	case "queued":
		return "queued"
	case "detecting_language", "processing":
		return "processing"
	case "indexing":
		return "indexing"
	case "completed", "completed_with_errors", "failed":
		return status
	default:
		return "unavailable"
	}
}

func (s *OCRService) GetTextLayerPage(documentID, generation string, pageNumber int) (*contract.TextLayerPage, error) {
	document, err := s.storage.GetDocument(documentID)
	if err != nil {
		return nil, ErrOCRDocumentNotFound
	}
	if document.Status != "completed" {
		return nil, ErrTextLayerNotReady
	}
	if !s.Enabled() {
		return nil, ErrTextLayerUnavailable
	}
	if pageNumber < 1 || pageNumber > document.TotalPages {
		return nil, fmt.Errorf("page fuera de rango")
	}
	if generation == "" {
		summary, summaryErr := s.GetSummary(documentID)
		if summaryErr != nil || summary.ActiveGeneration == "" {
			return nil, ErrTextLayerNotReady
		}
		generation = summary.ActiveGeneration
	}
	cacheKey := fmt.Sprintf("%s\x00%s\x00%d", documentID, generation, pageNumber)
	if s.textLayerCache != nil {
		if cached, found := s.textLayerCache.Get(cacheKey); found {
			if page, ok := cached.(*contract.TextLayerPage); ok {
				return page, nil
			}
		}
	}
	page, err := s.getPageGeneration(documentID, generation, pageNumber)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrOCRGenerationNotFound
		}
		return nil, err
	}
	words := make([]contract.TextLayerWord, 0, len(page.Words))
	for _, word := range page.Words {
		words = append(words, contract.TextLayerWord{Order: word.Order, BlockIndex: word.BlockIndex, ParagraphIndex: word.ParagraphIndex, LineIndex: word.LineIndex, WordIndex: word.WordIndex, Text: word.Text, Confidence: word.Confidence, BBox: contract.BoundingBox{X0: word.BBox.X0, Y0: word.BBox.Y0, X1: word.BBox.X1, Y1: word.BBox.Y1}})
	}
	response := &contract.TextLayerPage{SchemaVersion: contract.TextLayerSchemaVersion, DocumentID: page.DocumentID, Generation: page.Generation, PageNumber: page.PageNumber, Canvas: contract.Canvas{ID: page.CanvasV3, Width: page.Width, Height: page.Height, ImageID: page.ImageID, ImageServiceID: page.IIIFImage}, GeometrySpace: "canvas", GeometryStatus: page.GeometryStatus, Source: page.Source, Language: page.Language, Confidence: page.Confidence, Text: page.Text, LayerSHA256: page.LayerSHA256, Words: words}
	if s.textLayerCache != nil {
		s.textLayerCache.SetDefault(cacheKey, response)
	}
	return response, nil
}

func (s *OCRService) getPageGeneration(documentID, generation string, page int) (*OCRPage, error) {
	result, err := s.readPage(documentID, generation, page)
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
		result.Width, result.Height, result.GeometrySpace = canvasWidth, canvasHeight, "canvas"
	}
	finalizeOCRPage(result)
	return result, nil
}

func (s *OCRService) ListOCRGenerations(documentID string) (*contract.OCRGenerationList, error) {
	if _, err := s.storage.GetDocument(documentID); err != nil {
		return nil, ErrOCRDocumentNotFound
	}
	index, err := s.GetGenerationIndex(documentID)
	if err != nil {
		return nil, err
	}
	if summary, summaryErr := s.GetSummary(documentID); summaryErr == nil && index.ActiveGeneration == "" {
		index.ActiveGeneration = summary.ActiveGeneration
	}
	if len(index.Generations) == 0 {
		s.mu.RLock()
		for _, job := range s.jobs {
			if job.DocumentID == documentID {
				index.Generations = append(index.Generations, OCRGenerationRecord{ID: job.Generation, Status: publicOCRStatus(job.Status), CreatedAt: job.CreatedAt, UpdatedAt: time.Now().UTC(), PagesTotal: job.TotalPages, PagesReady: job.ProcessedPages - job.FailedPages, PagesFailed: job.FailedPages, Geometry: "mixed"})
			}
		}
		s.mu.RUnlock()
	}
	sort.Slice(index.Generations, func(i, j int) bool { return index.Generations[i].CreatedAt.After(index.Generations[j].CreatedAt) })
	response := &contract.OCRGenerationList{SchemaVersion: contract.TextLayerSchemaVersion, DocumentID: documentID, ActiveGeneration: index.ActiveGeneration, Generations: make([]contract.OCRGeneration, 0, len(index.Generations))}
	for _, item := range index.Generations {
		response.Generations = append(response.Generations, contract.OCRGeneration{ID: item.ID, Active: item.ID == index.ActiveGeneration, Status: publicOCRStatus(item.Status), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, PagesTotal: item.PagesTotal, PagesReady: item.PagesReady, PagesFailed: item.PagesFailed, Geometry: item.Geometry})
	}
	return response, nil
}
