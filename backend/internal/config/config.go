package config

import (
	"os"
	"path/filepath"

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
		MySQL struct {
			Host      string `yaml:"host"`
			Port      string `yaml:"port"`
			User      string `yaml:"user"`
			Password  string `yaml:"password"`
			Database  string `yaml:"database"`
			Charset   string `yaml:"charset"`
			ParseTime bool   `yaml:"parse_time"`
		} `yaml:"mysql"`
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

	Frontend struct {
		Enabled     bool   `yaml:"enabled"`
		Path        string `yaml:"path"`
		RequireAuth bool   `yaml:"require_auth"`
		Username    string `yaml:"username"`
		Password    string `yaml:"password"`
	} `yaml:"frontend"`

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

	applyDefaults(&config)
	config.SourcePath = resolvedPath(filename)
	return &config, nil
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
	if config.StorageBackend != "" {
		config.Storage.Backend = config.StorageBackend
	}
	if config.DBConnection != "" {
		config.Storage.Backend = config.DBConnection
	}
	if config.DBHost != "" {
		config.Database.MySQL.Host = config.DBHost
	}
	if config.DBPort != "" {
		config.Database.MySQL.Port = config.DBPort
	}
	if config.DBUsername != "" {
		config.Database.MySQL.User = config.DBUsername
	}
	if config.DBPassword != "" {
		config.Database.MySQL.Password = config.DBPassword
	}
	if config.DBDatabase != "" {
		config.Database.MySQL.Database = config.DBDatabase
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
	if config.BinaryStorage.Mode == "" {
		if config.Storage.Backend == "local" {
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
	config.StorageBackend = config.Storage.Backend
	config.DBConnection = config.Storage.Backend
	config.DBHost = config.Database.MySQL.Host
	config.DBPort = config.Database.MySQL.Port
	config.DBUsername = config.Database.MySQL.User
	config.DBPassword = config.Database.MySQL.Password
	config.DBDatabase = config.Database.MySQL.Database
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
			MySQL struct {
				Host      string `yaml:"host"`
				Port      string `yaml:"port"`
				User      string `yaml:"user"`
				Password  string `yaml:"password"`
				Database  string `yaml:"database"`
				Charset   string `yaml:"charset"`
				ParseTime bool   `yaml:"parse_time"`
			} `yaml:"mysql"`
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
		Frontend: struct {
			Enabled     bool   `yaml:"enabled"`
			Path        string `yaml:"path"`
			RequireAuth bool   `yaml:"require_auth"`
			Username    string `yaml:"username"`
			Password    string `yaml:"password"`
		}{
			Enabled:     false,
			Path:        "./frontend",
			RequireAuth: true,
			Username:    "admin",
			Password:    "CAMBIAR_PASSWORD",
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
	cfg.StorageBackend = cfg.Storage.Backend
	cfg.DBConnection = cfg.Storage.Backend
	cfg.DBHost = cfg.Database.MySQL.Host
	cfg.DBPort = cfg.Database.MySQL.Port
	cfg.DBUsername = cfg.Database.MySQL.User
	cfg.DBPassword = cfg.Database.MySQL.Password
	cfg.DBDatabase = cfg.Database.MySQL.Database
	return cfg
}
