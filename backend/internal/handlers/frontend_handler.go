package handlers

import (
	"net/http"
	"os"
	"path/filepath"

	"iiif-pdf-server/internal/config"

	"github.com/gin-gonic/gin"
)

type FrontendHandler struct {
	config *config.Config
}

// NewFrontendHandler crea e inicializa la dependencia con una configuración válida.
func NewFrontendHandler(config *config.Config) *FrontendHandler {
	return &FrontendHandler{config: config}
}

// Dashboard encapsula esta operación interna y conserva las invariantes del componente.
func (h *FrontendHandler) Dashboard(c *gin.Context) {
	indexPath := filepath.Join(h.config.Frontend.Path, "dist", "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		writeJSON(c, http.StatusServiceUnavailable, gin.H{"error": "frontend no compilado; ejecuta npm run build"})
		return
	}
	c.File(indexPath)
}

// Disabled encapsula esta operación interna y conserva las invariantes del componente.
func (h *FrontendHandler) Disabled(c *gin.Context) {
	writeJSON(c, http.StatusNotFound, gin.H{"error": "frontend deshabilitado"})
}
