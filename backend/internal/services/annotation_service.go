package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"

	contract "iiif-pdf-server/internal/api"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

	"github.com/google/uuid"
)

var (
	ErrAnnotationStorageUnavailable  = errors.New("almacenamiento de anotaciones no disponible")
	ErrAnnotationResourceNotFound    = errors.New("recurso no encontrado")
	ErrAnnotationGenerationMismatch  = errors.New("la generación OCR no corresponde con la selección")
	ErrInvalidAnnotationSelector     = errors.New("selector inválido")
	ErrAnnotationIdempotencyConflict = errors.New("la clave de idempotencia ya fue utilizada con otro contenido")
	ErrAnnotationChanged             = storage.ErrAnnotationChanged
)

const maxAnnotationRectangles = 200

type annotationMetadataStorage interface {
	GetDocument(string) (*models.PDFDocument, error)
	GetDocumentImage(string) (*models.DocumentImage, error)
}

type annotationTextLayer interface {
	GetTextLayerPage(string, string, int) (*contract.TextLayerPage, error)
	ListOCRGenerations(string) (*contract.OCRGenerationList, error)
}

type AnnotationService struct {
	repository storage.AnnotationStorage
	metadata   annotationMetadataStorage
	textLayer  annotationTextLayer
}

type CreateAnnotationInput struct {
	ProjectKey        string
	TenantID          string
	DocumentID        string
	PageID            string
	ClientReferenceID string
	OCRGenerationID   string
	LayerSHA256       string
	SelectedText      string
	Note              *string
	Selector          models.AnnotationSelector
	Color             *string
}

type PatchAnnotationInput struct {
	NoteSet      bool
	Note         *string
	ColorSet     bool
	Color        *string
	SelectionSet bool
	SelectedText string
	Selector     models.AnnotationSelector
	GenerationID string
	LayerSHA256  string
}

func NewAnnotationService(repository storage.AnnotationStorage, metadata annotationMetadataStorage, textLayer annotationTextLayer) *AnnotationService {
	return &AnnotationService{repository: repository, metadata: metadata, textLayer: textLayer}
}

func (s *AnnotationService) Create(ctx context.Context, input CreateAnnotationInput) (*models.DocumentAnnotation, bool, error) {
	if s.repository == nil {
		return nil, false, ErrAnnotationStorageUnavailable
	}
	if input.ProjectKey == "" || input.TenantID == "" {
		return nil, false, ErrAnnotationResourceNotFound
	}
	if _, err := uuid.Parse(input.ClientReferenceID); err != nil {
		return nil, false, fmt.Errorf("%w: client_reference_id debe ser UUID", ErrInvalidAnnotationSelector)
	}
	if err := validateSelector(input.Selector, input.SelectedText); err != nil {
		return nil, false, err
	}
	if err := validateOptionalFields(input.Note, input.Color); err != nil {
		return nil, false, err
	}
	fingerprint, err := annotationFingerprint(input)
	if err != nil {
		return nil, false, err
	}
	existing, err := s.repository.GetAnnotationByClientReference(ctx, input.TenantID, input.ClientReferenceID)
	if err == nil {
		if existing.ProjectKey != input.ProjectKey || existing.RequestFingerprint != fingerprint || existing.DeletedAt != nil {
			return nil, false, ErrAnnotationIdempotencyConflict
		}
		if err := s.decorateGeneration(existing); err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, storage.ErrAnnotationNotFound) {
		return nil, false, err
	}
	page, active, err := s.validateSelection(input.ProjectKey, input.TenantID, input.DocumentID, input.PageID,
		input.OCRGenerationID, input.LayerSHA256, input.SelectedText, input.Selector, true)
	if err != nil {
		return nil, false, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, false, err
	}
	annotation := &models.DocumentAnnotation{
		ID: id.String(), ProjectKey: input.ProjectKey, TenantID: input.TenantID,
		DocumentID: input.DocumentID, PageID: input.PageID, PageNumber: page.PageNumber,
		OCRGenerationID: input.OCRGenerationID, LayerSHA256: strings.ToLower(input.LayerSHA256),
		ClientReferenceID: input.ClientReferenceID, RequestFingerprint: fingerprint,
		SelectedText: input.SelectedText, Note: input.Note, Selector: input.Selector, Color: input.Color,
		Version: 1, GenerationActive: active, Historical: !active,
	}
	created, replayed, err := s.repository.CreateAnnotation(ctx, annotation)
	if err != nil {
		return nil, false, err
	}
	if replayed {
		if created.ProjectKey != input.ProjectKey || created.RequestFingerprint != fingerprint || created.DeletedAt != nil {
			return nil, false, ErrAnnotationIdempotencyConflict
		}
		if err := s.decorateGeneration(created); err != nil {
			return nil, false, err
		}
		return created, true, nil
	}
	created.GenerationActive, created.Historical = active, !active
	return created, false, nil
}

func (s *AnnotationService) Get(ctx context.Context, id, tenant, project string) (*models.DocumentAnnotation, error) {
	if s.repository == nil {
		return nil, ErrAnnotationStorageUnavailable
	}
	annotation, err := s.repository.GetAnnotation(ctx, id, tenant, project, false)
	if errors.Is(err, storage.ErrAnnotationNotFound) {
		return nil, ErrAnnotationResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.decorateGeneration(annotation); err != nil {
		return nil, err
	}
	return annotation, nil
}

func (s *AnnotationService) Batch(ctx context.Context, ids []string, tenant, project, pageID string) ([]*models.DocumentAnnotation, error) {
	if s.repository == nil {
		return nil, ErrAnnotationStorageUnavailable
	}
	if len(ids) == 0 || len(ids) > 100 {
		return nil, fmt.Errorf("ids debe contener entre 1 y 100 UUID")
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("id inválido: %s", id)
		}
	}
	annotations, err := s.repository.GetAnnotationsByIDs(ctx, ids, tenant, project, pageID)
	if err != nil {
		return nil, err
	}
	for _, annotation := range annotations {
		if err := s.decorateGeneration(annotation); err != nil {
			return nil, err
		}
	}
	return annotations, nil
}

func (s *AnnotationService) Patch(ctx context.Context, id, tenant, project string, expectedVersion int64, input PatchAnnotationInput) (*models.DocumentAnnotation, error) {
	if s.repository == nil {
		return nil, ErrAnnotationStorageUnavailable
	}
	annotation, err := s.repository.GetAnnotation(ctx, id, tenant, project, false)
	if errors.Is(err, storage.ErrAnnotationNotFound) {
		return nil, ErrAnnotationResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if annotation.Version != expectedVersion {
		return nil, ErrAnnotationChanged
	}
	if !input.NoteSet && !input.ColorSet && !input.SelectionSet {
		return nil, fmt.Errorf("no hay campos editables")
	}
	if input.NoteSet {
		annotation.Note = input.Note
	}
	if input.ColorSet {
		annotation.Color = input.Color
	}
	if err := validateOptionalFields(annotation.Note, annotation.Color); err != nil {
		return nil, err
	}
	if input.SelectionSet {
		_, _, err := s.validateSelection(project, tenant, annotation.DocumentID, annotation.PageID,
			input.GenerationID, input.LayerSHA256, input.SelectedText, input.Selector, true)
		if err != nil {
			return nil, err
		}
		annotation.SelectedText, annotation.Selector = input.SelectedText, input.Selector
		annotation.OCRGenerationID, annotation.LayerSHA256 = input.GenerationID, strings.ToLower(input.LayerSHA256)
	}
	updated, err := s.repository.UpdateAnnotation(ctx, annotation, expectedVersion)
	if errors.Is(err, storage.ErrAnnotationChanged) {
		return nil, ErrAnnotationChanged
	}
	if errors.Is(err, storage.ErrAnnotationNotFound) {
		return nil, ErrAnnotationResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.decorateGeneration(updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *AnnotationService) Delete(ctx context.Context, id, tenant, project string) error {
	if s.repository == nil {
		return ErrAnnotationStorageUnavailable
	}
	err := s.repository.SoftDeleteAnnotation(ctx, id, tenant, project)
	if errors.Is(err, storage.ErrAnnotationNotFound) {
		return ErrAnnotationResourceNotFound
	}
	return err
}

func (s *AnnotationService) validateSelection(project, tenant, documentID, pageID, generationID, layerSHA, selectedText string, selector models.AnnotationSelector, requireActive bool) (*models.DocumentImage, bool, error) {
	document, err := s.metadata.GetDocument(documentID)
	if err != nil || document.ProjectKey != project || document.TenantKey != tenant {
		return nil, false, ErrAnnotationResourceNotFound
	}
	page, err := s.metadata.GetDocumentImage(pageID)
	if err != nil || page.DocumentID != documentID || page.ProjectKey != project || page.TenantKey != tenant {
		return nil, false, ErrAnnotationResourceNotFound
	}
	if err := validateSelector(selector, selectedText); err != nil {
		return nil, false, err
	}
	if _, err := uuid.Parse(generationID); err != nil || !regexp.MustCompile(`^[0-9a-fA-F]{64}$`).MatchString(layerSHA) {
		return nil, false, ErrAnnotationGenerationMismatch
	}
	layer, err := s.textLayer.GetTextLayerPage(documentID, generationID, page.PageNumber)
	if err != nil || !strings.EqualFold(layer.LayerSHA256, layerSHA) || layer.Generation != generationID ||
		layer.Canvas.ID != selector.Canvas.ID || layer.Canvas.Width != selector.Canvas.Width || layer.Canvas.Height != selector.Canvas.Height {
		return nil, false, ErrAnnotationGenerationMismatch
	}
	generations, err := s.textLayer.ListOCRGenerations(documentID)
	if err != nil {
		return nil, false, ErrAnnotationGenerationMismatch
	}
	active := generations.ActiveGeneration == generationID
	if requireActive && !active {
		return nil, false, ErrAnnotationGenerationMismatch
	}
	return page, active, nil
}

func (s *AnnotationService) decorateGeneration(annotation *models.DocumentAnnotation) error {
	generations, err := s.textLayer.ListOCRGenerations(annotation.DocumentID)
	if err != nil {
		return err
	}
	annotation.GenerationActive = generations.ActiveGeneration == annotation.OCRGenerationID
	annotation.Historical = !annotation.GenerationActive
	return nil
}

func validateSelector(selector models.AnnotationSelector, selectedText string) error {
	if selector.Type != "MultiRectangleSelector" || selector.CoordinateSpace != "canvas_normalized" {
		return fmt.Errorf("%w: tipo o coordinate_space no permitido", ErrInvalidAnnotationSelector)
	}
	if strings.TrimSpace(selector.Canvas.ID) == "" || selector.Canvas.Width <= 0 || selector.Canvas.Height <= 0 {
		return fmt.Errorf("%w: canvas inválido", ErrInvalidAnnotationSelector)
	}
	if len(selector.Rectangles) < 1 || len(selector.Rectangles) > maxAnnotationRectangles {
		return fmt.Errorf("%w: rectangles debe contener entre 1 y %d elementos", ErrInvalidAnnotationSelector, maxAnnotationRectangles)
	}
	for _, rectangle := range selector.Rectangles {
		values := []float64{rectangle.X, rectangle.Y, rectangle.Width, rectangle.Height}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				return fmt.Errorf("%w: coordenada fuera de rango", ErrInvalidAnnotationSelector)
			}
		}
		if rectangle.Width <= 0 || rectangle.Height <= 0 || rectangle.X+rectangle.Width > 1.0001 || rectangle.Y+rectangle.Height > 1.0001 {
			return fmt.Errorf("%w: rectángulo fuera del canvas", ErrInvalidAnnotationSelector)
		}
	}
	if normalizeAnnotationText(selector.Quote.Exact) != normalizeAnnotationText(selectedText) {
		return fmt.Errorf("%w: quote.exact no coincide con selected_text", ErrInvalidAnnotationSelector)
	}
	if normalizeAnnotationText(selectedText) == "" {
		return fmt.Errorf("%w: selected_text no puede estar vacío", ErrInvalidAnnotationSelector)
	}
	return nil
}

func normalizeAnnotationText(value string) string {
	return strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
}

func validateOptionalFields(note, color *string) error {
	if color != nil && len(*color) > 20 {
		return fmt.Errorf("color excede 20 caracteres")
	}
	if note != nil && len(*note) > 100000 {
		return fmt.Errorf("note excede el límite permitido")
	}
	return nil
}

func annotationFingerprint(input CreateAnnotationInput) (string, error) {
	payload := struct {
		DocumentID, PageID, ClientReferenceID, GenerationID, LayerSHA, SelectedText string
		Note, Color                                                                 *string
		Selector                                                                    models.AnnotationSelector
	}{input.DocumentID, input.PageID, input.ClientReferenceID, input.OCRGenerationID,
		strings.ToLower(input.LayerSHA256), input.SelectedText, input.Note, input.Color, input.Selector}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
