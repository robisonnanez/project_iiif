package handlers

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/services"
	"iiif-pdf-server/internal/storage"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	config          *config.Config
	documentService *services.DocumentService
	migrationRunner *migrationRunner
	dbMigrateMu     sync.Mutex
	dbMigrateRunning bool
	dbMigrateStatus storage.MigrationRunResult
	restartMu       sync.Mutex
	lastRestartByIP map[string]time.Time
}

func NewAdminHandler(config *config.Config, documentService *services.DocumentService) *AdminHandler {
	return &AdminHandler{
		config:          config,
		documentService: documentService,
		migrationRunner: newMigrationRunner(config.Migration.MaxLogLines),
		dbMigrateStatus: storage.MigrationRunResult{Engine: config.Storage.Backend, Message: "sin ejecutar"},
		lastRestartByIP: map[string]time.Time{},
	}
}

// GetConfig godoc
// @Summary Obtener configuracion saneada
// @Tags Admin
// @Security SessionCookie
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} errorResponse
// @Router /admin/api/config [get]
func (h *AdminHandler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.sanitizedConfig())
}

// GetDBMigrationsStatus godoc
// @Summary Estado de migraciones de base de datos
// @Tags Admin
// @Security SessionCookie
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} errorResponse
// @Router /admin/api/db/migrations/status [get]
func (h *AdminHandler) GetDBMigrationsStatus(c *gin.Context) {
	h.dbMigrateMu.Lock()
	defer h.dbMigrateMu.Unlock()
	c.JSON(http.StatusOK, gin.H{
		"running": h.dbMigrateRunning,
		"result":  h.dbMigrateStatus,
	})
}

// RunDBMigrations godoc
// @Summary Ejecutar migraciones pendientes del motor activo
// @Tags Admin
// @Security SessionCookie
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /admin/api/db/migrations/run [post]
func (h *AdminHandler) RunDBMigrations(c *gin.Context) {
	h.dbMigrateMu.Lock()
	if h.dbMigrateRunning {
		h.dbMigrateMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "ya hay una ejecucion de migraciones en curso"})
		return
	}
	h.dbMigrateRunning = true
	h.dbMigrateMu.Unlock()

	result, err := storage.RunDBMigrations(h.config, ".")
	h.dbMigrateMu.Lock()
	h.dbMigrateStatus = result
	h.dbMigrateRunning = false
	h.dbMigrateMu.Unlock()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "no se pudieron ejecutar migraciones",
			"result": result,
		})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetProjects godoc
// @Summary Listar proyectos y tenants
// @Tags Admin
// @Security SessionCookie
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} errorResponse
// @Router /admin/api/projects [get]
func (h *AdminHandler) GetProjects(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":               h.config.Projects.Enabled,
		"default_project":       h.config.Projects.DefaultProject,
		"require_project":       h.config.Projects.RequireProject,
		"allow_dynamic_tenants": h.config.Projects.AllowDynamicTenants,
		"items":                 h.config.Projects.Items,
	})
}

// RestartService godoc
// @Summary Programar reinicio del servicio
// @Description Valida password sudo y programa reinicio asincrono de project-iiif.
// @Tags Admin
// @Security SessionCookie
// @Accept json
// @Produce json
// @Param request body restartServiceRequest true "Password sudo"
// @Success 200 {object} restartServiceResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 429 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /admin/api/service/restart [post]
func (h *AdminHandler) RestartService(c *gin.Context) {
	// Programa un reinicio asíncrono de project-iiif para evitar cortar la respuesta HTTP al frontend.
	var payload struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "payload invalido"})
		return
	}
	if strings.TrimSpace(payload.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "password obligatoria"})
		return
	}

	clientIP := strings.TrimSpace(c.ClientIP())
	h.restartMu.Lock()
	if last, ok := h.lastRestartByIP[clientIP]; ok && time.Since(last) < 10*time.Second {
		h.restartMu.Unlock()
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "error": "espera unos segundos antes de reintentar"})
		return
	}
	h.lastRestartByIP[clientIP] = time.Now()
	h.restartMu.Unlock()

	log.Printf("INFO restart request received for service project-iiif from ip=%s", clientIP)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	// Valida credenciales sudo antes de programar el reinicio.
	if err := runSudoSystemctl(ctx, payload.Password, "-k", "true"); err != nil {
		log.Printf("ERROR service restart precheck failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":     false,
			"error":  "no se pudo validar permisos para reiniciar",
			"details": sanitizeCommandError(err.Error()),
		})
		return
	}

	// Ejecuta restart después de responder para que el fetch no pierda conexión.
	password := payload.Password
	time.AfterFunc(1500*time.Millisecond, func() {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer bgCancel()
		if err := runSudoSystemctl(bgCtx, password, "-k", "systemctl", "restart", "project-iiif"); err != nil {
			log.Printf("ERROR deferred service restart failed: %v", err)
			return
		}
		log.Printf("INFO service project-iiif restart scheduled and executed")
	})

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "reinicio programado; el servicio se reiniciara en breve",
		"active":  true,
	})
}

// UpdateConfig godoc
// @Summary Guardar configuracion editable
// @Tags Admin
// @Security SessionCookie
// @Accept json
// @Produce json
// @Param request body editableConfigPayload true "Configuracion editable"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /admin/api/config [put]
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

func runSudoSystemctl(ctx context.Context, password string, args ...string) error {
	cmdArgs := append([]string{"-S"}, args...)
	cmd := exec.CommandContext(ctx, "sudo", cmdArgs...)
	cmd.Stdin = strings.NewReader(password + "\n")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &configError{message: strings.TrimSpace(stderr.String() + " " + out.String() + " " + err.Error())}
	}
	return nil
}

func sanitizeCommandError(message string) string {
	clean := strings.ReplaceAll(message, "\n", " ")
	clean = strings.ReplaceAll(clean, "\r", " ")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return "error de ejecucion"
	}
	if len(clean) > 220 {
		return clean[:220] + "..."
	}
	return clean
}

// GetDocumentImages godoc
// @Summary Listar imagenes IIIF por documento
// @Tags Admin
// @Security SessionCookie
// @Produce json
// @Param id path string true "ID documento"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /admin/api/documents/{id}/images [get]
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
			"migrated_from_local": image.MigratedFromLocal,
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

// StartLocalToDBMigration godoc
// @Summary Iniciar migracion local/ssh a base de datos activa (MySQL/Postgres)
// @Tags Admin
// @Security SessionCookie
// @Accept json
// @Produce json
// @Param request body migrationStartPayload true "Origen y scope"
// @Success 202 {object} map[string]interface{}
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Router /admin/api/migrations/local-to-db/start [post]
func (h *AdminHandler) StartLocalToDBMigration(c *gin.Context) {
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

// GetLocalToDBMigrationStatus godoc
// @Summary Estado de migracion local/ssh a base de datos activa (MySQL/Postgres)
// @Tags Admin
// @Security SessionCookie
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} errorResponse
// @Router /admin/api/migrations/local-to-db/status [get]
func (h *AdminHandler) GetLocalToDBMigrationStatus(c *gin.Context) {
	// Devuelve estado y logs acumulados de la ultima migracion.
	c.JSON(http.StatusOK, h.migrationRunner.Status())
}

// StartLocalToMySQLMigration mantiene compatibilidad con clientes antiguos.
func (h *AdminHandler) StartLocalToMySQLMigration(c *gin.Context) {
	h.StartLocalToDBMigration(c)
}

// GetLocalToMySQLMigrationStatus mantiene compatibilidad con clientes antiguos.
func (h *AdminHandler) GetLocalToMySQLMigrationStatus(c *gin.Context) {
	h.GetLocalToDBMigrationStatus(c)
}

// BrowseLocalMigrationSource godoc
// @Summary Explorar directorios locales permitidos para migracion
// @Tags Admin
// @Security SessionCookie
// @Produce json
// @Param path query string false "Ruta base a explorar"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Router /admin/api/migrations/sources/local/browse [get]
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
	} else {
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
	}

	project := strings.TrimSpace(req.Scope.ProjectKey)
	tenant := strings.TrimSpace(req.Scope.TenantKey)
	if project == "" {
		return &configError{"scope.project_key es obligatorio"}
	}
	item, ok := cfg.ProjectByKey(project)
	if !ok {
		return &configError{"scope.project_key no existe en projects.items"}
	}
	if item.Multitenant {
		if tenant == "" {
			return &configError{"scope.tenant_key es obligatorio para proyecto multitenant"}
		}
		if !cfg.Projects.AllowDynamicTenants {
			found := false
			for _, t := range item.Tenants {
				if strings.EqualFold(strings.TrimSpace(t), tenant) {
					found = true
					break
				}
			}
			if !found {
				return &configError{"scope.tenant_key no permitido para el proyecto"}
			}
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
			"postgres": gin.H{
				"host":     h.config.Database.Postgres.Host,
				"port":     h.config.Database.Postgres.Port,
				"user":     h.config.Database.Postgres.User,
				"password": maskedSecret(h.config.Database.Postgres.Password),
				"database": h.config.Database.Postgres.Database,
				"sslmode":  h.config.Database.Postgres.SSLMode,
				"schema":   h.config.Database.Postgres.Schema,
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
		Postgres struct {
			Host     string `json:"host"`
			Port     string `json:"port"`
			User     string `json:"user"`
			Password string `json:"password"`
			Database string `json:"database"`
			SSLMode  string `json:"sslmode"`
			Schema   string `json:"schema"`
		} `json:"postgres"`
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
	if err := validatePort(payload.Database.Postgres.Port, "database.postgres.port"); err != nil {
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
	next.Database.Postgres.Host = payload.Database.Postgres.Host
	next.Database.Postgres.Port = payload.Database.Postgres.Port
	next.Database.Postgres.User = payload.Database.Postgres.User
	next.Database.Postgres.Password = secretOrCurrent(payload.Database.Postgres.Password, current.Database.Postgres.Password)
	next.Database.Postgres.Database = payload.Database.Postgres.Database
	next.Database.Postgres.SSLMode = payload.Database.Postgres.SSLMode
	next.Database.Postgres.Schema = payload.Database.Postgres.Schema

	next.DBConnection = payload.Database.DBConnection
	next.DBHost = payload.Database.DBHost
	next.DBPort = payload.Database.DBPort
	next.DBDatabase = payload.Database.DBDatabase
	next.DBUsername = payload.Database.DBUsername
	next.DBPassword = secretOrCurrent(payload.Database.DBPassword, current.DBPassword)
	if strings.EqualFold(next.Storage.Backend, "postgres") || strings.EqualFold(next.DBConnection, "postgres") {
		next.DBHost = next.Database.Postgres.Host
		next.DBPort = next.Database.Postgres.Port
		next.DBDatabase = next.Database.Postgres.Database
		next.DBUsername = next.Database.Postgres.User
		next.DBPassword = next.Database.Postgres.Password
	} else {
		next.DBHost = next.Database.MySQL.Host
		next.DBPort = next.Database.MySQL.Port
		next.DBDatabase = next.Database.MySQL.Database
		next.DBUsername = next.Database.MySQL.User
		next.DBPassword = next.Database.MySQL.Password
	}

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
