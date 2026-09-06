// Package buildinfo expone metadatos inmutables de la revisión compilada.
package buildinfo

// Estos valores se sustituyen mediante -ldflags durante la compilación de una versión.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info describe la versión exacta del backend y los artefactos asociados.
type Info struct {
	Version   string `json:"version" example:"1.1.0"`
	Commit    string `json:"commit" example:"9e0befef0bce9e200349ac2a5b86c231e5d02d29"`
	BuildDate string `json:"build_date" example:"2026-09-06T15:56:00Z"`
}

// Current devuelve una copia de los metadatos incorporados al binario.
func Current() Info {
	return Info{Version: Version, Commit: Commit, BuildDate: BuildDate}
}
