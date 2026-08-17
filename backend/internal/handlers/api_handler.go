package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/services"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	pdfService      *services.PDFService
	iiifService     *services.IIIFService
	documentService *services.DocumentService
	config          *config.Config
}

const (
	defaultUploadWidth  = 1241
	defaultUploadHeight = 1754
	defaultUploadDPI    = 150
)

func NewAPIHandler(
	pdfService *services.PDFService,
	iiifService *services.IIIFService,
	documentService *services.DocumentService,
	config *config.Config,
) *APIHandler {
	return &APIHandler{
		pdfService:      pdfService,
		iiifService:     iiifService,
		documentService: documentService,
		config:          config,
	}
}

// UploadPDF godoc
// @Summary Subir y convertir PDF
// @Description Recibe archivo PDF y crea documento con paginas convertidas.
// @Tags Documentos
// @Accept multipart/form-data
// @Produce json
// @Param pdf formData file true "Archivo PDF"
// @Param project formData string false "Proyecto"
// @Param tenant formData string false "Tenant"
// @Param max_width formData int false "Ancho maximo de imagen" default(1241)
// @Param max_height formData int false "Alto maximo de imagen" default(1754)
// @Param dpi formData int false "Resolucion de render" default(150)
// @Param format formData string false "Formato de salida (jpg o png)" default(jpg)
// @Param quality formData int false "Calidad JPEG" default(85)
// @Success 200 {object} models.PDFDocument
// @Failure 400 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/documents/upload [post]
func (h *APIHandler) UploadPDF(c *gin.Context) {
	// Obtener archivo
	file, header, err := c.Request.FormFile("pdf")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No se pudo obtener el archivo"})
		return
	}
	defer file.Close()

	// Validar tamaño
	if header.Size > h.config.PDF.MaxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Archivo demasiado grande"})
		return
	}

	// Obtener configuración de conversión
	settings, err := h.conversionSettings(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Usar un nombre aleatorio evita colisiones entre cargas concurrentes con el mismo nombre.
	if err := os.MkdirAll(h.config.PDF.TempPath, 0o750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error preparando almacenamiento temporal"})
		return
	}
	tempFile, err := os.CreateTemp(h.config.PDF.TempPath, "upload-*.pdf")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error guardando archivo"})
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error copiando archivo"})
		return
	}
	if err := tempFile.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error cerrando archivo temporal"})
		return
	}

	scope, err := h.requestScope(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Procesar PDF
	doc, err := h.pdfService.ProcessPDF(tempPath, header.Filename, settings, scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error procesando PDF"})
		return
	}

	c.JSON(http.StatusOK, doc)
}

func (h *APIHandler) conversionSettings(c *gin.Context) (models.ConversionSettings, error) {
	dpi := h.config.Conversion.DPI
	if dpi == 0 {
		dpi = defaultUploadDPI
	}
	settings := models.ConversionSettings{
		Format:    strings.ToLower(strings.TrimSpace(h.config.Conversion.DefaultFormat)),
		Quality:   h.config.Conversion.DefaultQuality,
		MaxWidth:  h.config.Conversion.DefaultWidth,
		MaxHeight: h.config.Conversion.DefaultHeight,
		DPI:       dpi,
		EnableOCR: h.config.Conversion.EnableOCR,
	}
	if settings.MaxWidth == 0 {
		settings.MaxWidth = defaultUploadWidth
	}
	if settings.MaxHeight == 0 {
		settings.MaxHeight = defaultUploadHeight
	}
	if settings.Format == "" {
		settings.Format = "jpg"
	}
	if settings.Quality == 0 {
		settings.Quality = 85
	}

	var err error
	if settings.MaxWidth, err = formInt(c, "max_width", settings.MaxWidth); err != nil {
		return settings, err
	}
	if settings.MaxHeight, err = formInt(c, "max_height", settings.MaxHeight); err != nil {
		return settings, err
	}
	if settings.DPI, err = formInt(c, "dpi", settings.DPI); err != nil {
		return settings, err
	}
	if settings.Quality, err = formInt(c, "quality", settings.Quality); err != nil {
		return settings, err
	}
	if value := strings.ToLower(strings.TrimSpace(c.PostForm("format"))); value != "" {
		settings.Format = value
	}
	if settings.Format == "jpeg" {
		settings.Format = "jpg"
	}

	if settings.MaxWidth < 256 || settings.MaxWidth > 8192 {
		return settings, errors.New("el ancho debe estar entre 256 y 8192 pixeles")
	}
	if settings.MaxHeight < 256 || settings.MaxHeight > 8192 {
		return settings, errors.New("el alto debe estar entre 256 y 8192 pixeles")
	}
	if settings.DPI < 72 || settings.DPI > 600 {
		return settings, errors.New("el DPI debe estar entre 72 y 600")
	}
	if settings.Quality < 1 || settings.Quality > 100 {
		return settings, errors.New("la calidad debe estar entre 1 y 100")
	}
	if settings.Format != "jpg" && settings.Format != "png" {
		return settings, errors.New("el formato debe ser jpg o png")
	}
	return settings, nil
}

func formInt(c *gin.Context, name string, fallback int) (int, error) {
	value := strings.TrimSpace(c.PostForm(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback, fmt.Errorf("%s debe ser un numero entero", name)
	}
	return parsed, nil
}

// GetDocuments godoc
// @Summary Listar documentos
// @Description Lista documentos con filtros opcionales por proyecto y tenant.
// @Tags Documentos
// @Produce json
// @Param project query string false "Proyecto"
// @Param tenant query string false "Tenant"
// @Success 200 {array} models.PDFDocument
// @Failure 500 {object} errorResponse
// @Router /api/v1/documents [get]
func (h *APIHandler) GetDocuments(c *gin.Context) {
	project := strings.TrimSpace(c.Query("project"))
	tenant := strings.TrimSpace(c.Query("tenant"))
	var docs []*models.PDFDocument
	var err error
	if project != "" || tenant != "" {
		docs, err = h.documentService.GetDocumentsByScope(project, tenant)
	} else {
		docs, err = h.documentService.GetAllDocuments()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error obteniendo documentos"})
		return
	}

	c.JSON(http.StatusOK, docs)
}

// GetDocument godoc
// @Summary Obtener documento por ID
// @Tags Documentos
// @Produce json
// @Param id path string true "ID documento"
// @Success 200 {object} models.PDFDocument
// @Failure 404 {object} errorResponse
// @Router /api/v1/documents/{id} [get]
func (h *APIHandler) GetDocument(c *gin.Context) {
	id := c.Param("id")
	doc, err := h.documentService.GetDocument(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Documento no encontrado"})
		return
	}

	c.JSON(http.StatusOK, doc)
}

// DeleteDocument godoc
// @Summary Eliminar documento
// @Tags Documentos
// @Produce json
// @Param id path string true "ID documento"
// @Success 200 {object} okMessageResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/documents/{id} [delete]
func (h *APIHandler) DeleteDocument(c *gin.Context) {
	id := c.Param("id")
	if err := h.documentService.DeleteDocument(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error eliminando documento"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Documento eliminado"})
}

func (h *APIHandler) GetProperties(c *gin.Context) {
	props := models.ServerProperties{
		Endpoint:       h.config.IIIF.BaseURL,
		MaxFileSize:    int(h.config.PDF.MaxFileSize / (1024 * 1024)), // Convert to MB
		AllowedFormats: h.config.PDF.AllowedFormats,
		CacheEnabled:   h.config.IIIF.CacheEnabled,
		CacheTTL:       h.config.IIIF.CacheTTL,
		EnableAuth:     h.config.Security.EnableAuth,
		LogLevel:       h.config.Security.LogLevel,
	}

	c.JSON(http.StatusOK, props)
}

func (h *APIHandler) UpdateProperties(c *gin.Context) {
	var props models.ServerProperties
	if err := c.ShouldBindJSON(&props); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Datos inválidos"})
		return
	}

	// Actualizar configuración (en una implementación real, guardarías esto en archivo)
	h.config.IIIF.BaseURL = props.Endpoint
	h.config.PDF.MaxFileSize = int64(props.MaxFileSize * 1024 * 1024)
	h.config.PDF.AllowedFormats = props.AllowedFormats
	h.config.IIIF.CacheEnabled = props.CacheEnabled
	h.config.IIIF.CacheTTL = props.CacheTTL
	h.config.Security.EnableAuth = props.EnableAuth
	h.config.Security.LogLevel = props.LogLevel

	c.JSON(http.StatusOK, gin.H{"message": "Propiedades actualizadas"})
}

// IIIF Handlers - Formato Cantaloupe

// GetManifest godoc
// @Summary Obtener manifiesto IIIF Presentation API v2
// @Description Devuelve un manifiesto v2 con secuencia, canvases y rangos jerarquicos derivados de los marcadores PDF. El parametro pages permite seleccionar paginas y poda rangos vacios.
// @Tags IIIF
// @Produce json
// @Param id path string true "ID del documento"
// @Param pages query string false "Paginas o rangos, por ejemplo 1-3,5"
// @Success 200 {object} models.IIIFManifestV2
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /api/iiif/{id}/manifest [get]
// @Router /api/v1/iiif/{id}/manifest [get]
func (h *APIHandler) GetManifest(c *gin.Context) {
	id := c.Param("id")
	manifest, err := h.iiifService.GetManifest(id, c.Query("pages"))
	if err != nil {
		var selectionError *services.InvalidPageSelectionError
		if errors.As(err, &selectionError) {
			c.JSON(http.StatusBadRequest, gin.H{"error": selectionError.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Manifiesto no encontrado"})
		return
	}

	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, manifest)
}

// GetManifestV3 godoc
// @Summary Obtener manifiesto IIIF Presentation API v3
// @Description Endpoint de compatibilidad para clientes IIIF Presentation v3.
// @Tags IIIF
// @Produce json
// @Param id path string true "ID del documento"
// @Param pages query string false "Paginas o rangos, por ejemplo 1-3,5"
// @Success 200 {object} models.IIIFManifest
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /api/iiif/v3/{id}/manifest [get]
// @Router /api/v1/iiif/{id}/manifest/v3 [get]
func (h *APIHandler) GetManifestV3(c *gin.Context) {
	id := c.Param("id")
	manifest, err := h.iiifService.GetManifestV3(id, c.Query("pages"))
	if err != nil {
		var selectionError *services.InvalidPageSelectionError
		if errors.As(err, &selectionError) {
			c.JSON(http.StatusBadRequest, gin.H{"error": selectionError.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Manifiesto no encontrado"})
		return
	}
	c.JSON(http.StatusOK, manifest)
}

// GetImageInfoV2 godoc
// @Summary Obtener info IIIF Image API v2
// @Tags IIIF
// @Produce json
// @Param identifier path string true "Identificador IIIF, por ejemplo documento_page_1"
// @Success 200 {object} models.IIIFImageInfoV2
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /iiif/2/{identifier}/info.json [get]
func (h *APIHandler) GetImageInfoV2(c *gin.Context) {
	docID, page, err := h.parseIdentifier(c.Param("identifier"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Identificador inválido"})
		return
	}
	info, err := h.iiifService.GetImageInfoV2(docID, page)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Información de imagen no encontrada"})
		return
	}
	c.JSON(http.StatusOK, info)
}

// GET /iiif/3/{identifier}/info.json
// GetImageInfo godoc
// @Summary Obtener info IIIF
// @Tags IIIF
// @Produce json
// @Param identifier path string true "Identificador IIIF"
// @Success 200 {object} models.IIIFImageInfo
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /iiif/3/{identifier}/info.json [get]
func (h *APIHandler) GetImageInfo(c *gin.Context) {
	identifier := c.Param("identifier")

	// Extraer documento ID y página del identificador
	docID, page, err := h.parseIdentifier(identifier)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Identificador inválido"})
		return
	}

	info, err := h.iiifService.GetImageInfo(docID, page)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Información de imagen no encontrada"})
		return
	}

	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, info)
}

// GET /iiif/3/{identifier}/{region}/{size}/{rotation}/{quality}.{format}
// GetImage godoc
// @Summary Obtener imagen IIIF transformada
// @Tags IIIF
// @Produce image/jpeg,image/png,image/webp
// @Param identifier path string true "Identificador IIIF"
// @Param region path string true "Region"
// @Param size path string true "Tamano"
// @Param rotation path string true "Rotacion"
// @Param quality_format path string true "quality.format"
// @Success 200 {file} file
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /iiif/2/{identifier}/{region}/{size}/{rotation}/{quality_format} [get]
// @Router /iiif/3/{identifier}/{region}/{size}/{rotation}/{quality_format} [get]
func (h *APIHandler) GetImage(c *gin.Context) {
	identifier := c.Param("identifier")
	region := c.Param("region")
	size := c.Param("size")
	rotation := c.Param("rotation")
	qualityFormat := c.Param("quality_format")

	// Parsear quality.format
	parts := strings.Split(qualityFormat, ".")
	if len(parts) != 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Formato inválido"})
		return
	}
	quality := parts[0]
	format := parts[1]

	// Extraer documento ID y página del identificador
	docID, page, err := h.parseIdentifier(identifier)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Identificador inválido"})
		return
	}

	data, contentType, err := h.iiifService.GetImageWithRegion(docID, page, region, size, rotation, quality, format)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Imagen no encontrada"})
		return
	}

	c.Header("Content-Type", contentType)
	c.Data(http.StatusOK, contentType, data)
}

// GET /iiif/3/{identifier}/default.jpg (acceso directo)
func (h *APIHandler) GetImageDefault(c *gin.Context) {
	identifier := c.Param("identifier")

	// Extraer documento ID y página del identificador
	docID, page, err := h.parseIdentifier(identifier)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Identificador inválido"})
		return
	}

	data, contentType, err := h.iiifService.GetImageWithRegion(docID, page, "full", "max", "0", "default", "jpg")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Imagen no encontrada"})
		return
	}

	c.Header("Content-Type", contentType)
	c.Data(http.StatusOK, contentType, data)
}

// parseIdentifier extrae el documento ID y página del identificador
// Soporta múltiples formatos:
// - CJUOWBJGIFFZFOQXLRSEUNKE7M.png (imagen individual, página 1)
// - CJUOWBJGIFFZFOQXLRSEUNKE7M_page_2.png (página específica)
// - documento.pdf_page_3 (PDF con página específica)
// - documento_page_1 (sin extensión)
func (h *APIHandler) parseIdentifier(identifier string) (string, int, error) {
	if strings.Contains(identifier, "~") {
		parts := strings.Split(identifier, "~")
		identifier = parts[len(parts)-1]
	}

	// Guardar la extensión original si existe
	originalExt := ""
	if strings.Contains(identifier, ".") {
		parts := strings.Split(identifier, ".")
		if len(parts) >= 2 {
			originalExt = parts[len(parts)-1]
			// Remover la extensión para el procesamiento
			identifier = strings.TrimSuffix(identifier, "."+originalExt)
		}
	}

	// Buscar patrón _page_
	if strings.Contains(identifier, "_page_") {
		parts := strings.Split(identifier, "_page_")
		if len(parts) == 2 {
			docID := parts[0]
			page, err := strconv.Atoi(parts[1])
			if err != nil {
				return "", 0, fmt.Errorf("página inválida: %s", parts[1])
			}

			// Si el docID original tenía extensión, agregarla de vuelta
			if originalExt != "" && !strings.Contains(docID, ".") {
				docID = docID + "." + originalExt
			}

			return docID, page, nil
		}
	}

	// Si no tiene _page_, asumir página 1
	// Restaurar extensión si existía
	if originalExt != "" {
		identifier = identifier + "." + originalExt
	}

	return identifier, 1, nil
}

func (h *APIHandler) requestScope(c *gin.Context) (*models.Scope, error) {
	project := firstNonEmpty(c.PostForm("project"), c.GetHeader("X-IIIF-Project"))
	tenant := firstNonEmpty(c.PostForm("tenant"), c.GetHeader("X-IIIF-Tenant"))
	return h.config.ResolveScope(project, tenant)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
