package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iiif-pdf-server/internal/models"

	"gopkg.in/yaml.v2"
)

type Config struct {
	SourcePath string `yaml:"-"`

	StorageBackend string `yaml:"STORAGE_BACKEND"`
	DBConnection   string `yaml:"DB_CONNECTION"`
	DBHost         string `yaml:"DB_HOST"`
	DBPort         string `yaml:"DB_PORT"`
	DBDatabase     string `yaml:"DB_DATABASE"`
	DBUsername     string `yaml:"DB_USERNAME"`
	DBPassword     string `yaml:"DB_PASSWORD"`

	FilesystemDisk          string `yaml:"FILESYSTEM_DISK"`
	AWSAccessKeyID          string `yaml:"AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey      string `yaml:"AWS_SECRET_ACCESS_KEY"`
	AWSDefaultRegion        string `yaml:"AWS_DEFAULT_REGION"`
	AWSBucket               string `yaml:"AWS_BUCKET"`
	AWSEndpoint             string `yaml:"AWS_ENDPOINT"`
	AWSUsePathStyleEndpoint bool   `yaml:"AWS_USE_PATH_STYLE_ENDPOINT"`

	Server struct {
		Port string `yaml:"port"`
		Mode string `yaml:"mode"`
	} `yaml:"server"`

	Storage struct {
		Backend        string `yaml:"backend"`
		DataPath       string `yaml:"data_path"`
		PDFsPath       string `yaml:"pdfs_path"`
		ImagesPath     string `yaml:"images_path"`
		DocumentsPath  string `yaml:"documents_path"`
		ThumbnailsPath string `yaml:"thumbnails_path"`
		ManifestsPath  string `yaml:"manifests_path"`
	} `yaml:"storage"`

	Database struct {
		AutoMigrate bool `yaml:"auto_migrate"`
		MySQL       struct {
			Host      string `yaml:"host"`
			Port      string `yaml:"port"`
			User      string `yaml:"user"`
			Password  string `yaml:"password"`
			Database  string `yaml:"database"`
			Charset   string `yaml:"charset"`
			ParseTime bool   `yaml:"parse_time"`
		} `yaml:"mysql"`
		Postgres struct {
			Host     string `yaml:"host"`
			Port     string `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Database string `yaml:"database"`
			SSLMode  string `yaml:"sslmode"`
			Schema   string `yaml:"schema"`
		} `yaml:"postgres"`
		MongoDB struct {
			Host                     string `yaml:"host"`
			Port                     string `yaml:"port"`
			User                     string `yaml:"user"`
			Password                 string `yaml:"password"`
			Database                 string `yaml:"database"`
			AuthSource               string `yaml:"auth_source"`
			DirectConnection         bool   `yaml:"direct_connection"`
			ServerSelectionTimeoutMS int    `yaml:"server_selection_timeout_ms"`
		} `yaml:"mongodb"`
	} `yaml:"database"`

	PDF struct {
		MaxFileSize    int64    `yaml:"max_file_size"`
		AllowedFormats []string `yaml:"allowed_formats"`
		TempPath       string   `yaml:"temp_path"`
	} `yaml:"pdf"`

	BinaryStorage struct {
		Mode     string `yaml:"mode"`
		TempPath string `yaml:"temp_path"`
	} `yaml:"binary_storage"`

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
		DefaultWidth    int    `yaml:"default_width"`
		DefaultHeight   int    `yaml:"default_height"`
		DefaultFormat   string `yaml:"default_format"`
		DefaultQuality  int    `yaml:"default_quality"`
		EnableOCR       bool   `yaml:"enable_ocr"`
		DPI             int    `yaml:"dpi"`
		BackgroundColor string `yaml:"background_color"`
	} `yaml:"conversion"`

	OCR OCRConfig `yaml:"ocr"`

	Security struct {
		EnableAuth           bool     `yaml:"enable_auth"`
		LogLevel             string   `yaml:"log_level"`
		CorsOrigins          []string `yaml:"cors_origins"`
		MaxConcurrentUploads int      `yaml:"max_concurrent_uploads"`
	} `yaml:"security"`

	Frontend struct {
		Enabled         bool   `yaml:"enabled"`
		Path            string `yaml:"path"`
		RequireAuth     bool   `yaml:"require_auth"`
		Username        string `yaml:"username"`
		Password        string `yaml:"password"`
		MenuOrientation string `yaml:"menu_orientation"`
	} `yaml:"frontend"`

	Projects struct {
		Enabled             bool            `yaml:"enabled"`
		DefaultProject      string          `yaml:"default_project"`
		RequireProject      bool            `yaml:"require_project"`
		AllowDynamicTenants bool            `yaml:"allow_dynamic_tenants"`
		Items               []ProjectConfig `yaml:"items"`
	} `yaml:"projects"`

	Performance struct {
		MaxWorkers      int    `yaml:"max_workers"`
		QueueSize       int    `yaml:"queue_size"`
		CleanupInterval int    `yaml:"cleanup_interval"`
		MaxCacheSize    string `yaml:"max_cache_size"`
	} `yaml:"performance"`

	Migration struct {
		Enabled           bool     `yaml:"enabled"`
		AllowedLocalRoots []string `yaml:"allowed_local_roots"`
		MaxLogLines       int      `yaml:"max_log_lines"`
		SSH               struct {
			ConnectTimeoutSec int      `yaml:"connect_timeout_sec"`
			AllowedHosts      []string `yaml:"allowed_hosts"`
		} `yaml:"ssh"`
	} `yaml:"migration"`
}

type OCRConfig struct {
	Enabled             bool     `yaml:"enabled" json:"enabled"`
	AutoAfterConversion bool     `yaml:"auto_after_conversion" json:"auto_after_conversion"`
	DefaultMode         string   `yaml:"default_mode" json:"default_mode"`
	Workers             int      `yaml:"workers" json:"workers"`
	PageTimeoutSeconds  int      `yaml:"page_timeout_seconds" json:"page_timeout_seconds"`
	RetriesPerPage      int      `yaml:"retries_per_page" json:"retries_per_page"`
	RenderDPI           int      `yaml:"render_dpi" json:"render_dpi"`
	MinTextChars        int      `yaml:"min_text_chars" json:"min_text_chars"`
	CandidateLanguages  []string `yaml:"candidate_languages" json:"candidate_languages"`
	FallbackLanguages   []string `yaml:"fallback_languages" json:"fallback_languages"`
	LanguageDetection   struct {
		Enabled           bool    `yaml:"enabled" json:"enabled"`
		SamplePages       int     `yaml:"sample_pages" json:"sample_pages"`
		MinSampleChars    int     `yaml:"min_sample_chars" json:"min_sample_chars"`
		MinimumConfidence float64 `yaml:"minimum_confidence" json:"minimum_confidence"`
		MaxLanguages      int     `yaml:"max_languages" json:"max_languages"`
	} `yaml:"language_detection" json:"language_detection"`
	Artifacts struct {
		Gzip bool `yaml:"gzip" json:"gzip"`
	} `yaml:"artifacts" json:"artifacts"`
}

type ProjectConfig struct {
	Key                    string   `yaml:"key" json:"key"`
	Name                   string   `yaml:"name" json:"name"`
	BulkUpload             bool     `yaml:"bulk_upload" json:"bulk_upload"`
	Multitenant            bool     `yaml:"multitenant" json:"multitenant"`
	Tenants                []string `yaml:"tenants" json:"tenants"`
	TenantsEndpoint        string   `yaml:"tenants_endpoint,omitempty" json:"tenants_endpoint,omitempty"`
	TenantsAuthType        string   `yaml:"tenants_auth_type,omitempty" json:"tenants_auth_type,omitempty"`
	TenantsAuthHeader      string   `yaml:"tenants_auth_header,omitempty" json:"tenants_auth_header,omitempty"`
	TenantsAuthToken       string   `yaml:"tenants_auth_token,omitempty" json:"tenants_auth_token,omitempty"`
	TenantsTokenConfigured bool     `yaml:"-" json:"tenants_token_configured,omitempty"`
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

	applyEnvironment(&config)
	applyDefaults(&config)
	config.SourcePath = resolvedPath(filename)
	return &config, nil
}

func applyEnvironment(config *Config) {
	stringValues := []struct {
		name   string
		target *string
	}{
		{"STORAGE_BACKEND", &config.StorageBackend},
		{"DB_CONNECTION", &config.DBConnection},
		{"DB_HOST", &config.DBHost},
		{"DB_PORT", &config.DBPort},
		{"DB_DATABASE", &config.DBDatabase},
		{"DB_USERNAME", &config.DBUsername},
		{"DB_PASSWORD", &config.DBPassword},
		{"FILESYSTEM_DISK", &config.FilesystemDisk},
		{"AWS_ACCESS_KEY_ID", &config.AWSAccessKeyID},
		{"AWS_SECRET_ACCESS_KEY", &config.AWSSecretAccessKey},
		{"AWS_DEFAULT_REGION", &config.AWSDefaultRegion},
		{"AWS_BUCKET", &config.AWSBucket},
		{"AWS_ENDPOINT", &config.AWSEndpoint},
		{"FRONTEND_USERNAME", &config.Frontend.Username},
		{"FRONTEND_PASSWORD", &config.Frontend.Password},
		{"IIIF_BASE_URL", &config.IIIF.BaseURL},
	}
	for _, item := range stringValues {
		if value, ok := os.LookupEnv(item.name); ok {
			*item.target = strings.TrimSpace(value)
		}
	}
	if value, ok := os.LookupEnv("AWS_USE_PATH_STYLE_ENDPOINT"); ok {
		config.AWSUsePathStyleEndpoint = strings.EqualFold(strings.TrimSpace(value), "true") || strings.TrimSpace(value) == "1"
	}
}

func Save(filename string, config *Config) error {
	applyDefaults(config)
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(filename), ".config-*.yaml")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempName)
		return err
	}
	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		os.Remove(tempName)
		return err
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(tempName)
		return err
	}
	return os.Rename(tempName, filename)
}

func resolvedPath(filename string) string {
	absolute, err := filepath.Abs(filename)
	if err != nil {
		absolute = filename
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return absolute
	}
	return resolved
}

func (config *Config) ApplyDefaults() {
	applyDefaults(config)
}

func applyDefaults(config *Config) {
	defaults := Default()
	if config.OCR.DefaultMode == "" {
		config.OCR.DefaultMode = defaults.OCR.DefaultMode
	}
	if config.OCR.Workers <= 0 {
		config.OCR.Workers = defaults.OCR.Workers
	}
	if config.OCR.PageTimeoutSeconds <= 0 {
		config.OCR.PageTimeoutSeconds = defaults.OCR.PageTimeoutSeconds
	}
	if config.OCR.RetriesPerPage <= 0 {
		config.OCR.RetriesPerPage = defaults.OCR.RetriesPerPage
	}
	if config.OCR.RenderDPI <= 0 {
		config.OCR.RenderDPI = defaults.OCR.RenderDPI
	}
	if config.OCR.MinTextChars <= 0 {
		config.OCR.MinTextChars = defaults.OCR.MinTextChars
	}
	if len(config.OCR.CandidateLanguages) == 0 {
		config.OCR.CandidateLanguages = defaults.OCR.CandidateLanguages
	}
	if len(config.OCR.FallbackLanguages) == 0 {
		config.OCR.FallbackLanguages = defaults.OCR.FallbackLanguages
	}
	if config.OCR.LanguageDetection.SamplePages <= 0 {
		config.OCR.LanguageDetection.SamplePages = defaults.OCR.LanguageDetection.SamplePages
	}
	if config.OCR.LanguageDetection.MinSampleChars <= 0 {
		config.OCR.LanguageDetection.MinSampleChars = defaults.OCR.LanguageDetection.MinSampleChars
	}
	if config.OCR.LanguageDetection.MinimumConfidence <= 0 {
		config.OCR.LanguageDetection.MinimumConfidence = defaults.OCR.LanguageDetection.MinimumConfidence
	}
	if config.OCR.LanguageDetection.MaxLanguages <= 0 {
		config.OCR.LanguageDetection.MaxLanguages = defaults.OCR.LanguageDetection.MaxLanguages
	}
	if config.Conversion.EnableOCR {
		config.OCR.Enabled = true
	}
	if config.FilesystemDisk == "" {
		config.FilesystemDisk = "local"
	}
	config.FilesystemDisk = strings.ToLower(strings.TrimSpace(config.FilesystemDisk))
	if config.AWSDefaultRegion == "" {
		config.AWSDefaultRegion = "us-east-1"
	}
	if config.StorageBackend != "" {
		config.Storage.Backend = config.StorageBackend
	}
	if config.DBConnection != "" {
		config.Storage.Backend = config.DBConnection
	}
	engine := normalizedEngine(config.Storage.Backend)
	if engine == "local" {
		engine = normalizedEngine(config.DBConnection)
	}
	if config.DBHost != "" {
		switch engine {
		case "postgres":
			config.Database.Postgres.Host = config.DBHost
		case "mongodb":
			config.Database.MongoDB.Host = config.DBHost
		default:
			config.Database.MySQL.Host = config.DBHost
		}
	}
	if config.DBPort != "" {
		switch engine {
		case "postgres":
			config.Database.Postgres.Port = config.DBPort
		case "mongodb":
			config.Database.MongoDB.Port = config.DBPort
		default:
			config.Database.MySQL.Port = config.DBPort
		}
	}
	if config.DBUsername != "" {
		switch engine {
		case "postgres":
			config.Database.Postgres.User = config.DBUsername
		case "mongodb":
			config.Database.MongoDB.User = config.DBUsername
		default:
			config.Database.MySQL.User = config.DBUsername
		}
	}
	if config.DBPassword != "" {
		switch engine {
		case "postgres":
			config.Database.Postgres.Password = config.DBPassword
		case "mongodb":
			config.Database.MongoDB.Password = config.DBPassword
		default:
			config.Database.MySQL.Password = config.DBPassword
		}
	}
	if config.DBDatabase != "" {
		switch engine {
		case "postgres":
			config.Database.Postgres.Database = config.DBDatabase
		case "mongodb":
			config.Database.MongoDB.Database = config.DBDatabase
		default:
			config.Database.MySQL.Database = config.DBDatabase
		}
	}
	if config.Storage.Backend == "" {
		config.Storage.Backend = defaults.Storage.Backend
	}
	if config.Storage.DataPath == "" {
		config.Storage.DataPath = defaults.Storage.DataPath
	}
	if config.Storage.PDFsPath == "" {
		config.Storage.PDFsPath = defaults.Storage.PDFsPath
	}
	if config.Storage.ImagesPath == "" {
		config.Storage.ImagesPath = defaults.Storage.ImagesPath
	}
	if config.Storage.DocumentsPath == "" {
		config.Storage.DocumentsPath = defaults.Storage.DocumentsPath
	}
	if config.Storage.ThumbnailsPath == "" {
		config.Storage.ThumbnailsPath = defaults.Storage.ThumbnailsPath
	}
	if config.Storage.ManifestsPath == "" {
		config.Storage.ManifestsPath = defaults.Storage.ManifestsPath
	}
	if config.Database.MySQL.Host == "" {
		config.Database.MySQL.Host = defaults.Database.MySQL.Host
	}
	if config.Database.MySQL.Port == "" {
		config.Database.MySQL.Port = defaults.Database.MySQL.Port
	}
	if config.Database.MySQL.User == "" {
		config.Database.MySQL.User = defaults.Database.MySQL.User
	}
	if config.Database.MySQL.Database == "" {
		config.Database.MySQL.Database = defaults.Database.MySQL.Database
	}
	if config.Database.MySQL.Charset == "" {
		config.Database.MySQL.Charset = defaults.Database.MySQL.Charset
	}
	if config.Database.Postgres.Host == "" {
		config.Database.Postgres.Host = defaults.Database.Postgres.Host
	}
	if config.Database.Postgres.Port == "" {
		config.Database.Postgres.Port = defaults.Database.Postgres.Port
	}
	if config.Database.Postgres.User == "" {
		config.Database.Postgres.User = defaults.Database.Postgres.User
	}
	if config.Database.Postgres.Database == "" {
		config.Database.Postgres.Database = defaults.Database.Postgres.Database
	}
	if config.Database.Postgres.SSLMode == "" {
		config.Database.Postgres.SSLMode = defaults.Database.Postgres.SSLMode
	}
	if config.Database.MongoDB.Host == "" {
		config.Database.MongoDB.Host = defaults.Database.MongoDB.Host
	}
	if config.Database.MongoDB.Port == "" {
		config.Database.MongoDB.Port = defaults.Database.MongoDB.Port
	}
	if config.Database.MongoDB.User == "" {
		config.Database.MongoDB.User = defaults.Database.MongoDB.User
	}
	if config.Database.MongoDB.Database == "" {
		config.Database.MongoDB.Database = defaults.Database.MongoDB.Database
	}
	if config.Database.MongoDB.AuthSource == "" {
		config.Database.MongoDB.AuthSource = defaults.Database.MongoDB.AuthSource
	}
	if config.Database.MongoDB.ServerSelectionTimeoutMS == 0 {
		config.Database.MongoDB.ServerSelectionTimeoutMS = defaults.Database.MongoDB.ServerSelectionTimeoutMS
	}
	if !config.Database.MongoDB.DirectConnection {
		config.Database.MongoDB.DirectConnection = defaults.Database.MongoDB.DirectConnection
	}
	if config.BinaryStorage.Mode == "" {
		if config.FilesystemDisk == "s3" {
			config.BinaryStorage.Mode = "s3"
		} else if config.Storage.Backend == "local" {
			config.BinaryStorage.Mode = "local"
		} else {
			config.BinaryStorage.Mode = defaults.BinaryStorage.Mode
		}
	}
	if config.BinaryStorage.TempPath == "" {
		config.BinaryStorage.TempPath = defaults.BinaryStorage.TempPath
	}
	if config.Frontend.Path == "" {
		config.Frontend.Path = defaults.Frontend.Path
	}
	if config.Frontend.Username == "" {
		config.Frontend.Username = defaults.Frontend.Username
	}
	if config.Frontend.Password == "" {
		config.Frontend.Password = defaults.Frontend.Password
	}
	if config.Frontend.MenuOrientation != "vertical" {
		config.Frontend.MenuOrientation = "horizontal"
	}
	if config.Security.MaxConcurrentUploads <= 0 {
		config.Security.MaxConcurrentUploads = defaults.Security.MaxConcurrentUploads
	} else if config.Security.MaxConcurrentUploads > 100 {
		config.Security.MaxConcurrentUploads = 100
	}
	if config.Projects.DefaultProject == "" {
		config.Projects.DefaultProject = defaults.Projects.DefaultProject
	}
	if len(config.Projects.Items) == 0 {
		config.Projects.Items = defaults.Projects.Items
	}
	if len(config.Migration.AllowedLocalRoots) == 0 {
		config.Migration.AllowedLocalRoots = defaults.Migration.AllowedLocalRoots
	}
	if config.Migration.MaxLogLines <= 0 {
		config.Migration.MaxLogLines = defaults.Migration.MaxLogLines
	}
	if config.Migration.SSH.ConnectTimeoutSec <= 0 {
		config.Migration.SSH.ConnectTimeoutSec = defaults.Migration.SSH.ConnectTimeoutSec
	}
	config.StorageBackend = config.Storage.Backend
	config.DBConnection = config.Storage.Backend
	switch engine {
	case "postgres":
		config.DBHost = config.Database.Postgres.Host
		config.DBPort = config.Database.Postgres.Port
		config.DBUsername = config.Database.Postgres.User
		config.DBPassword = config.Database.Postgres.Password
		config.DBDatabase = config.Database.Postgres.Database
	case "mongodb":
		config.DBHost = config.Database.MongoDB.Host
		config.DBPort = config.Database.MongoDB.Port
		config.DBUsername = config.Database.MongoDB.User
		config.DBPassword = config.Database.MongoDB.Password
		config.DBDatabase = config.Database.MongoDB.Database
	default:
		config.DBHost = config.Database.MySQL.Host
		config.DBPort = config.Database.MySQL.Port
		config.DBUsername = config.Database.MySQL.User
		config.DBPassword = config.Database.MySQL.Password
		config.DBDatabase = config.Database.MySQL.Database
	}
}

func Default() *Config {
	cfg := &Config{
		Server: struct {
			Port string `yaml:"port"`
			Mode string `yaml:"mode"`
		}{
			Port: "8080",
			Mode: "debug",
		},
		Storage: struct {
			Backend        string `yaml:"backend"`
			DataPath       string `yaml:"data_path"`
			PDFsPath       string `yaml:"pdfs_path"`
			ImagesPath     string `yaml:"images_path"`
			DocumentsPath  string `yaml:"documents_path"`
			ThumbnailsPath string `yaml:"thumbnails_path"`
			ManifestsPath  string `yaml:"manifests_path"`
		}{
			Backend:        "local",
			DataPath:       "./data",
			PDFsPath:       "./data/pdfs",
			ImagesPath:     "./data/images",
			DocumentsPath:  "./data/documents",
			ThumbnailsPath: "./data/thumbnails",
			ManifestsPath:  "./data/manifests",
		},
		Database: struct {
			AutoMigrate bool `yaml:"auto_migrate"`
			MySQL       struct {
				Host      string `yaml:"host"`
				Port      string `yaml:"port"`
				User      string `yaml:"user"`
				Password  string `yaml:"password"`
				Database  string `yaml:"database"`
				Charset   string `yaml:"charset"`
				ParseTime bool   `yaml:"parse_time"`
			} `yaml:"mysql"`
			Postgres struct {
				Host     string `yaml:"host"`
				Port     string `yaml:"port"`
				User     string `yaml:"user"`
				Password string `yaml:"password"`
				Database string `yaml:"database"`
				SSLMode  string `yaml:"sslmode"`
				Schema   string `yaml:"schema"`
			} `yaml:"postgres"`
			MongoDB struct {
				Host                     string `yaml:"host"`
				Port                     string `yaml:"port"`
				User                     string `yaml:"user"`
				Password                 string `yaml:"password"`
				Database                 string `yaml:"database"`
				AuthSource               string `yaml:"auth_source"`
				DirectConnection         bool   `yaml:"direct_connection"`
				ServerSelectionTimeoutMS int    `yaml:"server_selection_timeout_ms"`
			} `yaml:"mongodb"`
		}{
			MySQL: struct {
				Host      string `yaml:"host"`
				Port      string `yaml:"port"`
				User      string `yaml:"user"`
				Password  string `yaml:"password"`
				Database  string `yaml:"database"`
				Charset   string `yaml:"charset"`
				ParseTime bool   `yaml:"parse_time"`
			}{
				Host:      "127.0.0.1",
				Port:      "3306",
				User:      "project_iiif",
				Password:  "",
				Database:  "project_iiif",
				Charset:   "utf8mb4",
				ParseTime: true,
			},
			Postgres: struct {
				Host     string `yaml:"host"`
				Port     string `yaml:"port"`
				User     string `yaml:"user"`
				Password string `yaml:"password"`
				Database string `yaml:"database"`
				SSLMode  string `yaml:"sslmode"`
				Schema   string `yaml:"schema"`
			}{
				Host:     "127.0.0.1",
				Port:     "5432",
				User:     "postgres",
				Password: "",
				Database: "project_iiif",
				SSLMode:  "disable",
				Schema:   "public",
			},
			MongoDB: struct {
				Host                     string `yaml:"host"`
				Port                     string `yaml:"port"`
				User                     string `yaml:"user"`
				Password                 string `yaml:"password"`
				Database                 string `yaml:"database"`
				AuthSource               string `yaml:"auth_source"`
				DirectConnection         bool   `yaml:"direct_connection"`
				ServerSelectionTimeoutMS int    `yaml:"server_selection_timeout_ms"`
			}{
				Host:                     "127.0.0.1",
				Port:                     "27017",
				User:                     "",
				Password:                 "",
				Database:                 "project_iiif",
				AuthSource:               "admin",
				DirectConnection:         true,
				ServerSelectionTimeoutMS: 2000,
			},
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
		BinaryStorage: struct {
			Mode     string `yaml:"mode"`
			TempPath string `yaml:"temp_path"`
		}{
			Mode:     "database",
			TempPath: "./data/temp",
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
			DefaultWidth    int    `yaml:"default_width"`
			DefaultHeight   int    `yaml:"default_height"`
			DefaultFormat   string `yaml:"default_format"`
			DefaultQuality  int    `yaml:"default_quality"`
			EnableOCR       bool   `yaml:"enable_ocr"`
			DPI             int    `yaml:"dpi"`
			BackgroundColor string `yaml:"background_color"`
		}{
			DefaultWidth:    1241,
			DefaultHeight:   1754,
			DefaultFormat:   "jpg",
			DefaultQuality:  85,
			EnableOCR:       false,
			DPI:             150,
			BackgroundColor: "white",
		},
		OCR: func() OCRConfig {
			value := OCRConfig{
				Enabled: false, AutoAfterConversion: false, DefaultMode: "hybrid", Workers: 2,
				PageTimeoutSeconds: 120, RetriesPerPage: 2, RenderDPI: 300, MinTextChars: 40,
				CandidateLanguages: []string{"spa", "eng", "fra", "por"}, FallbackLanguages: []string{"spa"},
			}
			value.LanguageDetection.Enabled = true
			value.LanguageDetection.SamplePages = 5
			value.LanguageDetection.MinSampleChars = 200
			value.LanguageDetection.MinimumConfidence = 0.70
			value.LanguageDetection.MaxLanguages = 2
			value.Artifacts.Gzip = true
			return value
		}(),
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
		Frontend: struct {
			Enabled         bool   `yaml:"enabled"`
			Path            string `yaml:"path"`
			RequireAuth     bool   `yaml:"require_auth"`
			Username        string `yaml:"username"`
			Password        string `yaml:"password"`
			MenuOrientation string `yaml:"menu_orientation"`
		}{
			Enabled:         false,
			Path:            "./frontend",
			RequireAuth:     true,
			Username:        "admin",
			Password:        "CAMBIAR_PASSWORD",
			MenuOrientation: "horizontal",
		},
		Projects: struct {
			Enabled             bool            `yaml:"enabled"`
			DefaultProject      string          `yaml:"default_project"`
			RequireProject      bool            `yaml:"require_project"`
			AllowDynamicTenants bool            `yaml:"allow_dynamic_tenants"`
			Items               []ProjectConfig `yaml:"items"`
		}{
			Enabled:             false,
			DefaultProject:      "default",
			RequireProject:      false,
			AllowDynamicTenants: false,
			Items: []ProjectConfig{
				{Key: "default", Name: "Proyecto por defecto", Multitenant: false, Tenants: []string{}},
			},
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
		Migration: struct {
			Enabled           bool     `yaml:"enabled"`
			AllowedLocalRoots []string `yaml:"allowed_local_roots"`
			MaxLogLines       int      `yaml:"max_log_lines"`
			SSH               struct {
				ConnectTimeoutSec int      `yaml:"connect_timeout_sec"`
				AllowedHosts      []string `yaml:"allowed_hosts"`
			} `yaml:"ssh"`
		}{
			Enabled:           true,
			AllowedLocalRoots: []string{"./data", "/var/lib/project_iiif"},
			MaxLogLines:       1000,
			SSH: struct {
				ConnectTimeoutSec int      `yaml:"connect_timeout_sec"`
				AllowedHosts      []string `yaml:"allowed_hosts"`
			}{
				ConnectTimeoutSec: 15,
				AllowedHosts:      []string{},
			},
		},
	}
	cfg.StorageBackend = cfg.Storage.Backend
	cfg.DBConnection = cfg.Storage.Backend
	cfg.DBHost = cfg.Database.MySQL.Host
	cfg.DBPort = cfg.Database.MySQL.Port
	cfg.DBUsername = cfg.Database.MySQL.User
	cfg.DBPassword = cfg.Database.MySQL.Password
	cfg.DBDatabase = cfg.Database.MySQL.Database
	return cfg
}

func (config *Config) ResolveScope(project, tenant string) (*models.Scope, error) {
	project = strings.TrimSpace(project)
	tenant = strings.TrimSpace(tenant)
	if project == "" {
		project = config.Projects.DefaultProject
	}
	if project == "" {
		project = "default"
	}

	if !config.Projects.Enabled {
		return &models.Scope{ProjectKey: project}, nil
	}
	if config.Projects.RequireProject && project == "" {
		return nil, fmt.Errorf("project es obligatorio")
	}

	item, ok := config.ProjectByKey(project)
	if !ok {
		return nil, fmt.Errorf("project no configurado: %s", project)
	}
	if !item.Multitenant {
		return &models.Scope{ProjectKey: project}, nil
	}
	if tenant == "" {
		return nil, fmt.Errorf("tenant es obligatorio para el proyecto %s", project)
	}
	if !config.Projects.AllowDynamicTenants && !hasTenant(item, tenant) {
		return nil, fmt.Errorf("tenant no configurado para %s: %s", project, tenant)
	}
	return &models.Scope{ProjectKey: project, TenantKey: tenant}, nil
}

func (config *Config) ProjectByKey(key string) (ProjectConfig, bool) {
	for _, item := range config.Projects.Items {
		if strings.EqualFold(item.Key, key) {
			return item, true
		}
	}
	return ProjectConfig{}, false
}

func hasTenant(project ProjectConfig, tenant string) bool {
	for _, item := range project.Tenants {
		if strings.EqualFold(item, tenant) {
			return true
		}
	}
	return false
}

func normalizedEngine(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "postgresql":
		return "postgres"
	case "mongo":
		return "mongodb"
	case "":
		return "local"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
