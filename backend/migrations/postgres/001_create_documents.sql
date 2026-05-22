CREATE TABLE IF NOT EXISTS documents (
  id VARCHAR(64) PRIMARY KEY,
  original_name VARCHAR(255) NOT NULL,
  status VARCHAR(32) NOT NULL,
  total_pages INT NOT NULL DEFAULT 0,
  converted_pages INT NOT NULL DEFAULT 0,
  pdf_path TEXT NULL,
  thumbnail_path TEXT NULL,
  manifest_url TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(status);
CREATE INDEX IF NOT EXISTS idx_documents_created_at ON documents(created_at);

CREATE TABLE IF NOT EXISTS document_images (
  id VARCHAR(64) PRIMARY KEY,
  document_id VARCHAR(64) NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  page_number INT NOT NULL,
  image_path TEXT NULL,
  width INT NOT NULL DEFAULT 0,
  height INT NOT NULL DEFAULT 0,
  format VARCHAR(16) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uq_document_page UNIQUE(document_id, page_number)
);

CREATE INDEX IF NOT EXISTS idx_document_images_document_id ON document_images(document_id);
