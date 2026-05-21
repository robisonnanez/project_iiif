package handlers

type loginRequest struct {
	Username string `json:"username" example:"admin"`
	Password string `json:"password" example:"CAMBIAR_PASSWORD"`
}

type restartServiceRequest struct {
	Password string `json:"password" example:"tu_password_sudo"`
}

type migrationStartSourceLocal struct {
	Path string `json:"path" example:"/var/lib/project_iiif"`
}

type migrationStartSourceSSH struct {
	Host       string `json:"host" example:"172.21.227.83"`
	Port       int    `json:"port" example:"2230"`
	User       string `json:"user" example:"robison"`
	Path       string `json:"path" example:"/var/lib/project_iiif"`
	PrivateKey string `json:"private_key" example:"-----BEGIN OPENSSH PRIVATE KEY-----"`
}

type migrationStartSource struct {
	Type  string                  `json:"type" example:"local"`
	Local migrationStartSourceLocal `json:"local"`
	SSH   migrationStartSourceSSH `json:"ssh"`
}

type migrationStartScope struct {
	ProjectKey string `json:"project_key" example:"metavisor"`
	TenantKey  string `json:"tenant_key" example:"sunat"`
}

type migrationStartPayload struct {
	Source migrationStartSource `json:"source"`
	Scope  migrationStartScope  `json:"scope"`
}

type sessionResponse struct {
	Authenticated bool   `json:"authenticated" example:"true"`
	Username      string `json:"username,omitempty" example:"admin"`
}

type okMessageResponse struct {
	Message string `json:"message" example:"ok"`
}

type restartServiceResponse struct {
	OK      bool   `json:"ok" example:"true"`
	Message string `json:"message" example:"reinicio programado; el servicio se reiniciara en breve"`
	Active  bool   `json:"active" example:"true"`
}

type errorResponse struct {
	Error   string `json:"error" example:"login requerido"`
	Details string `json:"details,omitempty" example:"detalle interno"`
}
