package handlers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	contract "iiif-pdf-server/internal/api"
	"iiif-pdf-server/internal/services"

	"github.com/andybalholm/brotli"
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
	startedAt := time.Now()
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
	writeTextLayerPage(c, response, startedAt)
}

func writeTextLayerPage(c *gin.Context, response *contract.TextLayerPage, startedAt time.Time) {
	etag := `"` + response.LayerSHA256 + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private")
	c.Header("Vary", "Accept-Encoding, Authorization")
	if c.GetHeader("If-None-Match") == etag {
		log.Printf("[TEXT_LAYER] document=%s generation=%s page=%d status=304 cache_hit=true latency=%s", response.DocumentID, response.Generation, response.PageNumber, time.Since(startedAt).Round(time.Millisecond))
		c.Status(http.StatusNotModified)
		return
	}
	payload, err := json.Marshal(response)
	if err != nil {
		writeContractError(c, http.StatusInternalServerError, "internal_error", "no se pudo serializar la capa", nil)
		return
	}
	if len(payload) > 10<<20 {
		log.Printf("[TEXT_LAYER] oversized=true document=%s generation=%s page=%d bytes=%d", response.DocumentID, response.Generation, response.PageNumber, len(payload))
	}
	encoding, encoded, err := compressTextLayer(c.GetHeader("Accept-Encoding"), payload)
	if err != nil {
		writeContractError(c, http.StatusInternalServerError, "internal_error", "no se pudo comprimir la capa", nil)
		return
	}
	if encoding != "" {
		c.Header("Content-Encoding", encoding)
	}
	log.Printf("[TEXT_LAYER] document=%s generation=%s page=%d status=200 bytes=%d encoding=%s latency=%s", response.DocumentID, response.Generation, response.PageNumber, len(payload), encoding, time.Since(startedAt).Round(time.Millisecond))
	c.Data(http.StatusOK, "application/json; charset=utf-8", encoded)
}

func compressTextLayer(accepted string, payload []byte) (string, []byte, error) {
	accepted = strings.ToLower(accepted)
	var buffer bytes.Buffer
	if strings.Contains(accepted, "br") {
		writer := brotli.NewWriterLevel(&buffer, 4)
		if _, err := writer.Write(payload); err != nil {
			return "", nil, err
		}
		if err := writer.Close(); err != nil {
			return "", nil, err
		}
		return "br", buffer.Bytes(), nil
	}
	if strings.Contains(accepted, "gzip") {
		writer, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
		if err != nil {
			return "", nil, err
		}
		if _, err := writer.Write(payload); err != nil {
			return "", nil, err
		}
		if err := writer.Close(); err != nil {
			return "", nil, err
		}
		return "gzip", buffer.Bytes(), nil
	}
	return "", payload, nil
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
