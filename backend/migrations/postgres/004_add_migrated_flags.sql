ALTER TABLE documents
  ADD COLUMN IF NOT EXISTS migrated_from_local BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_documents_migrated_from_local ON documents(migrated_from_local);

ALTER TABLE document_images
  ADD COLUMN IF NOT EXISTS migrated_from_local BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_document_images_migrated_from_local ON document_images(migrated_from_local);
