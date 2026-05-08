package handlers

import (
	"net/http"
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
	c.File(filepath.Join(h.config.Frontend.Path, "index.html"))
}

func (h *FrontendHandler) Disabled(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "frontend deshabilitado"})
}
