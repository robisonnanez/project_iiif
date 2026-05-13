CREATE TABLE IF NOT EXISTS documents (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  original_name VARCHAR(255) NOT NULL,
  status VARCHAR(32) NOT NULL,
  total_pages INT NOT NULL DEFAULT 0,
  converted_pages INT NOT NULL DEFAULT 0,
  pdf_path TEXT NOT NULL,
  thumbnail_path TEXT NULL,
  manifest_url TEXT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_documents_status (status),
  INDEX idx_documents_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS document_images (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  document_id VARCHAR(64) NOT NULL,
  page_number INT NOT NULL,
  image_path TEXT NOT NULL,
  width INT NOT NULL DEFAULT 0,
  height INT NOT NULL DEFAULT 0,
  format VARCHAR(16) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_document_page (document_id, page_number),
  INDEX idx_document_images_document_id (document_id),
  CONSTRAINT fk_document_images_document
    FOREIGN KEY (document_id) REFERENCES documents(id)
    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
