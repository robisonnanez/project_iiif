ALTER TABLE documents
  ADD COLUMN migrated_from_local TINYINT(1) NOT NULL DEFAULT 0 AFTER tenant_key,
  ADD INDEX idx_documents_migrated_from_local (migrated_from_local);

ALTER TABLE document_images
  ADD COLUMN migrated_from_local TINYINT(1) NOT NULL DEFAULT 0 AFTER tenant_key,
  ADD INDEX idx_document_images_migrated_from_local (migrated_from_local);
