package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/storage"

	"github.com/gin-gonic/gin"
)

const textLayerReadScope = "text-layer:read"

type integrationClaims struct {
	Issuer     string `json:"iss"`
	Subject    string `json:"sub"`
	IssuedAt   int64  `json:"iat"`
	ExpiresAt  int64  `json:"exp"`
	Scope      string `json:"scope"`
	Project    string `json:"project"`
	Tenant     string `json:"tenant,omitempty"`
	DocumentID string `json:"document_id,omitempty"`
}

type rateWindow struct {
	Started time.Time
	Count   int
}

type IntegrationAuth struct {
	config   config.IntegrationAuthConfig
	store    storage.Storage
	sessions *AuthHandler
	mu       sync.Mutex
	windows  map[string]rateWindow
}

func NewIntegrationAuth(cfg *config.Config, store storage.Storage, sessions *AuthHandler) (*IntegrationAuth, error) {
	settings := cfg.Security.IntegrationAuth
	if settings.Enabled && len([]byte(settings.HMACSecret)) < 32 {
		return nil, errors.New("security.integration_auth.hmac_secret debe tener al menos 32 caracteres")
	}
	return &IntegrationAuth{config: settings, store: store, sessions: sessions, windows: map[string]rateWindow{}}, nil
}

func (a *IntegrationAuth) IssueToken(c *gin.Context) {
	if !a.config.Enabled {
		writeContractError(c, http.StatusServiceUnavailable, "integration_auth_disabled", "la autenticación de integración está desactivada", nil)
		return
	}
	var request struct {
		ConsumerID string `json:"consumer_id"`
		Project    string `json:"project"`
		Tenant     string `json:"tenant"`
		DocumentID string `json:"document_id"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "payload de token inválido", nil)
		return
	}
	request.ConsumerID, request.Project, request.Tenant, request.DocumentID = strings.TrimSpace(request.ConsumerID), strings.TrimSpace(request.Project), strings.TrimSpace(request.Tenant), strings.TrimSpace(request.DocumentID)
	if request.ConsumerID == "" || request.Project == "" {
		writeContractError(c, http.StatusBadRequest, "invalid_request", "consumer_id y project son obligatorios", nil)
		return
	}
	ttl := request.TTLSeconds
	if ttl == 0 {
		ttl = a.config.DefaultTTLSeconds
	}
	if ttl < 1 || ttl > a.config.MaxTTLSeconds {
		writeContractError(c, http.StatusBadRequest, "invalid_request", fmt.Sprintf("ttl_seconds debe estar entre 1 y %d", a.config.MaxTTLSeconds), nil)
		return
	}
	if request.DocumentID != "" {
		document, err := a.store.GetDocument(request.DocumentID)
		if err != nil || document.ProjectKey != request.Project || (request.Tenant != "" && document.TenantKey != request.Tenant) {
			writeContractError(c, http.StatusForbidden, "forbidden", "el documento no pertenece al ámbito solicitado", nil)
			return
		}
		request.Tenant = document.TenantKey
	}
	now := time.Now().UTC()
	claims := integrationClaims{Issuer: a.config.Issuer, Subject: request.ConsumerID, IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Duration(ttl) * time.Second).Unix(), Scope: textLayerReadScope, Project: request.Project, Tenant: request.Tenant, DocumentID: request.DocumentID}
	token, err := a.sign(claims)
	if err != nil {
		writeContractError(c, http.StatusInternalServerError, "internal_error", "no se pudo emitir el token", nil)
		return
	}
	writeJSON(c, http.StatusCreated, gin.H{"access_token": token, "token_type": "Bearer", "expires_at": time.Unix(claims.ExpiresAt, 0).UTC(), "scope": claims.Scope})
}

func (a *IntegrationAuth) RequireRead() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		if !a.config.Enabled {
			c.Next()
			return
		}
		if username, ok := a.sessions.sessionUsername(c); ok {
			c.Set("integration_consumer", "session:"+username)
			c.Next()
			return
		}
		value := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
			writeContractError(c, http.StatusUnauthorized, "unauthorized", "Bearer token requerido", nil)
			c.Abort()
			return
		}
		claims, err := a.verify(strings.TrimSpace(value[7:]))
		if err != nil {
			writeContractError(c, http.StatusUnauthorized, "unauthorized", "Bearer token inválido o expirado", nil)
			c.Abort()
			return
		}
		if !strings.Contains(" "+claims.Scope+" ", " "+textLayerReadScope+" ") {
			writeContractError(c, http.StatusForbidden, "forbidden", "scope insuficiente", nil)
			c.Abort()
			return
		}
		documentID := strings.TrimSpace(c.Param("id"))
		if documentID != "" {
			document, documentErr := a.store.GetDocument(documentID)
			if documentErr != nil || (claims.DocumentID != "" && claims.DocumentID != documentID) || claims.Project != document.ProjectKey || (claims.Tenant != "" && claims.Tenant != document.TenantKey) {
				writeContractError(c, http.StatusForbidden, "forbidden", "recurso fuera del ámbito del token", nil)
				c.Abort()
				return
			}
		}
		query := c.Request.URL.Query()
		if documentID == "" {
			if query.Get("project") != "" && query.Get("project") != claims.Project {
				writeContractError(c, http.StatusForbidden, "forbidden", "proyecto fuera del ámbito del token", nil)
				c.Abort()
				return
			}
			if query.Get("tenant") != "" && query.Get("tenant") != claims.Tenant {
				writeContractError(c, http.StatusForbidden, "forbidden", "tenant fuera del ámbito del token", nil)
				c.Abort()
				return
			}
			query.Set("project", claims.Project)
			if claims.Tenant != "" {
				query.Set("tenant", claims.Tenant)
			}
			c.Request.URL.RawQuery = query.Encode()
		}
		key := claims.Subject + "\x00" + documentID
		if !a.allow(key) {
			c.Header("Retry-After", "60")
			writeContractError(c, http.StatusTooManyRequests, "rate_limited", "límite de solicitudes excedido", nil)
			c.Abort()
			return
		}
		c.Set("integration_consumer", claims.Subject)
		c.Set("integration_claims", claims)
		c.Next()
		log.Printf("[AUDIT] consumer=%s route=%s document=%s page=%s status=%d latency=%s", claims.Subject, c.FullPath(), documentID, c.Param("page"), c.Writer.Status(), time.Since(startedAt).Round(time.Millisecond))
	}
}

func (a *IntegrationAuth) allow(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	window := a.windows[key]
	if window.Started.IsZero() || now.Sub(window.Started) >= time.Minute {
		window = rateWindow{Started: now}
	}
	if window.Count >= a.config.RatePerMinute+a.config.Burst {
		a.windows[key] = window
		return false
	}
	window.Count++
	a.windows[key] = window
	return true
}

func (a *IntegrationAuth) sign(claims integrationClaims) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(a.config.HMACSecret))
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (a *IntegrationAuth) verify(token string) (*integrationClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("formato JWT inválido")
	}
	mac := hmac.New(sha256.New, []byte(a.config.HMACSecret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, errors.New("firma JWT inválida")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims integrationClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if claims.Issuer != a.config.Issuer || claims.Subject == "" || claims.IssuedAt > now+30 || claims.ExpiresAt <= now {
		return nil, errors.New("claims JWT inválidos")
	}
	return &claims, nil
}
