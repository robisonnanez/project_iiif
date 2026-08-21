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

func NewFrontendHandler(config *config.Config) *FrontendHandler {
	return &FrontendHandler{config: config}
}

func (h *FrontendHandler) Dashboard(c *gin.Context) {
	indexPath := filepath.Join(h.config.Frontend.Path, "dist", "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "frontend no compilado; ejecuta npm run build"})
		return
	}
	c.File(indexPath)
}

func (h *FrontendHandler) Disabled(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "frontend deshabilitado"})
}
