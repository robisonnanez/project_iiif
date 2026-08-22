export type Engine = "mysql" | "postgres" | "mongodb";
export type BinaryMode = "local" | "database" | "s3";

export interface ProjectConfig {
  key: string;
  name: string;
  multitenant: boolean;
  tenants: string[];
  tenants_endpoint?: string;
  tenants_auth_type?: "none" | "bearer" | "api_key";
  tenants_auth_header?: string;
  tenants_auth_token?: string;
  tenants_token_configured?: boolean;
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
    auto_migrate: boolean;
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
  frontend: { enabled: boolean; path: string; require_auth: boolean; username: string; password: string; menu_orientation: "horizontal" | "vertical" };
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
  ocr: {
    enabled: boolean; auto_after_conversion: boolean; default_mode: "hybrid" | "exhaustive" | "ocr_only";
    workers: number; page_timeout_seconds: number; retries_per_page: number; render_dpi: number;
    min_text_chars: number; candidate_languages: string[]; fallback_languages: string[];
    language_detection: { enabled: boolean; sample_pages: number; min_sample_chars: number; minimum_confidence: number; max_languages: number };
    artifacts: { gzip: boolean };
  };
  projects: { enabled: boolean; default_project: string; require_project: boolean; allow_dynamic_tenants: boolean; items: ProjectConfig[] };
  security: { enable_auth: boolean; log_level: string; cors_origins: string[]; max_concurrent_uploads: number };
}

export interface OCRJob {
  id: string; document_id: string; generation: string; mode: string; languages: string[];
  status: string; total_pages: number; processed_pages: number; failed_pages: number;
  current_page?: number; error?: string;
}

export interface OCRSearchResult {
  document_id: string; page_number: number; canvas_v2: string; canvas_v3: string;
  image_id?: string; iiif_image?: string;
  source: string; snippet: string; score: number; matches: number;
}

export interface OCRSearchResponse { results: OCRSearchResult[]; total: number; limit: number; offset: number }

export interface DBMigrationResult {
  engine: string;
  pending_before: number;
  applied: number;
  skipped: number;
  duration_ms: number;
  message: string;
  errors?: string[];
  applied_files?: string[];
}

export interface DBMigrationStatus { running: boolean; result: DBMigrationResult }

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

export type NoticeTone = "success" | "danger" | "info";
export type Notify = (message: string, tone?: NoticeTone) => void;

export interface DocumentImage {
  image_id: string;
  document_id: string;
  project_key: string;
  tenant_key?: string;
  page_number: number;
  width: number;
  height: number;
  format: string;
  media_type: string;
  byte_size: number;
  migrated_from_local: boolean;
  iiif_url: string;
  info_url: string;
}

export interface DocumentImagesResponse {
  document_id: string;
  project_key: string;
  tenant_key?: string;
  images: DocumentImage[];
}

export interface MigrationItem {
  document_id: string;
  pdf_name: string;
  images_done: number;
  images_total: number;
  status: string;
  message?: string;
}

export interface MigrationStatus {
  running: boolean;
  started_at?: string;
  finished_at?: string;
  exit_code: number;
  message?: string;
  logs: string[];
  job_id?: string;
  source?: string;
  metrics?: Record<string, number>;
  progress_percent?: number;
  current_document?: string;
  items?: MigrationItem[];
}

export interface MigrationPayload {
  source: {
    type: "local" | "ssh" | "database";
    local?: { path: string };
    ssh?: { host: string; port: number; user: string; path: string; private_key: string };
  };
  scope: { project_key: string; tenant_key: string };
}

export interface MigrationDirectory {
  name: string;
  path: string;
}
