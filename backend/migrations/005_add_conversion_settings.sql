ALTER TABLE documents
  ADD COLUMN conversion_width INT NULL AFTER converted_pages,
  ADD COLUMN conversion_height INT NULL AFTER conversion_width,
  ADD COLUMN conversion_dpi INT NULL AFTER conversion_height,
  ADD COLUMN conversion_format VARCHAR(16) NULL AFTER conversion_dpi,
  ADD COLUMN conversion_quality INT NULL AFTER conversion_format;
