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

func (h *OCRHandler) GetJob(c *gin.Context) {
	job, err := h.service.GetJob(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}
func (h *OCRHandler) CancelJob(c *gin.Context) {
	job, err := h.service.CancelJob(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}
func (h *OCRHandler) GetSummary(c *gin.Context) {
	summary, err := h.service.GetSummary(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

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

func (h *OCRHandler) SearchDocument(c *gin.Context) { h.search(c, c.Param("id")) }
func (h *OCRHandler) Search(c *gin.Context)         { h.search(c, strings.TrimSpace(c.Query("document_id"))) }
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
