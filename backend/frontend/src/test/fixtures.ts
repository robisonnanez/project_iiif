import type { AppConfig } from "../types";

export const sampleConfig: AppConfig = {
  server: { port: "8080", mode: "debug" },
  storage: { backend: "mysql", data_path: "./data", pdfs_path: "./data/pdfs", images_path: "./data/images", documents_path: "./data/documents", thumbnails_path: "./data/thumbnails", manifests_path: "./data/manifests" },
  database: {
    DB_CONNECTION: "mysql", DB_HOST: "mysql", DB_PORT: "3306", DB_DATABASE: "project_iiif", DB_USERNAME: "app", DB_PASSWORD: "********",
    mysql: { host: "mysql", port: "3306", user: "app", password: "********", password_configured: true, database: "project_iiif", charset: "utf8mb4", parse_time: true },
    postgres: { host: "postgres", port: "5432", user: "app", password: "", database: "project_iiif", sslmode: "disable", schema: "public" },
    mongodb: { host: "mongodb", port: "27017", user: "app", password: "********", password_configured: true, database: "project_iiif", auth_source: "admin", direct_connection: true, server_selection_timeout_ms: 2000 },
  },
  frontend: { enabled: true, path: "./frontend", require_auth: false, username: "admin", password: "" },
  binary_storage: { mode: "local", temp_path: "./data/temp" },
  s3: { filesystem_disk: "local", access_key_id: "", secret_access_key: "", region: "us-east-1", bucket: "project-iiif", endpoint: "http://rustfs:9000", use_path_style_endpoint: true },
  iiif: { base_url: "http://localhost:8080", api_version: "3", max_width: 2048, max_height: 2048, cache: true, cache_ttl: 3600 },
  conversion: { default_width: 1241, default_height: 1754, dpi: 150, default_format: "jpg", default_quality: 85, enable_ocr: false },
  projects: { enabled: false, default_project: "default", require_project: false, allow_dynamic_tenants: false, items: [] },
  security: { enable_auth: false, log_level: "info", cors_origins: [], max_concurrent_uploads: 5 },
};
