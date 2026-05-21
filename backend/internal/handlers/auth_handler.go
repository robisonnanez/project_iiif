package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"iiif-pdf-server/internal/config"

	"github.com/gin-gonic/gin"
)

const sessionCookieName = "project_iiif_session"

type AuthHandler struct {
	config *config.Config
}

func NewAuthHandler(config *config.Config) *AuthHandler {
	return &AuthHandler{config: config}
}

// Login godoc
// @Summary Iniciar sesion de dashboard
// @Description Crea cookie de sesion para rutas administrativas.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body loginRequest true "Credenciales"
// @Success 200 {object} sessionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "datos invalidos"})
		return
	}

	if payload.Username != h.config.Frontend.Username || payload.Password != h.config.Frontend.Password {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "usuario o password incorrectos"})
		return
	}

	expiresAt := time.Now().Add(12 * time.Hour).Unix()
	token := h.signSession(payload.Username, expiresAt)
	c.SetCookie(sessionCookieName, token, 12*3600, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"authenticated": true})
}

// Logout godoc
// @Summary Cerrar sesion
// @Description Elimina cookie de sesion activa.
// @Tags Auth
// @Produce json
// @Success 200 {object} sessionResponse
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}

// Me godoc
// @Summary Verificar sesion
// @Description Devuelve estado de autenticacion actual.
// @Tags Auth
// @Produce json
// @Success 200 {object} sessionResponse
// @Router /auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	username, ok := h.sessionUsername(c)
	c.JSON(http.StatusOK, gin.H{"authenticated": ok, "username": username})
}

func (h *AuthHandler) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.config.Frontend.RequireAuth {
			c.Next()
			return
		}
		if _, ok := h.sessionUsername(c); ok {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "login requerido"})
	}
}

func (h *AuthHandler) sessionUsername(c *gin.Context) (string, bool) {
	token, err := c.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}

	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return "", false
	}

	username := parts[0]
	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expiresAt {
		return "", false
	}

	expected := h.signature(username, expiresAt)
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return "", false
	}

	return username, true
}

func (h *AuthHandler) signSession(username string, expiresAt int64) string {
	return fmt.Sprintf("%s:%d:%s", username, expiresAt, h.signature(username, expiresAt))
}

func (h *AuthHandler) signature(username string, expiresAt int64) string {
	key := h.config.Frontend.Password
	if key == "" {
		key = "project-iiif-session"
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(fmt.Sprintf("%s:%d", username, expiresAt)))
	return hex.EncodeToString(mac.Sum(nil))
}
