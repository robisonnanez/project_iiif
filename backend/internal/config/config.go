package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Server struct {
		Port string `yaml:"port"`
		Mode string `yaml:"mode"`
	} `yaml:"server"`

	Storage struct {
		DataPath       string `yaml:"data_path"`
		ImagesPath     string `yaml:"images_path"`
		DocumentsPath  string `yaml:"documents_path"`
		ThumbnailsPath string `yaml:"thumbnails_path"`
		ManifestsPath  string `yaml:"manifests_path"`
	} `yaml:"storage"`

	PDF struct {
		MaxFileSize    int64    `yaml:"max_file_size"`
		AllowedFormats []string `yaml:"allowed_formats"`
		TempPath       string   `yaml:"temp_path"`
	} `yaml:"pdf"`

	IIIF struct {
		BaseURL      string `yaml:"base_url"`
		APIVersion   string `yaml:"api_version"`
		MaxWidth     int    `yaml:"max_width"`
		MaxHeight    int    `yaml:"max_height"`
		CacheEnabled bool   `yaml:"cache_enabled"`
		CacheTTL     int    `yaml:"cache_ttl"`
		TileSize     int    `yaml:"tile_size"`
		ScaleFactors []int  `yaml:"scale_factors"`
	} `yaml:"iiif"`

	Conversion struct {
		DefaultFormat   string `yaml:"default_format"`
		DefaultQuality  int    `yaml:"default_quality"`
		EnableOCR       bool   `yaml:"enable_ocr"`
		DPI             int    `yaml:"dpi"`
		BackgroundColor string `yaml:"background_color"`
	} `yaml:"conversion"`

	Security struct {
		EnableAuth           bool     `yaml:"enable_auth"`
		LogLevel             string   `yaml:"log_level"`
		CorsOrigins          []string `yaml:"cors_origins"`
		MaxConcurrentUploads int      `yaml:"max_concurrent_uploads"`
	} `yaml:"security"`

	Performance struct {
		MaxWorkers      int    `yaml:"max_workers"`
		QueueSize       int    `yaml:"queue_size"`
		CleanupInterval int    `yaml:"cleanup_interval"`
		MaxCacheSize    string `yaml:"max_cache_size"`
	} `yaml:"performance"`
}

func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func Default() *Config {
	return &Config{
		Server: struct {
			Port string `yaml:"port"`
			Mode string `yaml:"mode"`
		}{
			Port: "8080",
			Mode: "debug",
		},
		Storage: struct {
			DataPath       string `yaml:"data_path"`
			ImagesPath     string `yaml:"images_path"`
			DocumentsPath  string `yaml:"documents_path"`
			ThumbnailsPath string `yaml:"thumbnails_path"`
			ManifestsPath  string `yaml:"manifests_path"`
		}{
			DataPath:       "./data",
			ImagesPath:     "./data/images",
			DocumentsPath:  "./data/documents",
			ThumbnailsPath: "./data/thumbnails",
			ManifestsPath:  "./data/manifests",
		},
		PDF: struct {
			MaxFileSize    int64    `yaml:"max_file_size"`
			AllowedFormats []string `yaml:"allowed_formats"`
			TempPath       string   `yaml:"temp_path"`
		}{
			MaxFileSize:    100 * 1024 * 1024, // 100MB
			AllowedFormats: []string{"pdf"},
			TempPath:       "./data/temp",
		},
		IIIF: struct {
			BaseURL      string `yaml:"base_url"`
			APIVersion   string `yaml:"api_version"`
			MaxWidth     int    `yaml:"max_width"`
			MaxHeight    int    `yaml:"max_height"`
			CacheEnabled bool   `yaml:"cache_enabled"`
			CacheTTL     int    `yaml:"cache_ttl"`
			TileSize     int    `yaml:"tile_size"`
			ScaleFactors []int  `yaml:"scale_factors"`
		}{
			BaseURL:      "http://localhost:8080",
			APIVersion:   "3",
			MaxWidth:     2048,
			MaxHeight:    2048,
			CacheEnabled: true,
			CacheTTL:     3600,
			TileSize:     512,
			ScaleFactors: []int{1, 2, 4, 8},
		},
		Conversion: struct {
			DefaultFormat   string `yaml:"default_format"`
			DefaultQuality  int    `yaml:"default_quality"`
			EnableOCR       bool   `yaml:"enable_ocr"`
			DPI             int    `yaml:"dpi"`
			BackgroundColor string `yaml:"background_color"`
		}{
			DefaultFormat:   "jpg",
			DefaultQuality:  85,
			EnableOCR:       false,
			DPI:             150,
			BackgroundColor: "white",
		},
		Security: struct {
			EnableAuth           bool     `yaml:"enable_auth"`
			LogLevel             string   `yaml:"log_level"`
			CorsOrigins          []string `yaml:"cors_origins"`
			MaxConcurrentUploads int      `yaml:"max_concurrent_uploads"`
		}{
			EnableAuth:           false,
			LogLevel:             "info",
			CorsOrigins:          []string{"http://localhost:5173", "http://localhost:3000"},
			MaxConcurrentUploads: 5,
		},
		Performance: struct {
			MaxWorkers      int    `yaml:"max_workers"`
			QueueSize       int    `yaml:"queue_size"`
			CleanupInterval int    `yaml:"cleanup_interval"`
			MaxCacheSize    string `yaml:"max_cache_size"`
		}{
			MaxWorkers:      4,
			QueueSize:       100,
			CleanupInterval: 3600,
			MaxCacheSize:    "1GB",
		},
	}
}
