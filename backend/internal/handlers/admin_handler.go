package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/services"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	config          *config.Config
	documentService *services.DocumentService
	migrationRunner *migrationRunner
}

func NewAdminHandler(config *config.Config, documentService *services.DocumentService) *AdminHandler {
	return &AdminHandler{
		config:          config,
		documentService: documentService,
		migrationRunner: newMigrationRunner(config.Migration.MaxLogLines),
	}
}

func (h *AdminHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.sanitizedConfig())
}

func (h *AdminHandler) GetProjects(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":               h.config.Projects.Enabled,
		"default_project":       h.config.Projects.DefaultProject,
		"require_project":       h.config.Projects.RequireProject,
		"allow_dynamic_tenants": h.config.Projects.AllowDynamicTenants,
		"items":                 h.config.Projects.Items,
	})
}

func (h *AdminHandler) UpdateConfig(c *gin.Context) {
	// Guarda solo campos permitidos del formulario para evitar sobrescribir secretos o YAML arbitrario.
	var payload editableConfigPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "configuracion invalida"})
		return
	}

	next := *h.config
	if err := applyEditableConfig(&next, h.config, payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	next.SourcePath = h.config.SourcePath
	next.ApplyDefaults()

	configPath := next.SourcePath
	if configPath == "" {
		configPath = "config.yaml"
	}
	if err := config.Save(configPath, &next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no se pudo guardar config.yaml: " + err.Error()})
		return
	}

	*h.config = next
	c.JSON(http.StatusOK, gin.H{
		"message":          "Configuracion guardada. Reinicia el servicio para aplicar cambios sensibles.",
		"requires_restart": true,
		"config":           h.sanitizedConfig(),
	})
}

func (h *AdminHandler) GetDocumentImages(c *gin.Context) {
	// Expone identificadores IIIF seguros para la galeria sin revelar rutas internas ni BLOBs.
	documentID := c.Param("id")
	doc, err := h.documentService.GetDocument(documentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Documento no encontrado"})
		return
	}

	images, err := h.documentService.GetDocumentImages(documentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error obteniendo imagenes"})
		return
	}

	base := strings.TrimRight(h.config.IIIF.BaseURL, "/")
	items := make([]gin.H, 0, len(images))
	for _, image := range images {
		identifier := image.ID
		servicePath := "/iiif/" + h.config.IIIF.APIVersion + "/" + identifier
		items = append(items, gin.H{
			"image_id":    identifier,
			"document_id": image.DocumentID,
			"project_key": image.ProjectKey,
			"tenant_key":  image.TenantKey,
			"page_number": image.PageNumber,
			"width":       image.Width,
			"height":      image.Height,
			"format":      image.Format,
			"media_type":  image.MediaType,
			"byte_size":   image.ByteSize,
			"iiif_url":    base + servicePath + "/full/max/0/default.jpg",
			"info_url":    base + servicePath + "/info.json",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"document_id": documentID,
		"project_key": doc.ProjectKey,
		"tenant_key":  doc.TenantKey,
		"images":      items,
	})
}

func (h *AdminHandler) StartLocalToMySQLMigration(c *gin.Context) {
	// Ejecuta la migracion en background y devuelve estado inicial.
	var payload migrationStartRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload invalido"})
		return
	}
	if err := validateMigrationRequest(payload, h.config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.migrationRunner.Start(payload); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Migracion iniciada",
		"status":  h.migrationRunner.Status(),
	})
}

func (h *AdminHandler) GetLocalToMySQLMigrationStatus(c *gin.Context) {
	// Devuelve estado y logs acumulados de la ultima migracion.
	c.JSON(http.StatusOK, h.migrationRunner.Status())
}

func (h *AdminHandler) BrowseLocalMigrationSource(c *gin.Context) {
	// Lista directorios hijos para ayudar a seleccionar ruta local de migracion.
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		path = h.config.Storage.DataPath
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ruta invalida"})
		return
	}
	if !h.isAllowedLocalPath(resolved) {
		c.JSON(http.StatusForbidden, gin.H{"error": "ruta fuera de los directorios permitidos"})
		return
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no se pudo leer la ruta: " + err.Error()})
		return
	}

	dirs := []gin.H{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(resolved, e.Name())
		if !h.isAllowedLocalPath(child) {
			continue
		}
		dirs = append(dirs, gin.H{
			"name": e.Name(),
			"path": child,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"path": path,
		"dirs": dirs,
	})
}

func (h *AdminHandler) isAllowedLocalPath(path string) bool {
	path = filepath.Clean(path)
	for _, root := range h.config.Migration.AllowedLocalRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absRoot = filepath.Clean(absRoot)
		if path == absRoot || strings.HasPrefix(path, absRoot+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func validateMigrationRequest(req migrationStartRequest, cfg *config.Config) error {
	sourceType := strings.ToLower(strings.TrimSpace(req.Source.Type))
	if sourceType != "local" && sourceType != "ssh" {
		return &configError{"source.type debe ser local o ssh"}
	}
	if sourceType == "local" {
		if strings.TrimSpace(req.Source.Local.Path) == "" {
			return &configError{"source.local.path es obligatorio"}
		}
		return nil
	}
	if strings.TrimSpace(req.Source.SSH.Host) == "" || strings.TrimSpace(req.Source.SSH.User) == "" {
		return &configError{"source.ssh.host y source.ssh.user son obligatorios"}
	}
	if strings.TrimSpace(req.Source.SSH.Path) == "" {
		return &configError{"source.ssh.path es obligatorio"}
	}
	if strings.TrimSpace(req.Source.SSH.PrivateKey) == "" {
		return &configError{"source.ssh.private_key es obligatoria"}
	}
	if len(cfg.Migration.SSH.AllowedHosts) > 0 {
		allowed := false
		for _, host := range cfg.Migration.SSH.AllowedHosts {
			if strings.EqualFold(strings.TrimSpace(host), strings.TrimSpace(req.Source.SSH.Host)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &configError{"host SSH no permitido por migration.ssh.allowed_hosts"}
		}
	}
	return nil
}

func (h *AdminHandler) sanitizedConfig() gin.H {
	return gin.H{
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
		"binary_storage": gin.H{
			"mode":      h.config.BinaryStorage.Mode,
			"temp_path": h.config.BinaryStorage.TempPath,
		},
		"projects": gin.H{
			"enabled":               h.config.Projects.Enabled,
			"default_project":       h.config.Projects.DefaultProject,
			"require_project":       h.config.Projects.RequireProject,
			"allow_dynamic_tenants": h.config.Projects.AllowDynamicTenants,
			"items":                 h.config.Projects.Items,
		},
		"iiif": gin.H{
			"base_url":    h.config.IIIF.BaseURL,
			"api_version": h.config.IIIF.APIVersion,
			"max_width":   h.config.IIIF.MaxWidth,
			"max_height":  h.config.IIIF.MaxHeight,
			"cache":       h.config.IIIF.CacheEnabled,
		},
	}
}

func maskedSecret(value string) string {
	if value == "" {
		return ""
	}
	return "********"
}

type editableConfigPayload struct {
	Server struct {
		Port string `json:"port"`
		Mode string `json:"mode"`
	} `json:"server"`
	Storage struct {
		Backend        string `json:"backend"`
		DataPath       string `json:"data_path"`
		PDFsPath       string `json:"pdfs_path"`
		ImagesPath     string `json:"images_path"`
		DocumentsPath  string `json:"documents_path"`
		ThumbnailsPath string `json:"thumbnails_path"`
		ManifestsPath  string `json:"manifests_path"`
	} `json:"storage"`
	Database struct {
		DBConnection string `json:"DB_CONNECTION"`
		DBHost       string `json:"DB_HOST"`
		DBPort       string `json:"DB_PORT"`
		DBDatabase   string `json:"DB_DATABASE"`
		DBUsername   string `json:"DB_USERNAME"`
		DBPassword   string `json:"DB_PASSWORD"`
		MySQL        struct {
			Host      string `json:"host"`
			Port      string `json:"port"`
			User      string `json:"user"`
			Password  string `json:"password"`
			Database  string `json:"database"`
			Charset   string `json:"charset"`
			ParseTime bool   `json:"parse_time"`
		} `json:"mysql"`
	} `json:"database"`
	Frontend struct {
		Enabled     bool   `json:"enabled"`
		Path        string `json:"path"`
		RequireAuth bool   `json:"require_auth"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	} `json:"frontend"`
	BinaryStorage struct {
		Mode     string `json:"mode"`
		TempPath string `json:"temp_path"`
	} `json:"binary_storage"`
	IIIF struct {
		BaseURL      string `json:"base_url"`
		APIVersion   string `json:"api_version"`
		MaxWidth     int    `json:"max_width"`
		MaxHeight    int    `json:"max_height"`
		CacheEnabled bool   `json:"cache"`
		CacheTTL     int    `json:"cache_ttl"`
	} `json:"iiif"`
	Projects struct {
		Enabled             bool                   `json:"enabled"`
		DefaultProject      string                 `json:"default_project"`
		RequireProject      bool                   `json:"require_project"`
		AllowDynamicTenants bool                   `json:"allow_dynamic_tenants"`
		Items               []config.ProjectConfig `json:"items"`
	} `json:"projects"`
}

func applyEditableConfig(next, current *config.Config, payload editableConfigPayload) error {
	if err := validatePort(payload.Server.Port, "server.port"); err != nil {
		return err
	}
	if err := validatePort(payload.Database.DBPort, "DB_PORT"); err != nil {
		return err
	}
	if err := validatePort(payload.Database.MySQL.Port, "database.mysql.port"); err != nil {
		return err
	}
	if !allowedValue(payload.Storage.Backend, "local", "mysql", "postgres", "postgresql", "mongo", "mongodb") {
		return &configError{"storage.backend debe ser local, mysql, postgres, postgresql, mongo o mongodb"}
	}
	if !allowedValue(payload.BinaryStorage.Mode, "local", "database") {
		return &configError{"binary_storage.mode debe ser local o database"}
	}
	if payload.IIIF.MaxWidth <= 0 || payload.IIIF.MaxHeight <= 0 {
		return &configError{"iiif.max_width y iiif.max_height deben ser mayores que cero"}
	}

	next.Server.Port = payload.Server.Port
	next.Server.Mode = payload.Server.Mode
	next.Storage.Backend = payload.Storage.Backend
	next.Storage.DataPath = payload.Storage.DataPath
	next.Storage.PDFsPath = payload.Storage.PDFsPath
	next.Storage.ImagesPath = payload.Storage.ImagesPath
	next.Storage.DocumentsPath = payload.Storage.DocumentsPath
	next.Storage.ThumbnailsPath = payload.Storage.ThumbnailsPath
	next.Storage.ManifestsPath = payload.Storage.ManifestsPath

	next.Database.MySQL.Host = payload.Database.MySQL.Host
	next.Database.MySQL.Port = payload.Database.MySQL.Port
	next.Database.MySQL.User = payload.Database.MySQL.User
	next.Database.MySQL.Password = secretOrCurrent(payload.Database.MySQL.Password, current.Database.MySQL.Password)
	next.Database.MySQL.Database = payload.Database.MySQL.Database
	next.Database.MySQL.Charset = payload.Database.MySQL.Charset
	next.Database.MySQL.ParseTime = payload.Database.MySQL.ParseTime

	next.DBConnection = payload.Database.DBConnection
	next.DBHost = payload.Database.DBHost
	next.DBPort = payload.Database.DBPort
	next.DBDatabase = payload.Database.DBDatabase
	next.DBUsername = payload.Database.DBUsername
	next.DBPassword = secretOrCurrent(payload.Database.DBPassword, current.DBPassword)

	next.Frontend.Enabled = payload.Frontend.Enabled
	next.Frontend.Path = payload.Frontend.Path
	next.Frontend.RequireAuth = payload.Frontend.RequireAuth
	next.Frontend.Username = payload.Frontend.Username
	next.Frontend.Password = secretOrCurrent(payload.Frontend.Password, current.Frontend.Password)
	next.BinaryStorage.Mode = payload.BinaryStorage.Mode
	next.BinaryStorage.TempPath = payload.BinaryStorage.TempPath
	next.IIIF.BaseURL = payload.IIIF.BaseURL
	next.IIIF.APIVersion = payload.IIIF.APIVersion
	next.IIIF.MaxWidth = payload.IIIF.MaxWidth
	next.IIIF.MaxHeight = payload.IIIF.MaxHeight
	next.IIIF.CacheEnabled = payload.IIIF.CacheEnabled
	next.IIIF.CacheTTL = payload.IIIF.CacheTTL
	next.Projects.Enabled = payload.Projects.Enabled
	next.Projects.DefaultProject = payload.Projects.DefaultProject
	next.Projects.RequireProject = payload.Projects.RequireProject
	next.Projects.AllowDynamicTenants = payload.Projects.AllowDynamicTenants
	next.Projects.Items = payload.Projects.Items
	if next.Projects.DefaultProject == "" {
		next.Projects.DefaultProject = "default"
	}
	if len(next.Projects.Items) == 0 {
		next.Projects.Items = []config.ProjectConfig{{Key: "default", Name: "Proyecto por defecto", Multitenant: false, Tenants: []string{}}}
	}
	return nil
}

func validatePort(value, field string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return &configError{field + " debe ser un puerto entre 1 y 65535"}
	}
	return nil
}

func allowedValue(value string, allowed ...string) bool {
	for _, item := range allowed {
		if strings.EqualFold(value, item) {
			return true
		}
	}
	return false
}

func secretOrCurrent(value, current string) string {
	if value == maskedSecret(current) {
		return current
	}
	return value
}

type configError struct {
	message string
}

func (e *configError) Error() string {
	return e.message
}
