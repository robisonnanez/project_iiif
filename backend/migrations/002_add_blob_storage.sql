ALTER TABLE documents
  MODIFY pdf_path TEXT NULL,
  ADD COLUMN pdf_blob LONGBLOB NULL AFTER pdf_path,
  ADD COLUMN pdf_media_type VARCHAR(128) NULL AFTER pdf_blob,
  ADD COLUMN pdf_size BIGINT NULL AFTER pdf_media_type;

ALTER TABLE document_images
  MODIFY image_path TEXT NULL,
  ADD COLUMN image_blob LONGBLOB NULL AFTER image_path,
  ADD COLUMN media_type VARCHAR(128) NULL AFTER image_blob,
  ADD COLUMN byte_size BIGINT NULL AFTER media_type;
