export type Engine = "mysql" | "postgres" | "mongodb";
export type BinaryMode = "local" | "database" | "s3";

export interface ProjectConfig {
  key: string;
  name: string;
  multitenant: boolean;
  tenants: string[];
}

export interface DocumentRecord {
  id: string;
  name: string;
  projectKey?: string;
  tenantKey?: string;
  migratedFromLocal: boolean;
  status: "processing" | "completed" | "error" | string;
  totalPages: number;
  convertedPages: number;
  manifestUrl?: string;
  thumbnailUrl?: string;
}

interface SQLConfig {
  host: string;
  port: string;
  user: string;
  password: string;
  password_configured?: boolean;
  database: string;
}

export interface AppConfig {
  server: { port: string; mode: string };
  storage: {
    backend: Engine | "local";
    data_path: string;
    pdfs_path: string;
    images_path: string;
    documents_path: string;
    thumbnails_path: string;
    manifests_path: string;
  };
  database: {
    DB_CONNECTION: Engine | "local";
    DB_HOST: string;
    DB_PORT: string;
    DB_DATABASE: string;
    DB_USERNAME: string;
    DB_PASSWORD: string;
    mysql: SQLConfig & { charset: string; parse_time: boolean };
    postgres: SQLConfig & { sslmode: string; schema: string };
    mongodb: {
      host: string;
      port: string;
      user: string;
      password: string;
      password_configured?: boolean;
      database: string;
      auth_source: string;
      direct_connection: boolean;
      server_selection_timeout_ms: number;
    };
  };
  frontend: { enabled: boolean; path: string; require_auth: boolean; username: string; password: string };
  binary_storage: { mode: BinaryMode; temp_path: string };
  s3: {
    filesystem_disk: string;
    access_key_id: string;
    secret_access_key: string;
    access_key_configured?: boolean;
    secret_key_configured?: boolean;
    region: string;
    bucket: string;
    endpoint: string;
    use_path_style_endpoint: boolean;
  };
  iiif: { base_url: string; api_version: string; max_width: number; max_height: number; cache: boolean; cache_ttl: number };
  conversion: { default_width: number; default_height: number; dpi: number; default_format: "jpg" | "png"; default_quality: number; enable_ocr: boolean };
  projects: { enabled: boolean; default_project: string; require_project: boolean; allow_dynamic_tenants: boolean; items: ProjectConfig[] };
  security: { enable_auth: boolean; log_level: string; cors_origins: string[]; max_concurrent_uploads: number };
}

export interface UploadSettings {
  width: number;
  height: number;
  dpi: number;
  format: "jpg" | "png";
  quality: number;
}

export interface UploadScope {
  project: string;
  tenant: string;
}
