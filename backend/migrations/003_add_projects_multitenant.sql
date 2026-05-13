ALTER TABLE documents
  ADD COLUMN project_key VARCHAR(128) NOT NULL DEFAULT 'default' AFTER original_name,
  ADD COLUMN tenant_key VARCHAR(128) NULL AFTER project_key,
  ADD INDEX idx_documents_project_tenant_created (project_key, tenant_key, created_at),
  ADD INDEX idx_documents_project_created (project_key, created_at);

ALTER TABLE document_images
  ADD COLUMN project_key VARCHAR(128) NOT NULL DEFAULT 'default' AFTER document_id,
  ADD COLUMN tenant_key VARCHAR(128) NULL AFTER project_key,
  ADD INDEX idx_document_images_project_tenant (project_key, tenant_key),
  ADD INDEX idx_document_images_project_document (project_key, document_id);

UPDATE documents SET project_key = 'default' WHERE project_key IS NULL OR project_key = '';
UPDATE document_images SET project_key = 'default' WHERE project_key IS NULL OR project_key = '';
