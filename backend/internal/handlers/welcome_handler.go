package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"iiif-pdf-server/internal/config"

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
	loginButton := ""
	docsButton := `<a class="button secondary" href="/swagger/index.html">Documentacion API</a>`
	if h.config.Frontend.Enabled {
		loginButton = `<button class="button" id="login-open" type="button">Iniciar sesion</button>`
	}

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Servidor IIIF PDF</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            min-height: 100vh;
            display: grid;
            place-items: center;
            background: #f4f7fb;
            color: #1f2937;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
            padding: 24px;
        }
        .container {
            width: min(920px, 100%%);
            background: #fff;
            border: 1px solid #dbe3ef;
            border-radius: 8px;
            padding: 32px;
            box-shadow: 0 16px 40px rgba(31, 41, 55, 0.08);
        }
        .top {
            display: flex;
            justify-content: space-between;
            gap: 24px;
            align-items: flex-start;
            border-bottom: 1px solid #e5edf7;
            padding-bottom: 24px;
            margin-bottom: 24px;
        }
        .logo {
            width: 64px;
            height: 64px;
            border-radius: 8px;
            display: grid;
            place-items: center;
            background: #1d4ed8;
            color: white;
            font-weight: 800;
            letter-spacing: 0;
            flex: 0 0 auto;
        }
        h1 { font-size: 32px; line-height: 1.15; margin-bottom: 8px; }
        .subtitle { color: #526173; line-height: 1.55; }
        .button {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            min-height: 42px;
            padding: 0 16px;
            border-radius: 6px;
            color: white;
            background: #1d4ed8;
            text-decoration: none;
            font-weight: 700;
            white-space: nowrap;
            border: 0;
            cursor: pointer;
        }
        .actions {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }
        .modal-backdrop {
            position: fixed;
            inset: 0;
            display: none;
            align-items: center;
            justify-content: center;
            background: rgba(15, 23, 42, 0.45);
            padding: 20px;
        }
        .modal-backdrop.open { display: flex; }
        .modal {
            width: min(420px, 100%%);
            background: white;
            border-radius: 8px;
            border: 1px solid #dbe3ef;
            padding: 22px;
            box-shadow: 0 24px 70px rgba(15, 23, 42, 0.25);
        }
        .modal h2 { margin: 0 0 12px; font-size: 22px; }
        .modal label { display: grid; gap: 6px; margin: 12px 0; color: #526173; font-size: 14px; }
        .modal input {
            min-height: 40px;
            border: 1px solid #cfd9e6;
            border-radius: 6px;
            padding: 0 10px;
            font: inherit;
        }
        .modal-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 16px; }
        .button.secondary { color: #1f2937; background: #eef3f8; }
        .login-error { color: #b91c1c; min-height: 20px; font-size: 14px; }
        .grid {
            display: grid;
            grid-template-columns: repeat(3, minmax(0, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }
        .panel {
            border: 1px solid #e5edf7;
            border-radius: 8px;
            padding: 16px;
            background: #fbfdff;
        }
        .panel h2 {
            font-size: 15px;
            margin-bottom: 8px;
            color: #111827;
        }
        .panel p { color: #526173; font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
        .endpoints {
            border: 1px solid #e5edf7;
            border-radius: 8px;
            padding: 16px;
        }
        .endpoint {
            display: flex;
            gap: 12px;
            align-items: center;
            padding: 8px 0;
            font-family: Consolas, "Liberation Mono", monospace;
            font-size: 14px;
            color: #374151;
        }
        .method {
            min-width: 48px;
            text-align: center;
            border-radius: 4px;
            padding: 3px 6px;
            background: #e8f0ff;
            color: #1d4ed8;
            font-weight: 700;
        }
        .footer {
            margin-top: 24px;
            color: #6b7280;
            font-size: 14px;
        }
        @media (max-width: 760px) {
            .top { flex-direction: column; }
            .grid { grid-template-columns: 1fr; }
            h1 { font-size: 26px; }
        }
    </style>
</head>
<body>
    <main class="container">
        <section class="top">
            <div style="display:flex; gap:18px;">
                <div class="logo">IIIF</div>
                <div>
                    <h1>Bienvenidos al Proyecto IIIF</h1>
                    <p class="subtitle">Servidor de conversion PDF a IIIF en <strong>%s</strong>.</p>
                </div>
            </div>
            <div class="actions">%s%s</div>
        </section>
        <section class="grid">
            <article class="panel">
                <h2>Servidor</h2>
                <p>Puerto: <strong>%s</strong><br>Modo: <strong>%s</strong></p>
            </article>
            <article class="panel">
                <h2>Almacenamiento</h2>
                <p>Backend: <strong>%s</strong><br>Datos: <strong>%s</strong></p>
            </article>
            <article class="panel">
                <h2>IIIF</h2>
                <p>API: <strong>%s</strong><br>Resolucion max: <strong>%s</strong></p>
            </article>
        </section>
        <section class="endpoints">
            <div class="endpoint"><span class="method">GET</span><span>/iiif/3/{identifier}/info.json</span></div>
            <div class="endpoint"><span class="method">GET</span><span>/iiif/3/{identifier}/full/max/0/default.jpg</span></div>
            <div class="endpoint"><span class="method">GET</span><span>/api/properties</span></div>
            <div class="endpoint"><span class="method">GET</span><span>/health</span></div>
        </section>
        <p class="footer">Compatible con IIIF Image API 3.0 y Presentation API 3.0.</p>
    </main>
    <div class="modal-backdrop" id="login-modal" aria-hidden="true">
        <form class="modal" id="login-form">
            <h2>Iniciar sesion</h2>
            <p>Ingresa con el usuario y password configurados en config.yaml.</p>
            <label>Usuario
                <input id="login-username" name="username" autocomplete="username" required>
            </label>
            <label>Password
                <input id="login-password" name="password" type="password" autocomplete="current-password" required>
            </label>
            <div class="login-error" id="login-error"></div>
            <div class="modal-actions">
                <button class="button secondary" id="login-cancel" type="button">Cancelar</button>
                <button class="button" type="submit">Ingresar</button>
            </div>
        </form>
    </div>
    <script>
        const openButton = document.getElementById("login-open");
        const modal = document.getElementById("login-modal");
        const form = document.getElementById("login-form");
        const cancel = document.getElementById("login-cancel");
        const error = document.getElementById("login-error");

        function closeModal() {
            modal?.classList.remove("open");
            if (error) error.textContent = "";
        }

        openButton?.addEventListener("click", () => {
            modal.classList.add("open");
            document.getElementById("login-username")?.focus();
        });
        cancel?.addEventListener("click", closeModal);
        modal?.addEventListener("click", (event) => {
            if (event.target === modal) closeModal();
        });
        form?.addEventListener("submit", async (event) => {
            event.preventDefault();
            error.textContent = "";
            const response = await fetch("/auth/login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    username: document.getElementById("login-username").value,
                    password: document.getElementById("login-password").value
                })
            });
            if (response.ok) {
                window.location.href = "/dashboard";
                return;
            }
            const data = await response.json().catch(() => ({}));
            error.textContent = data.error || "No se pudo iniciar sesion";
        });
    </script>
</body>
</html>`,
		html.EscapeString(h.config.IIIF.BaseURL),
		docsButton,
		loginButton,
		html.EscapeString(h.config.Server.Port),
		html.EscapeString(h.config.Server.Mode),
		html.EscapeString(h.config.Storage.Backend),
		html.EscapeString(h.config.Storage.DataPath),
		html.EscapeString(h.config.IIIF.APIVersion),
		html.EscapeString(fmt.Sprintf("%dx%d", h.config.IIIF.MaxWidth, h.config.IIIF.MaxHeight)),
	)

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, htmlBody)
}

func (h *WelcomeHandler) HealthCheck(c *gin.Context) {
	s3Enabled := strings.EqualFold(h.config.FilesystemDisk, "s3") || strings.EqualFold(h.config.BinaryStorage.Mode, "s3")
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Servidor IIIF PDF funcionando correctamente",
		"version": "1.0.0",
		"port":    h.config.Server.Port,
		"mode":    h.config.Server.Mode,
		"frontend": gin.H{
			"enabled":      h.config.Frontend.Enabled,
			"require_auth": h.config.Frontend.RequireAuth,
		},
		"binary_storage": gin.H{
			"mode":        h.config.BinaryStorage.Mode,
			"s3_enabled":  s3Enabled,
			"s3_endpoint": h.config.AWSEndpoint,
			"s3_bucket":   h.config.AWSBucket,
		},
		"endpoints": map[string]string{
			"upload":       "/api/v1/documents/upload",
			"documents":    "/api/v1/documents",
			"manifest_v2":  "/api/v1/iiif/{id}/manifest",
			"manifest_v3":  "/api/v1/iiif/{id}/manifest/v3",
			"ocr_document": "/api/v1/documents/{id}/ocr",
			"ocr_page":     "/api/v1/documents/{id}/ocr/pages/{page}",
			"ocr_search":   "/api/v1/ocr/search?q={text}&project={project}&tenant={tenant}",
			"image":        "/iiif/3/{identifier}/full/max/0/default.jpg",
			"info":         "/iiif/3/{identifier}/info.json",
		},
	})
}
