package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"iiif-pdf-server/internal/services"

	"github.com/gin-gonic/gin"
)

type OCRHandler struct {
	service         *services.OCRService
	languageService *services.OCRLanguageService
}

func NewOCRHandler(service *services.OCRService, languageServices ...*services.OCRLanguageService) *OCRHandler {
	handler := &OCRHandler{service: service}
	if len(languageServices) > 0 {
		handler.languageService = languageServices[0]
	}
	return handler
}

// GetLanguages godoc
// @Summary Consultar idiomas OCR del sistema
// @Description Obtiene los idiomas reconocidos por Tesseract y los paquetes APT disponibles para instalar. Requiere sesión administrativa.
// @Tags OCR
// @Security SessionCookie
// @Produce json
// @Success 200 {object} services.OCRLanguageCatalog
// @Failure 401 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /api/v1/admin/ocr/languages [get]
func (h *OCRHandler) GetLanguages(c *gin.Context) {
	if h.languageService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "administración de idiomas OCR no disponible"})
		return
	}
	catalog, err := h.languageService.Catalog(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, catalog)
}

// InstallLanguages godoc
// @Summary Instalar idiomas OCR
// @Description Instala hasta 10 paquetes descubiertos en APT mediante un helper privilegiado restringido y verifica el resultado con Tesseract. Instalar no habilita automáticamente el idioma.
// @Tags OCR
// @Security SessionCookie
// @Accept json
// @Produce json
// @Param request body services.InstallOCRLanguagesRequest true "Idiomas Tesseract"
// @Success 200 {object} services.InstallOCRLanguagesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /api/v1/admin/ocr/languages/install [post]
func (h *OCRHandler) InstallLanguages(c *gin.Context) {
	if h.languageService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "administración de idiomas OCR no disponible"})
		return
	}
	var request services.InstallOCRLanguagesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "selección de idiomas inválida"})
		return
	}
	result, err := h.languageService.Install(c.Request.Context(), request.Languages)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "en curso") {
			status = http.StatusConflict
		} else if strings.Contains(err.Error(), "deshabilitada") || strings.Contains(err.Error(), "no se pudo instalar") {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

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
// @Description Devuelve el texto completo y, cuando existe geometría, palabras con bbox x0/x1/y0/y1 expresado en coordenadas del Canvas IIIF. Los OCR históricos sin geometría continúan con geometry_status=page_only.
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

// FindPageWords godoc
// @Summary Buscar una palabra y sus bounding boxes en una página OCR
// @Description Devuelve únicamente las apariciones exactas de q con confianza y bbox en coordenadas del Canvas IIIF. Ignora mayúsculas, acentos y puntuación exterior. Las generaciones page_only deben reprocesarse con force=true.
// @Tags OCR
// @Produce json
// @Param id path string true "ID del documento"
// @Param page path int true "Número de página"
// @Param q query string true "Palabra a localizar"
// @Param limit query int false "Máximo de apariciones (máximo 1000)" default(100)
// @Success 200 {object} services.OCRWordSearchResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Router /api/v1/documents/{id}/ocr/pages/{page}/words [get]
func (h *OCRHandler) FindPageWords(c *gin.Context) {
	page, err := strconv.Atoi(c.Param("page"))
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page debe ser mayor a cero"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	result, err := h.service.FindPageWords(c.Param("id"), page, c.Query("q"), limit)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrOCRWordGeometryUnavailable) {
			status = http.StatusConflict
		} else if !strings.Contains(err.Error(), "q debe") {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
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

// AutocompleteDocument godoc
// @Summary Autocompletar palabras OCR dentro de un documento
// @Description Sugiere palabras reales del OCR por prefijo. La comparación ignora mayúsculas y acentos, conserva la grafía original y requiere al menos 2 caracteres.
// @Tags OCR
// @Produce json
// @Param id path string true "ID del documento"
// @Param q query string true "Prefijo de palabra (mínimo 2 caracteres)"
// @Param limit query int false "Máximo de sugerencias (máximo 50)" default(10)
// @Success 200 {object} services.OCRAutocompleteResponse
// @Failure 400 {object} errorResponse
// @Router /api/v1/documents/{id}/ocr/autocomplete [get]
func (h *OCRHandler) AutocompleteDocument(c *gin.Context) { h.autocomplete(c, c.Param("id")) }

// Autocomplete godoc
// @Summary Autocompletar palabras OCR por proyecto, tenant o documento
// @Description Sugiere palabras reales del OCR por prefijo. La comparación ignora mayúsculas y acentos, conserva la grafía original y requiere al menos 2 caracteres.
// @Tags OCR
// @Produce json
// @Param q query string true "Prefijo de palabra (mínimo 2 caracteres)"
// @Param project query string false "Proyecto"
// @Param tenant query string false "Tenant"
// @Param document_id query string false "ID del documento"
// @Param limit query int false "Máximo de sugerencias (máximo 50)" default(10)
// @Success 200 {object} services.OCRAutocompleteResponse
// @Failure 400 {object} errorResponse
// @Router /api/v1/ocr/autocomplete [get]
func (h *OCRHandler) Autocomplete(c *gin.Context) {
	h.autocomplete(c, strings.TrimSpace(c.Query("document_id")))
}

func (h *OCRHandler) autocomplete(c *gin.Context, documentID string) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	query := strings.TrimSpace(c.Query("q"))
	items, err := h.service.Autocomplete(query, strings.TrimSpace(c.Query("project")), strings.TrimSpace(c.Query("tenant")), documentID, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, services.OCRAutocompleteResponse{Query: query, Items: items})
}

func (h *OCRHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
