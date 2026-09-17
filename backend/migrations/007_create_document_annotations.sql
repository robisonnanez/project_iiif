CREATE TABLE document_annotations (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  project_key VARCHAR(128) NOT NULL,
  tenant_id VARCHAR(128) NOT NULL,
  document_id VARCHAR(64) NOT NULL,
  page_id VARCHAR(64) NOT NULL,
  page_number INT NOT NULL,
  ocr_generation_id VARCHAR(64) NOT NULL,
  layer_sha256 CHAR(64) NOT NULL,
  client_reference_id VARCHAR(64) NOT NULL,
  request_fingerprint CHAR(64) NOT NULL,
  selected_text TEXT NOT NULL,
  note TEXT NULL,
  selector JSON NOT NULL,
  color VARCHAR(20) NULL,
  version BIGINT NOT NULL DEFAULT 1,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  deleted_at DATETIME(6) NULL,
  UNIQUE KEY uq_document_annotations_tenant_client (tenant_id, client_reference_id),
  INDEX idx_document_annotations_scope_page (tenant_id, project_key, document_id, page_id),
  INDEX idx_document_annotations_generation (document_id, ocr_generation_id),
  INDEX idx_document_annotations_deleted_at (deleted_at),
  CONSTRAINT fk_document_annotations_document
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE RESTRICT,
  CONSTRAINT fk_document_annotations_page
    FOREIGN KEY (page_id) REFERENCES document_images(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
