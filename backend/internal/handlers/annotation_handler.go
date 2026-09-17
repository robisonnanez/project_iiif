package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type annotationApplication interface {
	Create(context.Context, services.CreateAnnotationInput) (*models.DocumentAnnotation, bool, error)
	Get(context.Context, string, string, string) (*models.DocumentAnnotation, error)
	Batch(context.Context, []string, string, string, string) ([]*models.DocumentAnnotation, error)
	Patch(context.Context, string, string, string, int64, services.PatchAnnotationInput) (*models.DocumentAnnotation, error)
	Delete(context.Context, string, string, string) error
}

type AnnotationHandler struct{ service annotationApplication }

func NewAnnotationHandler(service annotationApplication) *AnnotationHandler {
	return &AnnotationHandler{service: service}
}

type CreateAnnotationRequest struct {
	ClientReferenceID string                    `json:"client_reference_id" example:"0199b252-3c6f-7f6a-9a13-6b7c8d9e0001"`
	OCRGenerationID   string                    `json:"ocr_generation_id" example:"7c6a7b52-b427-4e3b-a3dc-8cb593dc8511"`
	LayerSHA256       string                    `json:"layer_sha256" example:"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	SelectedText      string                    `json:"selected_text" example:"fragmento seleccionado"`
	Note              *string                   `json:"note" example:"Comentario privado"`
	Color             *string                   `json:"color" example:"yellow"`
	Selector          models.AnnotationSelector `json:"selector"`
}

type AnnotationBatchRequest struct {
	IDs    []string `json:"ids"`
	PageID string   `json:"page_id,omitempty"`
}

type AnnotationBatchResponse struct {
	Annotations []*models.DocumentAnnotation `json:"annotations"`
}

// Create godoc
// @Summary Crear una anotación de documento
// @Description Persiste una selección sobre la generación OCR activa de una página estable.
// @Tags Annotations
// @Security IntegrationBearer
// @Accept json
// @Produce json
// @Param document_id path string true "UUID del documento"
// @Param page_id path string true "ID estable de document_images"
// @Param Idempotency-Key header string true "UUID de referencia generado por el cliente"
// @Param request body CreateAnnotationRequest true "Anotación"
// @Success 201 {object} models.DocumentAnnotation
// @Success 200 {object} models.DocumentAnnotation "Repetición idempotente"
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 422 {object} api.ErrorResponse
// @Failure 503 {object} api.ErrorResponse
// @Router /api/v1/documents/{document_id}/pages/{page_id}/annotations [post]
func (h *AnnotationHandler) Create(c *gin.Context) {
	started := time.Now()
	claims, ok := annotationClaims(c)
	if !ok {
		return
	}
	documentID := c.Param("document_id")
	if documentID == "" {
		documentID = c.Param("id")
	}
	if _, err := uuid.Parse(documentID); err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "document_id debe ser UUID", nil)
		return
	}
	var request CreateAnnotationRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_json", "JSON inválido", gin.H{"reason": err.Error()})
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" || idempotencyKey != request.ClientReferenceID {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "Idempotency-Key debe coincidir con client_reference_id", nil)
		return
	}
	annotation, replayed, err := h.service.Create(c.Request.Context(), services.CreateAnnotationInput{
		ProjectKey: claims.Project, TenantID: claims.Tenant, DocumentID: documentID, PageID: c.Param("page_id"),
		ClientReferenceID: request.ClientReferenceID, OCRGenerationID: request.OCRGenerationID,
		LayerSHA256: request.LayerSHA256, SelectedText: request.SelectedText, Note: request.Note,
		Selector: request.Selector, Color: request.Color,
	})
	if err != nil {
		writeAnnotationError(c, err)
		logAnnotationAudit(claims, "CREATE", "", c.Writer.Status(), started)
		return
	}
	writeAnnotationETag(c, annotation.Version)
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	writeJSON(c, status, annotation)
	logAnnotationAudit(claims, "CREATE", annotation.ID, status, started)
}

// Get godoc
// @Summary Consultar una anotación
// @Tags Annotations
// @Security IntegrationBearer
// @Produce json
// @Param annotation_id path string true "UUID v7 de la anotación"
// @Success 200 {object} models.DocumentAnnotation
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 503 {object} api.ErrorResponse
// @Router /api/v1/annotations/{annotation_id} [get]
func (h *AnnotationHandler) Get(c *gin.Context) {
	started := time.Now()
	claims, ok := annotationClaims(c)
	if !ok || !validAnnotationID(c) {
		return
	}
	annotation, err := h.service.Get(c.Request.Context(), c.Param("annotation_id"), claims.Tenant, claims.Project)
	if err != nil {
		writeAnnotationError(c, err)
		logAnnotationAudit(claims, "GET", c.Param("annotation_id"), c.Writer.Status(), started)
		return
	}
	writeAnnotationETag(c, annotation.Version)
	writeJSON(c, http.StatusOK, annotation)
	logAnnotationAudit(claims, "GET", annotation.ID, http.StatusOK, started)
}

// Batch godoc
// @Summary Consultar anotaciones por lote
// @Description Devuelve únicamente recursos visibles para el tenant y proyecto del token. Máximo 100 IDs.
// @Tags Annotations
// @Security IntegrationBearer
// @Accept json
// @Produce json
// @Param request body AnnotationBatchRequest true "IDs solicitados y filtro opcional de página"
// @Success 200 {object} AnnotationBatchResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 503 {object} api.ErrorResponse
// @Router /api/v1/annotations/batch [post]
func (h *AnnotationHandler) Batch(c *gin.Context) {
	started := time.Now()
	claims, ok := annotationClaims(c)
	if !ok {
		return
	}
	var request AnnotationBatchRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_json", "JSON inválido", gin.H{"reason": err.Error()})
		return
	}
	annotations, err := h.service.Batch(c.Request.Context(), request.IDs, claims.Tenant, claims.Project, request.PageID)
	if err != nil {
		writeAnnotationError(c, err)
		logAnnotationAudit(claims, "BATCH", "", c.Writer.Status(), started)
		return
	}
	writeJSON(c, http.StatusOK, AnnotationBatchResponse{Annotations: annotations})
	logAnnotationAudit(claims, "BATCH", "", http.StatusOK, started)
}

// Patch godoc
// @Summary Actualizar una anotación con concurrencia optimista
// @Description note y color son independientes. selector, selected_text, ocr_generation_id y layer_sha256 deben enviarse juntos.
// @Tags Annotations
// @Security IntegrationBearer
// @Accept json
// @Produce json
// @Param annotation_id path string true "UUID v7 de la anotación"
// @Param If-Match header string true "ETag actual, por ejemplo \"v1\""
// @Param request body object true "Campos editables"
// @Success 200 {object} models.DocumentAnnotation
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 412 {object} api.ErrorResponse
// @Failure 422 {object} api.ErrorResponse
// @Failure 503 {object} api.ErrorResponse
// @Router /api/v1/annotations/{annotation_id} [patch]
func (h *AnnotationHandler) Patch(c *gin.Context) {
	started := time.Now()
	claims, ok := annotationClaims(c)
	if !ok || !validAnnotationID(c) {
		return
	}
	version, err := parseAnnotationETag(c.GetHeader("If-Match"))
	if err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "If-Match es obligatorio y debe usar el formato \"vN\"", nil)
		return
	}
	input, err := decodeAnnotationPatch(c)
	if err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	annotation, err := h.service.Patch(c.Request.Context(), c.Param("annotation_id"), claims.Tenant, claims.Project, version, input)
	if err != nil {
		writeAnnotationError(c, err)
		logAnnotationAudit(claims, "PATCH", c.Param("annotation_id"), c.Writer.Status(), started)
		return
	}
	writeAnnotationETag(c, annotation.Version)
	writeJSON(c, http.StatusOK, annotation)
	logAnnotationAudit(claims, "PATCH", annotation.ID, http.StatusOK, started)
}

// Delete godoc
// @Summary Eliminar lógicamente una anotación
// @Tags Annotations
// @Security IntegrationBearer
// @Param annotation_id path string true "UUID v7 de la anotación"
// @Success 204
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 503 {object} api.ErrorResponse
// @Router /api/v1/annotations/{annotation_id} [delete]
func (h *AnnotationHandler) Delete(c *gin.Context) {
	started := time.Now()
	claims, ok := annotationClaims(c)
	if !ok || !validAnnotationID(c) {
		return
	}
	err := h.service.Delete(c.Request.Context(), c.Param("annotation_id"), claims.Tenant, claims.Project)
	if err != nil {
		writeAnnotationError(c, err)
		logAnnotationAudit(claims, "DELETE", c.Param("annotation_id"), c.Writer.Status(), started)
		return
	}
	c.Status(http.StatusNoContent)
	logAnnotationAudit(claims, "DELETE", c.Param("annotation_id"), http.StatusNoContent, started)
}

func decodeAnnotationPatch(c *gin.Context) (services.PatchAnnotationInput, error) {
	var raw map[string]json.RawMessage
	if err := decodeStrictJSON(c, &raw); err != nil {
		return services.PatchAnnotationInput{}, err
	}
	allowed := map[string]bool{"note": true, "color": true, "selector": true, "selected_text": true, "ocr_generation_id": true, "layer_sha256": true}
	for key := range raw {
		if !allowed[key] {
			return services.PatchAnnotationInput{}, fmt.Errorf("campo desconocido: %s", key)
		}
	}
	input := services.PatchAnnotationInput{}
	if value, exists := raw["note"]; exists {
		input.NoteSet = true
		if string(value) != "null" {
			var note string
			if err := json.Unmarshal(value, &note); err != nil {
				return input, fmt.Errorf("note debe ser string o null")
			}
			input.Note = &note
		}
	}
	if value, exists := raw["color"]; exists {
		input.ColorSet = true
		if string(value) != "null" {
			var color string
			if err := json.Unmarshal(value, &color); err != nil {
				return input, fmt.Errorf("color debe ser string o null")
			}
			input.Color = &color
		}
	}
	selectionFields := []string{"selector", "selected_text", "ocr_generation_id", "layer_sha256"}
	selectionCount := 0
	for _, field := range selectionFields {
		if _, exists := raw[field]; exists {
			selectionCount++
		}
	}
	if selectionCount != 0 && selectionCount != len(selectionFields) {
		return input, errors.New("selector, selected_text, ocr_generation_id y layer_sha256 deben enviarse juntos")
	}
	if selectionCount == len(selectionFields) {
		input.SelectionSet = true
		if err := json.Unmarshal(raw["selector"], &input.Selector); err != nil {
			return input, errors.New("selector inválido")
		}
		if err := json.Unmarshal(raw["selected_text"], &input.SelectedText); err != nil {
			return input, errors.New("selected_text inválido")
		}
		if err := json.Unmarshal(raw["ocr_generation_id"], &input.GenerationID); err != nil {
			return input, errors.New("ocr_generation_id inválido")
		}
		if err := json.Unmarshal(raw["layer_sha256"], &input.LayerSHA256); err != nil {
			return input, errors.New("layer_sha256 inválido")
		}
	}
	return input, nil
}

func decodeStrictJSON(c *gin.Context, destination any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("solo se permite un objeto JSON")
	}
	return nil
}

func annotationClaims(c *gin.Context) (*integrationClaims, bool) {
	value, exists := c.Get("integration_claims")
	claims, ok := value.(*integrationClaims)
	if !exists || !ok || claims.Project == "" || claims.Tenant == "" {
		writeContractError(c, http.StatusUnauthorized, "invalid_token", "el token debe identificar tenant y proyecto", nil)
		return nil, false
	}
	return claims, true
}

func validAnnotationID(c *gin.Context) bool {
	if _, err := uuid.Parse(c.Param("annotation_id")); err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "annotation_id debe ser UUID", nil)
		return false
	}
	return true
}

func parseAnnotationETag(value string) (int64, error) {
	match := regexp.MustCompile(`^"v([1-9][0-9]*)"$`).FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 2 {
		return 0, errors.New("ETag inválido")
	}
	return strconv.ParseInt(match[1], 10, 64)
}

func writeAnnotationETag(c *gin.Context, version int64) {
	c.Header("ETag", fmt.Sprintf(`"v%d"`, version))
}

func writeAnnotationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrAnnotationResourceNotFound):
		writeContractError(c, http.StatusNotFound, "resource_not_found", "recurso no encontrado", nil)
	case errors.Is(err, services.ErrAnnotationGenerationMismatch):
		writeContractError(c, http.StatusConflict, "generation_mismatch", err.Error(), nil)
	case errors.Is(err, services.ErrAnnotationIdempotencyConflict):
		writeContractError(c, http.StatusConflict, "idempotency_conflict", err.Error(), nil)
	case errors.Is(err, services.ErrInvalidAnnotationSelector):
		writeContractError(c, http.StatusUnprocessableEntity, "invalid_selector", err.Error(), nil)
	case errors.Is(err, services.ErrAnnotationChanged):
		writeContractError(c, http.StatusPreconditionFailed, "annotation_changed", err.Error(), nil)
	case errors.Is(err, services.ErrAnnotationStorageUnavailable):
		writeContractError(c, http.StatusServiceUnavailable, "storage_unavailable", err.Error(), nil)
	default:
		if strings.Contains(err.Error(), "ids debe") || strings.Contains(err.Error(), "id inválido") || strings.Contains(err.Error(), "campos editables") || strings.Contains(err.Error(), "excede") {
			writeContractError(c, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		writeContractError(c, http.StatusServiceUnavailable, "storage_unavailable", "almacenamiento temporalmente no disponible", nil)
	}
}

func logAnnotationAudit(claims *integrationClaims, operation, annotationID string, status int, started time.Time) {
	log.Printf("[AUDIT] consumer=%s tenant=%s project=%s annotation=%s operation=%s status=%d latency=%s",
		claims.Subject, claims.Tenant, claims.Project, annotationID, operation, status, time.Since(started).Round(time.Millisecond))
}
