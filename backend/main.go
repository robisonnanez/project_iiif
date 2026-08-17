package main

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "iiif-pdf-server/docs"
	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/handlers"
	"iiif-pdf-server/internal/services"
	"iiif-pdf-server/internal/storage"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title IIIF PDF Server API
// @version 2.1
// @description API para conversion de PDF, IIIF Presentation v2 (con v3 compatible), IIIF Image API, administracion y migracion. Las rutas recomendadas usan /api/v1 y los endpoints legacy se mantienen por compatibilidad temporal.
// @BasePath /
// @securityDefinitions.apikey SessionCookie
// @in cookie
// @name project_iiif_session
func main() {
	// Cargar configuración
	configPath := strings.TrimSpace(os.Getenv("CONFIG_PATH"))
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("Error cargando configuración: %v, usando valores por defecto", err)
		cfg = config.Default()
	}

	// Crear directorios necesarios
	createDirectories(cfg)
	if envEnabled("AUTO_MIGRATE") && cfg.Storage.Backend != "local" {
		result, migrationErr := storage.RunDBMigrations(cfg, ".")
		if migrationErr != nil {
			log.Fatalf("Error ejecutando migraciones de %s: %v", result.Engine, migrationErr)
		}
		log.Printf("Migraciones %s: aplicadas=%d omitidas=%d", result.Engine, result.Applied, result.Skipped)
	}

	// Inicializar servicios
	store, err := newStorage(cfg)
	if err != nil {
		log.Fatalf("Error inicializando almacenamiento: %v", err)
	}
	pdfService := services.NewPDFService(cfg, store)
	iiifService := services.NewIIIFService(cfg, store)
	documentService := services.NewDocumentService(store)
	ocrService, err := services.NewOCRService(cfg, store)
	if err != nil {
		log.Fatalf("Error inicializando OCR: %v", err)
	}
	if cfg.OCR.Enabled && cfg.OCR.AutoAfterConversion {
		pdfService.SetCompletionHook(func(documentID string) {
			if _, createErr := ocrService.CreateJob(documentID, services.CreateOCRJobRequest{Mode: cfg.OCR.DefaultMode, LanguageMode: "auto"}); createErr != nil {
				log.Printf("No se pudo iniciar OCR automático document=%s: %v", documentID, createErr)
			}
		})
	}

	// Configurar router
	if cfg.Server.Mode == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Configurar CORS
	corsConfig := cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return isOriginAllowed(origin, cfg.Security.CorsOrigins)
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * 3600, // 12 horas
	}
	router.Use(cors.New(corsConfig))

	// Middleware para servir archivos estáticos
	router.Static("/static", cfg.Storage.DataPath)

	// Inicializar handlers
	apiHandler := handlers.NewAPIHandler(pdfService, iiifService, documentService, cfg)
	welcomeHandler := handlers.NewWelcomeHandler(cfg)
	frontendHandler := handlers.NewFrontendHandler(cfg)
	adminHandler := handlers.NewAdminHandler(cfg, documentService)
	ocrHandler := handlers.NewOCRHandler(ocrService)
	authHandler := handlers.NewAuthHandler(cfg)

	// Ruta de bienvenida
	router.GET("/", welcomeHandler.Welcome)
	router.GET("/health", welcomeHandler.HealthCheck)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/logout", authHandler.Logout)
	router.GET("/auth/me", authHandler.Me)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	if cfg.Frontend.Enabled {
		dashboard := router.Group("/dashboard")
		dashboard.Use(authHandler.RequireSession())
		{
			dashboard.GET("", frontendHandler.Dashboard)
			dashboard.GET("/", frontendHandler.Dashboard)
			dashboard.GET("/inicio", frontendHandler.Dashboard)
			dashboard.GET("/subir-pdf", frontendHandler.Dashboard)
			dashboard.GET("/documentos", frontendHandler.Dashboard)
			dashboard.GET("/imagenes", frontendHandler.Dashboard)
			dashboard.GET("/iiif", frontendHandler.Dashboard)
			dashboard.GET("/configuracion", frontendHandler.Dashboard)
			dashboard.GET("/migracion", frontendHandler.Dashboard)
			dashboard.GET("/ocr", frontendHandler.Dashboard)
			dashboard.Static("/assets", filepath.Join(cfg.Frontend.Path, "dist", "assets"))
		}

		// Expone rutas admin versionadas y mantiene aliases legacy durante la transicion.
		registerAdminRoutes := func(group *gin.RouterGroup) {
			group.GET("/config", adminHandler.GetConfig)
			group.PUT("/config", adminHandler.UpdateConfig)
			group.POST("/service/restart", adminHandler.RestartService)
			group.GET("/projects", adminHandler.GetProjects)
			group.GET("/documents/:id/images", adminHandler.GetDocumentImages)
			group.GET("/migrations/sources/local/browse", adminHandler.BrowseLocalMigrationSource)
			group.POST("/migrations/local-to-db/start", adminHandler.StartLocalToDBMigration)
			group.GET("/migrations/local-to-db/status", adminHandler.GetLocalToDBMigrationStatus)
			group.POST("/migrations/local-to-mysql/start", adminHandler.StartLocalToMySQLMigration)
			group.GET("/migrations/local-to-mysql/status", adminHandler.GetLocalToMySQLMigrationStatus)
			group.POST("/db/migrations/run", adminHandler.RunDBMigrations)
			group.GET("/db/migrations/status", adminHandler.GetDBMigrationsStatus)
			group.POST("/documents/:id/ocr/jobs", ocrHandler.CreateJob)
			group.GET("/documents/:id/ocr", ocrHandler.GetSummary)
			group.GET("/documents/:id/ocr/pages/:page", ocrHandler.GetPage)
			group.GET("/documents/:id/ocr/search", ocrHandler.SearchDocument)
			group.DELETE("/documents/:id/ocr", ocrHandler.Delete)
			group.GET("/ocr/jobs/:job_id", ocrHandler.GetJob)
			group.POST("/ocr/jobs/:job_id/cancel", ocrHandler.CancelJob)
			group.GET("/ocr/search", ocrHandler.Search)
		}

		adminV1 := router.Group("/api/v1/admin")
		adminV1.Use(authHandler.RequireSession())
		registerAdminRoutes(adminV1)

		adminLegacy := router.Group("/admin/api")
		adminLegacy.Use(authHandler.RequireSession())
		registerAdminRoutes(adminLegacy)

	} else {
		router.GET("/dashboard", frontendHandler.Disabled)
		router.GET("/dashboard/*path", frontendHandler.Disabled)
	}

	// Rutas API de gestión
	registerDocumentRoutes := func(group *gin.RouterGroup) {
		group.GET("", apiHandler.GetDocuments)
		group.GET("/:id", apiHandler.GetDocument)
		group.DELETE("/:id", apiHandler.DeleteDocument)
	}

	documentV1 := router.Group("/api/v1/documents")
	{
		documentV1.POST("/upload", apiHandler.UploadPDF)
		registerDocumentRoutes(documentV1)
	}

	legacyAPI := router.Group("/api")
	{
		legacyAPI.POST("/upload", apiHandler.UploadPDF)
		legacyAPI.GET("/documents", apiHandler.GetDocuments)
		legacyAPI.GET("/documents/:id", apiHandler.GetDocument)
		legacyAPI.DELETE("/documents/:id", apiHandler.DeleteDocument)
		legacyAPI.GET("/properties", apiHandler.GetProperties)
		legacyAPI.PUT("/properties", apiHandler.UpdateProperties)
	}

	// Rutas IIIF estilo Cantaloupe
	// Formato: /iiif/{version}/{identifier}/{region}/{size}/{rotation}/{quality}.{format}
	iiifGroup := router.Group("/iiif")
	{
		// IIIF Image API v2, used by Presentation API v2 manifests.
		v2 := iiifGroup.Group("/2")
		{
			v2.GET("/:identifier/info.json", apiHandler.GetImageInfoV2)
			v2.GET("/:identifier/:region/:size/:rotation/:quality_format", apiHandler.GetImage)
			v2.GET("/:identifier/default.jpg", apiHandler.GetImageDefault)
		}

		// IIIF Image API v3
		v3 := iiifGroup.Group("/" + cfg.IIIF.APIVersion)
		{
			// Info.json: GET /iiif/3/{identifier}/info.json
			v3.GET("/:identifier/info.json", apiHandler.GetImageInfo)

			// Imagen completa: GET /iiif/3/{identifier}/{region}/{size}/{rotation}/{quality}.{format}
			v3.GET("/:identifier/:region/:size/:rotation/:quality_format", apiHandler.GetImage)

			// Acceso directo: GET /iiif/3/{identifier}/default.jpg
			v3.GET("/:identifier/default.jpg", apiHandler.GetImageDefault)
		}
	}

	// Rutas de manifiestos (mantener compatibilidad)
	legacyAPI.GET("/iiif/:id/manifest", apiHandler.GetManifest)
	legacyAPI.GET("/iiif/v3/:id/manifest", apiHandler.GetManifestV3)

	// Iniciar servidor
	log.Printf("Servidor IIIF iniciado en puerto %s", cfg.Server.Port)
	log.Printf("URLs de ejemplo para CJUOWBJGIFFZFOQXLRSEUNKE7M.png:")
	log.Printf("  Info: http://localhost:%s/iiif/3/CJUOWBJGIFFZFOQXLRSEUNKE7M.png/info.json", cfg.Server.Port)
	log.Printf("  Imagen: http://localhost:%s/iiif/3/CJUOWBJGIFFZFOQXLRSEUNKE7M.png/0,0,200,200/max/0/default.jpg", cfg.Server.Port)
	log.Printf("  Región: http://localhost:%s/iiif/3/CJUOWBJGIFFZFOQXLRSEUNKE7M.png/full/800,/0/default.jpg", cfg.Server.Port)
	log.Printf("  Acceso directo: http://localhost:%s/iiif/3/CJUOWBJGIFFZFOQXLRSEUNKE7M.png/default.jpg", cfg.Server.Port)
	log.Printf("")
	log.Printf("URLs de ejemplo para documento.pdf:")
	log.Printf("  Info página 1: http://localhost:%s/iiif/3/documento.pdf_page_1/info.json", cfg.Server.Port)
	log.Printf("  Imagen página 2: http://localhost:%s/iiif/3/documento.pdf_page_2/full/800,/0/default.jpg", cfg.Server.Port)

	log.Fatal(router.Run(":" + cfg.Server.Port))
}

func envEnabled(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func createDirectories(cfg *config.Config) {
	dirs := []string{
		cfg.Storage.DataPath,
		cfg.Storage.PDFsPath,
		cfg.Storage.DocumentsPath,
		cfg.Storage.ImagesPath,
		cfg.Storage.ThumbnailsPath,
		cfg.Storage.ManifestsPath,
		cfg.PDF.TempPath,
		cfg.BinaryStorage.TempPath,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Error creando directorio %s: %v", dir, err)
		}
	}
}

func newStorage(cfg *config.Config) (storage.Storage, error) {
	return storage.NewConfiguredStorage(cfg)
}

// isOriginAllowed verifica si un origen está permitido, incluyendo wildcards
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		// Coincidencia exacta
		if origin == allowed {
			return true
		}

		// Verificar wildcards
		if strings.Contains(allowed, "*") {
			// Convertir patrón wildcard a regex
			pattern := strings.ReplaceAll(allowed, "*", "([a-zA-Z0-9-]+)")
			pattern = strings.ReplaceAll(pattern, ".", "\\.")
			pattern = "^" + pattern + "$"

			if matched, err := regexp.MatchString(pattern, origin); err == nil && matched {
				return true
			}
		}
	}
	return false
}
