package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	var settings models.ConversionSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		// Usar valores por defecto
		settings = models.ConversionSettings{
			Format:    h.config.Conversion.DefaultFormat,
			Quality:   h.config.Conversion.DefaultQuality,
			MaxWidth:  h.config.IIIF.MaxWidth,
			MaxHeight: h.config.IIIF.MaxHeight,
			EnableOCR: h.config.Conversion.EnableOCR,
		}
	}

	// Guardar archivo temporal
	tempPath := filepath.Join(h.config.PDF.TempPath, "temp_"+header.Filename)
	tempFile, err := os.Create(tempPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error guardando archivo"})
		return
	}
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error copiando archivo"})
		return
	}

	// Procesar PDF
	doc, err := h.pdfService.ProcessPDF(tempPath, header.Filename, settings)
	if err != nil {
		os.Remove(tempPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error procesando PDF"})
		return
	}

	c.JSON(http.StatusOK, doc)
}

func (h *APIHandler) GetDocuments(c *gin.Context) {
	docs, err := h.documentService.GetAllDocuments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error obteniendo documentos"})
		return
	}

	c.JSON(http.StatusOK, docs)
}

func (h *APIHandler) GetDocument(c *gin.Context) {
	id := c.Param("id")
	doc, err := h.documentService.GetDocument(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Documento no encontrado"})
		return
	}

	c.JSON(http.StatusOK, doc)
}

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

func (h *APIHandler) GetManifest(c *gin.Context) {
	id := c.Param("id")
	manifest, err := h.iiifService.GetManifest(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Manifiesto no encontrado"})
		return
	}

	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, manifest)
}

// GET /iiif/3/{identifier}/info.json
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
