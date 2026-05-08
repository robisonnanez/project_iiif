package handlers

import (
	"net/http"

	"iiif-pdf-server/internal/config"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	config *config.Config
}

func NewAdminHandler(config *config.Config) *AdminHandler {
	return &AdminHandler{config: config}
}

func (h *AdminHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"server": gin.H{
			"port": h.config.Server.Port,
			"mode": h.config.Server.Mode,
		},
		"storage": gin.H{
			"backend":         h.config.Storage.Backend,
			"data_path":       h.config.Storage.DataPath,
			"pdfs_path":       h.config.Storage.PDFsPath,
			"images_path":     h.config.Storage.ImagesPath,
			"documents_path":  h.config.Storage.DocumentsPath,
			"thumbnails_path": h.config.Storage.ThumbnailsPath,
			"manifests_path":  h.config.Storage.ManifestsPath,
		},
		"database": gin.H{
			"DB_CONNECTION": h.config.DBConnection,
			"DB_HOST":       h.config.DBHost,
			"DB_PORT":       h.config.DBPort,
			"DB_DATABASE":   h.config.DBDatabase,
			"DB_USERNAME":   h.config.DBUsername,
			"DB_PASSWORD":   maskedSecret(h.config.DBPassword),
			"mysql": gin.H{
				"host":       h.config.Database.MySQL.Host,
				"port":       h.config.Database.MySQL.Port,
				"user":       h.config.Database.MySQL.User,
				"password":   maskedSecret(h.config.Database.MySQL.Password),
				"database":   h.config.Database.MySQL.Database,
				"charset":    h.config.Database.MySQL.Charset,
				"parse_time": h.config.Database.MySQL.ParseTime,
			},
		},
		"frontend": gin.H{
			"enabled":      h.config.Frontend.Enabled,
			"path":         h.config.Frontend.Path,
			"require_auth": h.config.Frontend.RequireAuth,
			"username":     h.config.Frontend.Username,
			"password":     maskedSecret(h.config.Frontend.Password),
		},
		"iiif": gin.H{
			"base_url":    h.config.IIIF.BaseURL,
			"api_version": h.config.IIIF.APIVersion,
			"max_width":   h.config.IIIF.MaxWidth,
			"max_height":  h.config.IIIF.MaxHeight,
			"cache":       h.config.IIIF.CacheEnabled,
		},
	})
}

func maskedSecret(value string) string {
	if value == "" {
		return ""
	}
	return "********"
}
