ALTER TABLE documents
  ADD COLUMN IF NOT EXISTS project_key VARCHAR(128) NOT NULL DEFAULT 'default',
  ADD COLUMN IF NOT EXISTS tenant_key VARCHAR(128) NULL;

CREATE INDEX IF NOT EXISTS idx_documents_project_tenant_created ON documents(project_key, tenant_key, created_at);
CREATE INDEX IF NOT EXISTS idx_documents_project_created ON documents(project_key, created_at);

ALTER TABLE document_images
  ADD COLUMN IF NOT EXISTS project_key VARCHAR(128) NOT NULL DEFAULT 'default',
  ADD COLUMN IF NOT EXISTS tenant_key VARCHAR(128) NULL;

CREATE INDEX IF NOT EXISTS idx_document_images_project_tenant ON document_images(project_key, tenant_key);
CREATE INDEX IF NOT EXISTS idx_document_images_project_document ON document_images(project_key, document_id);

UPDATE documents SET project_key = 'default' WHERE project_key IS NULL OR project_key = '';
UPDATE document_images SET project_key = 'default' WHERE project_key IS NULL OR project_key = '';
