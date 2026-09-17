CREATE TABLE document_annotations (
  id VARCHAR(64) PRIMARY KEY,
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
  selector JSONB NOT NULL,
  color VARCHAR(20) NULL,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL,
  CONSTRAINT uq_document_annotations_tenant_client UNIQUE (tenant_id, client_reference_id),
  CONSTRAINT fk_document_annotations_document
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE RESTRICT,
  CONSTRAINT fk_document_annotations_page
    FOREIGN KEY (page_id) REFERENCES document_images(id) ON DELETE RESTRICT
);

CREATE INDEX idx_document_annotations_scope_page
  ON document_annotations(tenant_id, project_key, document_id, page_id);
CREATE INDEX idx_document_annotations_generation
  ON document_annotations(document_id, ocr_generation_id);
CREATE INDEX idx_document_annotations_deleted_at
  ON document_annotations(deleted_at);
