package handlers

import (
	"fmt"
	"iiif-pdf-server/internal/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

type WelcomeHandler struct {
	config *config.Config
}

func NewWelcomeHandler(config *config.Config) *WelcomeHandler {
	return &WelcomeHandler{
		config: config,
	}
}

func (h *WelcomeHandler) Welcome(c *gin.Context) {
	html := `
<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Servidor IIIF PDF - Bienvenida</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #333;
        }
        
        .container {
            background: white;
            border-radius: 20px;
            padding: 3rem;
            box-shadow: 0 20px 40px rgba(0,0,0,0.1);
            text-align: center;
            max-width: 600px;
            width: 90%;
        }
        
        .logo {
            width: 80px;
            height: 80px;
            background: linear-gradient(135deg, #667eea, #764ba2);
            border-radius: 20px;
            margin: 0 auto 2rem;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 2rem;
            color: white;
            font-weight: bold;
        }
        
        h1 {
            font-size: 2.5rem;
            margin-bottom: 1rem;
            background: linear-gradient(135deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .subtitle {
            font-size: 1.2rem;
            color: #666;
            margin-bottom: 2rem;
        }
        
        .info-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 1.5rem;
            margin: 2rem 0;
        }
        
        .info-card {
            background: #f8f9fa;
            padding: 1.5rem;
            border-radius: 12px;
            border-left: 4px solid #667eea;
        }
        
        .info-card h3 {
            color: #333;
            margin-bottom: 0.5rem;
            font-size: 1.1rem;
        }
        
        .info-card p {
            color: #666;
            font-size: 0.9rem;
        }
        
        .endpoints {
            background: #f8f9fa;
            border-radius: 12px;
            padding: 1.5rem;
            margin: 2rem 0;
            text-align: left;
        }
        
        .endpoints h3 {
            color: #333;
            margin-bottom: 1rem;
            text-align: center;
        }
        
        .endpoint {
            display: flex;
            align-items: center;
            margin: 0.5rem 0;
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            font-size: 0.9rem;
        }
        
        .method {
            background: #667eea;
            color: white;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            margin-right: 1rem;
            min-width: 60px;
            text-align: center;
            font-size: 0.8rem;
        }
        
        .method.post { background: #28a745; }
        .method.delete { background: #dc3545; }
        .method.put { background: #ffc107; color: #333; }
        
        .url {
            color: #495057;
            flex: 1;
        }
        
        .status {
            display: inline-flex;
            align-items: center;
            background: #d4edda;
            color: #155724;
            padding: 0.5rem 1rem;
            border-radius: 20px;
            font-size: 0.9rem;
            margin: 1rem 0;
        }
        
        .status::before {
            content: "●";
            margin-right: 0.5rem;
            color: #28a745;
        }
        
        .footer {
            margin-top: 2rem;
            padding-top: 1rem;
            border-top: 1px solid #eee;
            color: #666;
            font-size: 0.9rem;
        }
        
        .version {
            background: #e9ecef;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-family: monospace;
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 2rem;
            }
            
            h1 {
                font-size: 2rem;
            }
            
            .info-grid {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="logo">IIIF</div>
        
        <h1>Bienvenidos al Proyecto IIIF</h1>
        <p class="subtitle">Servidor de conversión PDF a IIIF en <strong>` + h.config.IIIF.BaseURL + `</strong></p>
        
        <div class="status">
            Servidor activo y funcionando
        </div>
        
        <div class="info-grid">
            <div class="info-card">
                <h3>🚀 Estado del Servidor</h3>
                <p>Puerto: <strong>` + h.config.Server.Port + `</strong><br>
                Modo: <strong>` + h.config.Server.Mode + `</strong></p>
            </div>
            
            <div class="info-card">
                <h3>📁 Almacenamiento</h3>
                <p>Datos: <strong>` + h.config.Storage.DataPath + `</strong><br>
                Imágenes: <strong>` + h.config.Storage.ImagesPath + `</strong></p>
            </div>
            
            <div class="info-card">
                <h3>🔧 Configuración IIIF</h3>
                <p>Versión API: <strong>` + h.config.IIIF.APIVersion + `</strong><br>
                Resolución máx: <strong>` + fmt.Sprintf("%dx%d", h.config.IIIF.MaxWidth, h.config.IIIF.MaxHeight) + `</strong></p>
            </div>
            
            <!--
            <div class="info-card">
                <h3>📄 Archivos PDF</h3>
                <p>Tamaño máx: <strong>` + fmt.Sprintf("%d MB", h.config.PDF.MaxFileSize/(1024*1024)) + `</strong><br>
                Formatos: <strong>PDF</strong></p>
            </div>
            -->
        </div>
        
        <div class="endpoints">
            <h3>🌐 Endpoints Disponibles</h3>
            
            <!-- 
            <div>
                <div class="endpoint">
                <span class="method post">POST</span>
                <span class="url">/api/upload - Subir PDF</span>
            </div>
            
            <div class="endpoint">
                <span class="method">GET</span>
                <span class="url">/api/documents - Listar documentos</span>
            </div>
            
            <div class="endpoint">
                <span class="method">GET</span>
                <span class="url">/api/iiif/{id}/manifest - Manifiesto IIIF</span>
            </div>
            -->
            <div class="endpoint">
                <span class="method">GET</span>
                <span class="url">/iiif/3/{identifier}/info.json - Info de imagen</span>
            </div>
            
            <div class="endpoint">
                <span class="method">GET</span>
                <span class="url">/iiif/3/{identifier}/full/max/0/default.jpg - Imagen</span>
            </div>
            
            <div class="endpoint">
                <span class="method">GET</span>
                <span class="url">/api/properties - Propiedades del servidor</span>
            </div>
        </div>
        
        <div class="footer">
            <p>
                <strong>Servidor IIIF PDF</strong> - 
                <span class="version">v1.0.0</span><br>
                Compatible con IIIF Image API 3.0 y Presentation API 3.0
            </p>
            <p style="margin-top: 1rem;">
                📚 <a href="/api/iiif" style="color: #667eea;">Documentación API</a> | 
                ⚙️ <a href="/api/properties" style="color: #667eea;">Configuración</a> |
                📊 <a href="/api/documents" style="color: #667eea;">Estado</a>
            </p>
        </div>
    </div>
</body>
</html>
`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

func (h *WelcomeHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Servidor IIIF PDF funcionando correctamente",
		"version": "1.0.0",
		"port":    h.config.Server.Port,
		"mode":    h.config.Server.Mode,
		"endpoints": map[string]string{
			"upload":    "/api/upload",
			"documents": "/api/documents",
			"manifest":  "/api/iiif/{id}/manifest",
			"image":     "/iiif/3/{identifier}/full/max/0/default.jpg",
			"info":      "/iiif/3/{identifier}/info.json",
		},
	})
}
