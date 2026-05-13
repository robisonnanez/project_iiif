package models

import "time"

type PDFDocument struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	ProjectKey        string    `json:"projectKey,omitempty"`
	TenantKey         string    `json:"tenantKey,omitempty"`
	MigratedFromLocal bool      `json:"migratedFromLocal"`
	UploadDate        time.Time `json:"uploadDate"`
	Status            string    `json:"status"` // processing, completed, error
	TotalPages        int       `json:"totalPages"`
	ConvertedPages    int       `json:"convertedPages"`
	ManifestURL       string    `json:"manifestUrl,omitempty"`
	ThumbnailURL      string    `json:"thumbnailUrl,omitempty"`
	FilePath          string    `json:"-"`
	ImagePaths        []string  `json:"-"`
}

type DocumentImage struct {
	ID                string    `json:"id"`
	DocumentID        string    `json:"documentId"`
	ProjectKey        string    `json:"projectKey,omitempty"`
	TenantKey         string    `json:"tenantKey,omitempty"`
	MigratedFromLocal bool      `json:"migratedFromLocal"`
	PageNumber        int       `json:"pageNumber"`
	ImagePath         string    `json:"-"`
	Width             int       `json:"width"`
	Height            int       `json:"height"`
	Format            string    `json:"format"`
	MediaType         string    `json:"mediaType,omitempty"`
	ByteSize          int64     `json:"byteSize,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

type Scope struct {
	ProjectKey string `json:"projectKey"`
	TenantKey  string `json:"tenantKey,omitempty"`
}

type BinaryAsset struct {
	ID        string `json:"id"`
	Data      []byte `json:"-"`
	MediaType string `json:"mediaType"`
	ByteSize  int64  `json:"byteSize"`
}

type ConversionSettings struct {
	Format    string `json:"format"`
	Quality   int    `json:"quality"`
	MaxWidth  int    `json:"maxWidth"`
	MaxHeight int    `json:"maxHeight"`
	EnableOCR bool   `json:"enableOCR"`
}

type ServerProperties struct {
	Endpoint       string   `json:"endpoint"`
	MaxFileSize    int      `json:"maxFileSize"`
	AllowedFormats []string `json:"allowedFormats"`
	CacheEnabled   bool     `json:"cacheEnabled"`
	CacheTTL       int      `json:"cacheTTL"`
	EnableAuth     bool     `json:"enableAuth"`
	LogLevel       string   `json:"logLevel"`
}

type IIIFManifest struct {
	Context []string     `json:"@context"`
	ID      string       `json:"id"`
	Type    string       `json:"type"`
	Label   interface{}  `json:"label"`
	Items   []IIIFCanvas `json:"items"`
}

type IIIFCanvas struct {
	ID     string               `json:"id"`
	Type   string               `json:"type"`
	Label  interface{}          `json:"label"`
	Height int                  `json:"height"`
	Width  int                  `json:"width"`
	Items  []IIIFAnnotationPage `json:"items"`
}

type IIIFAnnotationPage struct {
	ID    string           `json:"id"`
	Type  string           `json:"type"`
	Items []IIIFAnnotation `json:"items"`
}

type IIIFAnnotation struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Motivation string   `json:"motivation"`
	Body       IIIFBody `json:"body"`
	Target     string   `json:"target"`
}

type IIIFBody struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`
	Format  string        `json:"format"`
	Service []IIIFService `json:"service"`
}

type IIIFService struct {
	Context string `json:"@context"`
	ID      string `json:"id"`
	Type    string `json:"type"`
	Profile string `json:"profile"`
}

type IIIFImageInfo struct {
	Context  string     `json:"@context"`
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Protocol string     `json:"protocol"`
	Profile  string     `json:"profile"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	Sizes    []IIIFSize `json:"sizes"`
	Tiles    []IIIFTile `json:"tiles"`
}

type IIIFSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type IIIFTile struct {
	Width        int   `json:"width"`
	Height       int   `json:"height"`
	ScaleFactors []int `json:"scaleFactors"`
}
