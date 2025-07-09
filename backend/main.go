package main

import (
	"log"
	"os"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/handlers"
	"iiif-pdf-server/internal/services"
	"iiif-pdf-server/internal/storage"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Cargar configuración
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Printf("Error cargando configuración: %v, usando valores por defecto", err)
		cfg = config.Default()
	}

	// Crear directorios necesarios
	createDirectories(cfg)

	// Inicializar servicios
	storage := storage.NewFileStorage(cfg.Storage.DataPath)
	pdfService := services.NewPDFService(cfg, storage)
	iiifService := services.NewIIIFService(cfg, storage)
	documentService := services.NewDocumentService(storage)

	// Configurar router
	if cfg.Server.Mode == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Configurar CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = cfg.Security.CorsOrigins
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	router.Use(cors.New(corsConfig))

	// Middleware para servir archivos estáticos
	router.Static("/static", cfg.Storage.DataPath)

	// Inicializar handlers
	apiHandler := handlers.NewAPIHandler(pdfService, iiifService, documentService, cfg)
	welcomeHandler := handlers.NewWelcomeHandler(cfg)

	// Ruta de bienvenida
	router.GET("/", welcomeHandler.Welcome)
	router.GET("/health", welcomeHandler.HealthCheck)

	// Rutas API de gestión
	api := router.Group("/api")
	{
		api.POST("/upload", apiHandler.UploadPDF)
		api.GET("/documents", apiHandler.GetDocuments)
		api.GET("/documents/:id", apiHandler.GetDocument)
		api.DELETE("/documents/:id", apiHandler.DeleteDocument)
		api.GET("/properties", apiHandler.GetProperties)
		api.PUT("/properties", apiHandler.UpdateProperties)
	}

	// Rutas IIIF estilo Cantaloupe
	// Formato: /iiif/{version}/{identifier}/{region}/{size}/{rotation}/{quality}.{format}
	iiifGroup := router.Group("/iiif")
	{
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
	api.GET("/iiif/:id/manifest", apiHandler.GetManifest)

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

func createDirectories(cfg *config.Config) {
	dirs := []string{
		cfg.Storage.DataPath,
		cfg.Storage.DocumentsPath,
		cfg.Storage.ImagesPath,
		cfg.Storage.ThumbnailsPath,
		cfg.Storage.ManifestsPath,
		cfg.PDF.TempPath,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Error creando directorio %s: %v", dir, err)
		}
	}
}
