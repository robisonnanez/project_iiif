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
	docsButton := `<a class="button secondary" href="/swagger/index.html" target="_blank" rel="noopener noreferrer">Documentación API</a>`
	if h.config.Frontend.Enabled {
		loginButton = `<button class="button" id="login-open" type="button">Iniciar sesión</button>`
	}

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
  <meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Project IIIF</title>
  <style>
    :root{--ink:#142033;--muted:#667085;--brand:#0ea896;--brand-dark:#08766c;--line:#e5eaf0;--surface:#fff;--nav:#101827}*{box-sizing:border-box}body{margin:0;min-width:320px;min-height:100vh;color:var(--ink);background:radial-gradient(circle at 12%% 5%%,#dff8f3 0,transparent 30%%),linear-gradient(135deg,#f7f9fc,#eef2f7);font-family:Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}.shell{width:min(1180px,calc(100%% - 32px));margin:auto;padding:42px 0}.hero{position:relative;overflow:hidden;display:grid;grid-template-columns:minmax(0,1.35fr) minmax(320px,.65fr);border:1px solid rgba(255,255,255,.8);border-radius:28px;background:var(--surface);box-shadow:0 28px 80px rgba(15,23,42,.1)}.hero-copy{padding:clamp(32px,6vw,72px)}.brand{display:inline-flex;align-items:center;gap:12px;margin-bottom:42px;font-weight:850}.logo{width:48px;height:48px;display:grid;place-items:center;border-radius:15px;color:#063e38;background:linear-gradient(145deg,#1fd1bd,#0ea896);font-size:.82rem;box-shadow:0 12px 28px rgba(14,168,150,.24)}.eyebrow{display:block;margin-bottom:12px;color:var(--brand-dark);font-size:.74rem;font-weight:850;letter-spacing:.14em;text-transform:uppercase}h1{max-width:760px;margin:0 0 18px;font-size:clamp(2.35rem,6vw,4.7rem);line-height:.98;letter-spacing:-.065em}.subtitle{max-width:680px;margin:0;color:var(--muted);font-size:clamp(1rem,2vw,1.18rem);line-height:1.7}.hero-actions{display:flex;gap:12px;margin-top:34px;flex-wrap:wrap}.button{min-height:48px;display:inline-flex;align-items:center;justify-content:center;padding:0 20px;border:1px solid transparent;border-radius:13px;color:white;background:var(--brand-dark);text-decoration:none;font:inherit;font-weight:780;cursor:pointer;transition:.18s ease}.button:hover{transform:translateY(-2px);box-shadow:0 12px 26px rgba(8,118,108,.18)}.button.secondary{color:var(--ink);border-color:var(--line);background:#f7f9fb}.hero-side{position:relative;padding:38px;display:flex;flex-direction:column;justify-content:flex-end;color:white;background:linear-gradient(155deg,#101827,#1e3140)}.hero-side:before{content:"";position:absolute;width:260px;height:260px;right:-80px;top:-70px;border-radius:50%%;border:52px solid rgba(31,209,189,.12)}.status-pill{position:relative;width:max-content;margin-bottom:auto;padding:8px 12px;border:1px solid rgba(255,255,255,.13);border-radius:999px;color:#a9eee5;background:rgba(255,255,255,.07);font-size:.76rem;font-weight:800}.side-title{position:relative;margin:0 0 10px;font-size:1.45rem}.side-copy{position:relative;margin:0;color:#b7c2d0;line-height:1.6}.content{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px;margin-top:20px}.panel{padding:23px;border:1px solid rgba(226,232,240,.9);border-radius:20px;background:rgba(255,255,255,.9);box-shadow:0 16px 38px rgba(15,23,42,.055)}.panel h2{margin:0 0 13px;font-size:.82rem;letter-spacing:.08em;text-transform:uppercase}.panel p{margin:0;color:var(--muted);font-size:.92rem;line-height:1.7;overflow-wrap:anywhere}.endpoints{grid-column:1/-1}.endpoint{display:grid;grid-template-columns:52px minmax(0,1fr);gap:13px;align-items:center;padding:10px 0;border-bottom:1px solid var(--line);font:13px/1.5 ui-monospace,SFMono-Regular,Consolas,monospace}.endpoint:last-child{border-bottom:0}.method{padding:5px 7px;border-radius:7px;color:#08766c;background:#ddf8f3;text-align:center;font-weight:850}.footer{margin:24px 4px 0;color:var(--muted);font-size:.83rem}.modal-backdrop{position:fixed;inset:0;z-index:10;display:none;place-items:center;padding:20px;background:rgba(10,18,30,.62);backdrop-filter:blur(8px)}.modal-backdrop.open{display:grid}.login-card{width:min(430px,100%%);padding:38px;border-radius:26px;background:white;box-shadow:0 30px 90px rgba(0,0,0,.28)}.login-brand{display:grid;place-items:center;margin-bottom:28px}.login-mark{width:58px;height:58px;display:grid;place-items:center;border-radius:18px;color:#063e38;background:linear-gradient(145deg,#25d2bf,#0ea896);font-weight:900}.login-card h2{margin:16px 0 6px;font-size:1.65rem;text-align:center}.login-card>p{margin:0;color:var(--muted);text-align:center}.input-wrap{position:relative;margin-top:16px}.input-wrap svg{position:absolute;left:15px;top:50%%;width:18px;transform:translateY(-50%%);color:#7a8797}.input-wrap input{width:100%%;min-height:52px;padding:0 15px 0 46px;border:1px solid #d3d9e2;border-radius:14px;background:#f7f8fa;font:inherit}.input-wrap input:focus{outline:3px solid rgba(14,168,150,.16);border-color:var(--brand)}.login-submit{width:100%%;margin-top:20px}.login-error{min-height:22px;margin-top:12px;color:#b4233c;font-size:.84rem;text-align:center}.modal-actions{display:flex;justify-content:center;margin-top:12px}.link-button{border:0;color:var(--muted);background:transparent;cursor:pointer}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}@media(max-width:820px){.shell{padding:18px 0}.hero{grid-template-columns:1fr;border-radius:22px}.hero-copy{padding:30px 24px}.hero-side{min-height:240px;padding:28px}.brand{margin-bottom:32px}.content{grid-template-columns:1fr}.endpoints{grid-column:auto}}@media(max-width:480px){h1{font-size:2.45rem}.hero-actions,.hero-actions .button{width:100%%}.login-card{padding:28px 20px;border-radius:20px}.endpoint{font-size:11px}}
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero"><div class="hero-copy"><div class="brand"><span class="logo">IIIF</span><span>project_iiif</span></div><span class="eyebrow">Plataforma de publicación documental</span><h1>Convierte, organiza y publica en IIIF.</h1><p class="subtitle">Transforma documentos PDF en imágenes interoperables, genera manifiestos y localiza texto por página desde una sola plataforma.</p><div class="hero-actions">%s%s</div></div><aside class="hero-side"><span class="status-pill">● SERVICIO DISPONIBLE</span><h2 class="side-title">API lista para integrar</h2><p class="side-copy">Endpoint base: <strong>%s</strong><br>Image API 3 · Presentation API 3</p></aside></section>
    <section class="content"><article class="panel"><h2>Servidor</h2><p>Puerto <strong>%s</strong><br>Entorno <strong>%s</strong></p></article><article class="panel"><h2>Almacenamiento</h2><p>Metadatos <strong>%s</strong><br>Datos <strong>%s</strong></p></article><article class="panel"><h2>IIIF</h2><p>Versión pública <strong>%s</strong><br>Resolución máxima <strong>%s</strong></p></article><article class="panel endpoints"><h2>Endpoints principales</h2><div class="endpoint"><span class="method">GET</span><span>/iiif/3/{identifier}/info.json</span></div><div class="endpoint"><span class="method">GET</span><span>/iiif/3/{identifier}/full/max/0/default.jpg</span></div><div class="endpoint"><span class="method">GET</span><span>/api/v1/documents</span></div><div class="endpoint"><span class="method">GET</span><span>/health</span></div></article></section>
    <p class="footer">IIIF Image API 3.0 · Presentation API 3.0 · API de gestión v1</p>
    </main>
    <div class="modal-backdrop" id="login-modal" aria-hidden="true">
        <form class="login-card" id="login-form">
            <div class="login-brand"><span class="login-mark">IIIF</span><h2>Bienvenido</h2><p>Accede al panel de administración</p></div>
            <label class="sr-only" for="login-username">Usuario</label><div class="input-wrap"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/></svg><input id="login-username" name="username" autocomplete="username" placeholder="Usuario" required></div>
            <label class="sr-only" for="login-password">Contraseña</label><div class="input-wrap"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg><input id="login-password" name="password" type="password" autocomplete="current-password" placeholder="Contraseña" required></div>
            <div class="login-error" id="login-error" role="alert"></div><button class="button login-submit" type="submit">Ingresar</button><div class="modal-actions"><button class="link-button" id="login-cancel" type="button">Volver al inicio</button></div>
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
            error.textContent = data.error || "No se pudo iniciar sesión";
        });
    </script>
</body>
</html>`,
		docsButton,
		loginButton,
		html.EscapeString(h.config.IIIF.BaseURL),
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

func (h *WelcomeHandler) ErrorPage(c *gin.Context, status int, title, message string) {
	body := fmt.Sprintf(`<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s · Project IIIF</title><style>*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px;color:#26303d;background:#f4f6f9;font-family:Inter,system-ui,-apple-system,"Segoe UI",sans-serif}.error{width:min(560px,100%%);text-align:center}.art{position:relative;width:250px;height:210px;margin:0 auto 22px}.triangle{position:absolute;left:32px;top:22px;width:0;height:0;border-left:92px solid transparent;border-right:92px solid transparent;border-bottom:165px solid #e75b4f;filter:drop-shadow(0 18px 25px rgba(231,91,79,.2))}.mark{position:absolute;z-index:2;left:118px;top:91px;color:white;font-size:72px;font-weight:300;line-height:1}.circle{position:absolute;right:2px;top:5px;width:72px;height:72px;border:15px solid #dfe8ff;border-radius:50%%}.small{position:absolute;left:8px;top:58px;width:0;height:0;border-left:28px solid transparent;border-right:28px solid transparent;border-top:52px solid #aeb4bc;transform:rotate(12deg)}h1{margin:0 0 10px;font-size:clamp(2.2rem,7vw,4rem);letter-spacing:-.05em}p{margin:0 auto 28px;color:#718096;line-height:1.6}.code{display:block;margin-bottom:8px;color:#e75b4f;font-size:.78rem;font-weight:850;letter-spacing:.15em}.button{min-height:46px;display:inline-flex;align-items:center;padding:0 20px;border-radius:12px;color:white;background:#08766c;text-decoration:none;font-weight:750}</style></head><body><main class="error"><div class="art"><span class="circle"></span><span class="small"></span><span class="triangle"></span><span class="mark">!</span></div><span class="code">ERROR %d</span><h1>%s</h1><p>%s</p><a class="button" href="/">Volver al inicio</a></main></body></html>`, html.EscapeString(title), status, html.EscapeString(title), html.EscapeString(message))
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(status, body)
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
			"upload":           "/api/v1/documents/upload",
			"documents":        "/api/v1/documents",
			"manifest":         "/api/v1/iiif/{id}/manifest",
			"ocr_document":     "/api/v1/documents/{id}/ocr",
			"ocr_page":         "/api/v1/documents/{id}/ocr/pages/{page}",
			"ocr_search":       "/api/v1/ocr/search?q={text}&project={project}&tenant={tenant}",
			"ocr_autocomplete": "/api/v1/ocr/autocomplete?q={prefix}&project={project}&tenant={tenant}",
			"image":            "/iiif/3/{identifier}/full/max/0/default.jpg",
			"info":             "/iiif/3/{identifier}/info.json",
		},
	})
}
