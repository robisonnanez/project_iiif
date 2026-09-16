package handlers

import (
	"errors"
	"net/http"
	"strconv"

	contract "iiif-pdf-server/internal/api"
	"iiif-pdf-server/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type textLayerService interface {
	GetTextLayerStatus(string) (*contract.TextLayerStatus, error)
	GetTextLayerPage(string, string, int) (*contract.TextLayerPage, error)
	ListOCRGenerations(string) (*contract.OCRGenerationList, error)
}

type TextLayerHandler struct{ service textLayerService }

func NewTextLayerHandler(service textLayerService) *TextLayerHandler {
	return &TextLayerHandler{service: service}
}

func (h *TextLayerHandler) Status(c *gin.Context) {
	if !validUUIDParam(c) {
		return
	}
	response, err := h.service.GetTextLayerStatus(c.Param("id"))
	if err != nil {
		writeTextLayerError(c, err, c.Param("id"), 0)
		return
	}
	writeJSON(c, http.StatusOK, response)
}

func (h *TextLayerHandler) Page(c *gin.Context) {
	if !validUUIDParam(c) {
		return
	}
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page < 1 {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "page debe ser un entero de base 1", gin.H{"page": c.Param("page")})
		return
	}
	generation := c.Query("generation")
	if generation != "" {
		if _, err := uuid.Parse(generation); err != nil {
			writeContractError(c, http.StatusBadRequest, "invalid_request", "generation debe ser UUID", gin.H{"generation": generation})
			return
		}
	}
	response, err := h.service.GetTextLayerPage(c.Param("id"), generation, page)
	if err != nil {
		writeTextLayerError(c, err, c.Param("id"), page)
		return
	}
	writeJSON(c, http.StatusOK, response)
}

func (h *TextLayerHandler) Generations(c *gin.Context) {
	if !validUUIDParam(c) {
		return
	}
	response, err := h.service.ListOCRGenerations(c.Param("id"))
	if err != nil {
		writeTextLayerError(c, err, c.Param("id"), 0)
		return
	}
	writeJSON(c, http.StatusOK, response)
}

func validUUIDParam(c *gin.Context) bool {
	if _, err := uuid.Parse(c.Param("id")); err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "id debe ser UUID", gin.H{"document_id": c.Param("id")})
		return false
	}
	return true
}

func writeTextLayerError(c *gin.Context, err error, documentID string, page int) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, services.ErrOCRDocumentNotFound), errors.Is(err, services.ErrOCRGenerationNotFound):
		status, code = http.StatusNotFound, "document_or_generation_not_found"
	case errors.Is(err, services.ErrTextLayerNotReady):
		status, code = http.StatusConflict, "document_not_ready"
	case errors.Is(err, services.ErrTextLayerUnavailable), errors.Is(err, services.ErrOCRDisabled):
		status, code = http.StatusServiceUnavailable, "ocr_unavailable"
	case err.Error() == "page fuera de rango":
		status, code = http.StatusBadRequest, "invalid_request"
	}
	details := gin.H{"document_id": documentID}
	if page > 0 {
		details["page_number"] = page
	}
	writeContractError(c, status, code, err.Error(), details)
}

func writeContractError(c *gin.Context, status int, code, message string, details map[string]any) {
	writeJSON(c, status, contract.ErrorResponse{Error: contract.ErrorDetail{Code: code, Message: message, Details: details}})
}
