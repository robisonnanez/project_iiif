package api

import "time"

const TextLayerSchemaVersion = 1

type ErrorDetail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type TextLayerStatus struct {
	SchemaVersion    int       `json:"schema_version"`
	DocumentID       string    `json:"document_id"`
	Available        bool      `json:"available"`
	Status           string    `json:"status"`
	ActiveGeneration string    `json:"active_generation,omitempty"`
	PagesTotal       int       `json:"pages_total"`
	PagesReady       int       `json:"pages_ready"`
	PagesFailed      int       `json:"pages_failed"`
	Geometry         string    `json:"geometry"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Canvas struct {
	ID             string `json:"id"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	ImageID        string `json:"image_id,omitempty"`
	ImageServiceID string `json:"image_service_id,omitempty"`
}

type BoundingBox struct {
	X0 int `json:"x0"`
	Y0 int `json:"y0"`
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
}

type TextLayerWord struct {
	Order          int         `json:"order"`
	BlockIndex     int         `json:"block_index"`
	ParagraphIndex int         `json:"paragraph_index"`
	LineIndex      int         `json:"line_index"`
	WordIndex      int         `json:"word_index"`
	Text           string      `json:"text"`
	Confidence     float64     `json:"confidence"`
	BBox           BoundingBox `json:"bbox"`
}

type TextLayerPage struct {
	SchemaVersion  int             `json:"schema_version"`
	DocumentID     string          `json:"document_id"`
	Generation     string          `json:"generation"`
	PageNumber     int             `json:"page_number"`
	Canvas         Canvas          `json:"canvas"`
	GeometrySpace  string          `json:"geometry_space"`
	GeometryStatus string          `json:"geometry_status"`
	Source         string          `json:"source"`
	Language       string          `json:"language"`
	Confidence     float64         `json:"confidence"`
	Text           string          `json:"text"`
	LayerSHA256    string          `json:"layer_sha256"`
	Words          []TextLayerWord `json:"words"`
}

type OCRGeneration struct {
	ID          string    `json:"id"`
	Active      bool      `json:"active"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PagesTotal  int       `json:"pages_total"`
	PagesReady  int       `json:"pages_ready"`
	PagesFailed int       `json:"pages_failed"`
	Geometry    string    `json:"geometry"`
}

type OCRGenerationList struct {
	SchemaVersion    int             `json:"schema_version"`
	DocumentID       string          `json:"document_id"`
	ActiveGeneration string          `json:"active_generation,omitempty"`
	Generations      []OCRGeneration `json:"generations"`
}
