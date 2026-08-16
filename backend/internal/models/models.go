package models

import "time"

type PDFDocument struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	ProjectKey        string           `json:"projectKey,omitempty"`
	TenantKey         string           `json:"tenantKey,omitempty"`
	MigratedFromLocal bool             `json:"migratedFromLocal"`
	UploadDate        time.Time        `json:"uploadDate"`
	Status            string           `json:"status"` // processing, completed, error
	TotalPages        int              `json:"totalPages"`
	ConvertedPages    int              `json:"convertedPages"`
	ManifestURL       string           `json:"manifestUrl,omitempty"`
	ThumbnailURL      string           `json:"thumbnailUrl,omitempty"`
	ConversionWidth   int              `json:"conversionWidth,omitempty"`
	ConversionHeight  int              `json:"conversionHeight,omitempty"`
	ConversionDPI     int              `json:"conversionDpi,omitempty"`
	ConversionFormat  string           `json:"conversionFormat,omitempty"`
	ConversionQuality int              `json:"conversionQuality,omitempty"`
	Outline           []PDFOutlineItem `json:"outline,omitempty"`
	FilePath          string           `json:"-"`
	ImagePaths        []string         `json:"-"`
}

// PDFOutlineItem is a normalized PDF bookmark. PageNumber is one-based and
// Level starts at one so it can be persisted independently from the PDF
// implementation and reused when the original file is stored remotely.
type PDFOutlineItem struct {
	Level      int    `json:"level" bson:"level"`
	Title      string `json:"title" bson:"title"`
	PageNumber int    `json:"pageNumber" bson:"page_number"`
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
	DPI       int    `json:"dpi"`
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

// IIIF Presentation API 2.1 resources. They intentionally use @id/@type and
// the v2 nesting model instead of reusing the Presentation API 3 structures.
type IIIFManifestV2 struct {
	Context     string           `json:"@context"`
	ID          string           `json:"@id"`
	Type        string           `json:"@type"`
	Label       string           `json:"label"`
	Metadata    []IIIFMetadataV2 `json:"metadata,omitempty"`
	Description string           `json:"description,omitempty"`
	Thumbnail   *IIIFResourceV2  `json:"thumbnail,omitempty"`
	Attribution string           `json:"attribution,omitempty"`
	License     string           `json:"license,omitempty"`
	Sequences   []IIIFSequenceV2 `json:"sequences"`
	Structures  []IIIFRangeV2    `json:"structures,omitempty"`
}

type IIIFMetadataV2 struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type IIIFSequenceV2 struct {
	ID          string         `json:"@id"`
	Type        string         `json:"@type"`
	Label       string         `json:"label,omitempty"`
	ViewingHint string         `json:"viewingHint,omitempty"`
	Canvases    []IIIFCanvasV2 `json:"canvases"`
}

type IIIFCanvasV2 struct {
	ID     string             `json:"@id"`
	Type   string             `json:"@type"`
	Label  string             `json:"label"`
	Height int                `json:"height"`
	Width  int                `json:"width"`
	Images []IIIFAnnotationV2 `json:"images"`
}

type IIIFAnnotationV2 struct {
	ID         string         `json:"@id"`
	Type       string         `json:"@type"`
	Motivation string         `json:"motivation"`
	Resource   IIIFResourceV2 `json:"resource"`
	On         string         `json:"on"`
}

type IIIFResourceV2 struct {
	ID      string         `json:"@id"`
	Type    string         `json:"@type"`
	Format  string         `json:"format,omitempty"`
	Height  int            `json:"height,omitempty"`
	Width   int            `json:"width,omitempty"`
	Service *IIIFServiceV2 `json:"service,omitempty"`
}

type IIIFServiceV2 struct {
	Context  string `json:"@context"`
	ID       string `json:"@id"`
	Profile  string `json:"profile"`
	Protocol string `json:"protocol,omitempty"`
}

type IIIFRangeV2 struct {
	ID       string        `json:"@id"`
	Type     string        `json:"@type"`
	Label    string        `json:"label"`
	Canvases []string      `json:"canvases,omitempty"`
	Ranges   []IIIFRangeV2 `json:"ranges,omitempty"`
	Within   string        `json:"within,omitempty"`
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

type IIIFImageInfoV2 struct {
	Context  string     `json:"@context"`
	ID       string     `json:"@id"`
	Protocol string     `json:"protocol"`
	Profile  string     `json:"profile"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	Sizes    []IIIFSize `json:"sizes,omitempty"`
	Tiles    []IIIFTile `json:"tiles,omitempty"`
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
