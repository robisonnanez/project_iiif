package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"iiif-pdf-server/internal/services"

	"github.com/gin-gonic/gin"
)

type OCRHandler struct{ service *services.OCRService }

func NewOCRHandler(service *services.OCRService) *OCRHandler { return &OCRHandler{service: service} }

// CreateJob godoc
// @Summary Crear trabajo OCR
// @Description Inicia una generación OCR híbrida, exhaustiva o solo OCR. Requiere sesión administrativa.
// @Tags OCR
// @Security SessionCookie
// @Accept json
// @Produce json
// @Param id path string true "ID del documento"
// @Param request body services.CreateOCRJobRequest true "Configuración del trabajo"
// @Success 202 {object} services.OCRJob
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /api/v1/admin/documents/{id}/ocr/jobs [post]
func (h *OCRHandler) CreateJob(c *gin.Context) {
	var request services.CreateOCRJobRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload OCR inválido"})
		return
	}
	job, err := h.service.CreateJob(c.Param("id"), request)
	if err != nil {
		status := http.StatusBadRequest
		if !h.service.Enabled() {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// GetJob godoc
// @Summary Consultar progreso OCR
// @Tags OCR
// @Security SessionCookie
// @Produce json
// @Param job_id path string true "ID del trabajo"
// @Success 200 {object} services.OCRJob
// @Failure 404 {object} errorResponse
// @Router /api/v1/admin/ocr/jobs/{job_id} [get]
func (h *OCRHandler) GetJob(c *gin.Context) {
	job, err := h.service.GetJob(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

// CancelJob godoc
// @Summary Cancelar trabajo OCR
// @Tags OCR
// @Security SessionCookie
// @Produce json
// @Param job_id path string true "ID del trabajo"
// @Success 200 {object} services.OCRJob
// @Failure 404 {object} errorResponse
// @Router /api/v1/admin/ocr/jobs/{job_id}/cancel [post]
func (h *OCRHandler) CancelJob(c *gin.Context) {
	job, err := h.service.CancelJob(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

// GetSummary godoc
// @Summary Obtener estado OCR de un documento
// @Tags OCR
// @Produce json
// @Param id path string true "ID del documento"
// @Success 200 {object} services.OCRDocumentSummary
// @Failure 404 {object} errorResponse
// @Router /api/v1/documents/{id}/ocr [get]
func (h *OCRHandler) GetSummary(c *gin.Context) {
	summary, err := h.service.GetSummary(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// GetPage godoc
// @Summary Obtener OCR y bounding boxes de una página
// @Tags OCR
// @Produce json
// @Param id path string true "ID del documento"
// @Param page path int true "Número de página"
// @Success 200 {object} services.OCRPage
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /api/v1/documents/{id}/ocr/pages/{page} [get]
func (h *OCRHandler) GetPage(c *gin.Context) {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page debe ser mayor a cero"})
		return
	}
	result, err := h.service.GetPage(c.Param("id"), page)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "página OCR no encontrada"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// SearchDocument godoc
// @Summary Buscar texto OCR dentro de un documento
// @Tags OCR
// @Produce json
// @Param id path string true "ID del documento"
// @Param q query string true "Texto a buscar"
// @Param limit query int false "Máximo de resultados" default(20)
// @Param offset query int false "Desplazamiento" default(0)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} errorResponse
// @Router /api/v1/documents/{id}/ocr/search [get]
func (h *OCRHandler) SearchDocument(c *gin.Context) { h.search(c, c.Param("id")) }

// Search godoc
// @Summary Buscar texto OCR por proyecto, tenant o documento
// @Tags OCR
// @Produce json
// @Param q query string true "Texto a buscar"
// @Param project query string false "Proyecto"
// @Param tenant query string false "Tenant"
// @Param document_id query string false "ID del documento"
// @Param limit query int false "Máximo de resultados" default(20)
// @Param offset query int false "Desplazamiento" default(0)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} errorResponse
// @Router /api/v1/ocr/search [get]
func (h *OCRHandler) Search(c *gin.Context) { h.search(c, strings.TrimSpace(c.Query("document_id"))) }
func (h *OCRHandler) search(c *gin.Context, documentID string) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	results, total, err := h.service.Search(c.Query("q"), strings.TrimSpace(c.Query("project")), strings.TrimSpace(c.Query("tenant")), documentID, limit, offset)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "total": total, "limit": limit, "offset": offset})
}

func (h *OCRHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
