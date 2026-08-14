package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresStorage struct {
	db       *sql.DB
	basePath string
}

func NewPostgresStorage(cfg *config.Config) (*PostgresStorage, error) {
	pg := cfg.Database.Postgres
	if pg.SSLMode == "" {
		pg.SSLMode = "disable"
	}
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		pg.Host, pg.Port, pg.User, pg.Password, pg.Database, pg.SSLMode,
	)
	if strings.TrimSpace(pg.Schema) != "" {
		dsn += " search_path=" + pg.Schema
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &PostgresStorage{db: db, basePath: cfg.Storage.DataPath}, nil
}

func (ps *PostgresStorage) SaveDocument(doc *models.PDFDocument) error {
	return ps.upsertDocument(doc)
}

func (ps *PostgresStorage) UpdateDocument(doc *models.PDFDocument) error {
	return ps.upsertDocument(doc)
}

func (ps *PostgresStorage) GetDocument(id string) (*models.PDFDocument, error) {
	row := ps.db.QueryRow(`
		SELECT id, original_name, COALESCE(project_key, 'default'), COALESCE(tenant_key, ''), COALESCE(migrated_from_local, false), status, total_pages, converted_pages,
		       COALESCE(conversion_width, 1241), COALESCE(conversion_height, 1754), COALESCE(conversion_dpi, 150), COALESCE(conversion_format, 'jpg'), COALESCE(conversion_quality, 85),
		       pdf_path, thumbnail_path, manifest_url, created_at
		FROM documents
		WHERE id = $1
	`, id)
	doc := &models.PDFDocument{}
	var pdfPath, thumbnailPath, manifestURL sql.NullString
	if err := row.Scan(
		&doc.ID, &doc.Name, &doc.ProjectKey, &doc.TenantKey, &doc.MigratedFromLocal, &doc.Status,
		&doc.TotalPages, &doc.ConvertedPages, &doc.ConversionWidth, &doc.ConversionHeight, &doc.ConversionDPI,
		&doc.ConversionFormat, &doc.ConversionQuality, &pdfPath, &thumbnailPath, &manifestURL, &doc.UploadDate,
	); err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}
	doc.FilePath = pdfPath.String
	doc.ThumbnailURL = thumbnailPath.String
	doc.ManifestURL = manifestURL.String
	doc.ImagePaths = ps.getImagePaths(doc.ID)
	return doc, nil
}

func (ps *PostgresStorage) GetAllDocuments() ([]*models.PDFDocument, error) {
	return ps.GetDocumentsByScope("", "")
}

func (ps *PostgresStorage) GetDocumentsByScope(projectKey, tenantKey string) ([]*models.PDFDocument, error) {
	query := `
		SELECT id, original_name, COALESCE(project_key, 'default'), COALESCE(tenant_key, ''), COALESCE(migrated_from_local, false), status, total_pages, converted_pages,
		       COALESCE(conversion_width, 1241), COALESCE(conversion_height, 1754), COALESCE(conversion_dpi, 150), COALESCE(conversion_format, 'jpg'), COALESCE(conversion_quality, 85),
		       pdf_path, thumbnail_path, manifest_url, created_at
		FROM documents`
	var conditions []string
	var args []interface{}
	argPos := 1
	if strings.TrimSpace(projectKey) != "" {
		conditions = append(conditions, fmt.Sprintf("project_key = $%d", argPos))
		args = append(args, projectKey)
		argPos++
	}
	if strings.TrimSpace(tenantKey) != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_key = $%d", argPos))
		args = append(args, tenantKey)
		argPos++
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	rows, err := ps.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []*models.PDFDocument{}
	for rows.Next() {
		doc := &models.PDFDocument{}
		var pdfPath, thumbnailPath, manifestURL sql.NullString
		if err := rows.Scan(
			&doc.ID, &doc.Name, &doc.ProjectKey, &doc.TenantKey, &doc.MigratedFromLocal, &doc.Status,
			&doc.TotalPages, &doc.ConvertedPages, &doc.ConversionWidth, &doc.ConversionHeight, &doc.ConversionDPI,
			&doc.ConversionFormat, &doc.ConversionQuality, &pdfPath, &thumbnailPath, &manifestURL, &doc.UploadDate,
		); err != nil {
			return nil, err
		}
		doc.FilePath = pdfPath.String
		doc.ThumbnailURL = thumbnailPath.String
		doc.ManifestURL = manifestURL.String
		doc.ImagePaths = ps.getImagePaths(doc.ID)
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (ps *PostgresStorage) DeleteDocument(id string) error {
	doc, _ := ps.GetDocument(id)
	if _, err := ps.db.Exec("DELETE FROM documents WHERE id = $1", id); err != nil {
		return err
	}
	if doc != nil {
		_ = os.Remove(doc.FilePath)
	}
	_ = os.RemoveAll(filepath.Join(ps.basePath, "images", id))
	_ = os.Remove(filepath.Join(ps.basePath, "thumbnails", id+".jpg"))
	_ = os.Remove(filepath.Join(ps.basePath, "manifests", id+".json"))
	return nil
}

func (ps *PostgresStorage) SaveDocumentPDF(documentID string, data []byte, mediaType string) error {
	_, err := ps.db.Exec(`
		UPDATE documents SET pdf_blob = $1, pdf_media_type = $2, pdf_size = $3 WHERE id = $4
	`, data, mediaType, len(data), documentID)
	return err
}

func (ps *PostgresStorage) GetDocumentPDFData(documentID string) (*models.BinaryAsset, error) {
	row := ps.db.QueryRow(`SELECT id, pdf_blob, pdf_media_type, pdf_size FROM documents WHERE id = $1`, documentID)
	asset := &models.BinaryAsset{}
	var data []byte
	var mediaType sql.NullString
	var byteSize sql.NullInt64
	if err := row.Scan(&asset.ID, &data, &mediaType, &byteSize); err != nil {
		return nil, fmt.Errorf("pdf blob not found: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("pdf blob is empty")
	}
	asset.Data = data
	asset.MediaType = mediaType.String
	asset.ByteSize = byteSize.Int64
	if asset.ByteSize == 0 {
		asset.ByteSize = int64(len(data))
	}
	return asset, nil
}

func (ps *PostgresStorage) SaveDocumentImage(image *models.DocumentImage) error {
	if image.CreatedAt.IsZero() {
		image.CreatedAt = time.Now()
	}
	_, err := ps.db.Exec(`
		INSERT INTO document_images (id, document_id, project_key, tenant_key, migrated_from_local, page_number, image_path, width, height, format, media_type, byte_size, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (id) DO UPDATE SET
			document_id = EXCLUDED.document_id,
			project_key = EXCLUDED.project_key,
			tenant_key = EXCLUDED.tenant_key,
			migrated_from_local = EXCLUDED.migrated_from_local,
			page_number = EXCLUDED.page_number,
			image_path = EXCLUDED.image_path,
			width = EXCLUDED.width,
			height = EXCLUDED.height,
			format = EXCLUDED.format,
			media_type = EXCLUDED.media_type,
			byte_size = EXCLUDED.byte_size
	`, image.ID, image.DocumentID, nullString(defaultProject(image.ProjectKey)), nullString(image.TenantKey), image.MigratedFromLocal, image.PageNumber, nullString(image.ImagePath), image.Width, image.Height, image.Format, nullString(image.MediaType), image.ByteSize, image.CreatedAt)
	return err
}

func (ps *PostgresStorage) SaveDocumentImageData(imageID string, data []byte, mediaType string) error {
	_, err := ps.db.Exec(`
		UPDATE document_images SET image_blob = $1, media_type = $2, byte_size = $3 WHERE id = $4
	`, data, mediaType, len(data), imageID)
	return err
}

func (ps *PostgresStorage) GetDocumentImage(id string) (*models.DocumentImage, error) {
	row := ps.db.QueryRow(`
		SELECT id, document_id, COALESCE(project_key, 'default'), COALESCE(tenant_key, ''), COALESCE(migrated_from_local, false), page_number, image_path, width, height, format, media_type, byte_size, created_at
		FROM document_images WHERE id = $1
	`, id)
	return scanDocumentImage(row)
}

func (ps *PostgresStorage) GetDocumentImageByPage(documentID string, page int) (*models.DocumentImage, error) {
	row := ps.db.QueryRow(`
		SELECT id, document_id, COALESCE(project_key, 'default'), COALESCE(tenant_key, ''), COALESCE(migrated_from_local, false), page_number, image_path, width, height, format, media_type, byte_size, created_at
		FROM document_images WHERE document_id = $1 AND page_number = $2
	`, documentID, page)
	return scanDocumentImage(row)
}

func (ps *PostgresStorage) GetDocumentImages(documentID string) ([]*models.DocumentImage, error) {
	rows, err := ps.db.Query(`
		SELECT id, document_id, COALESCE(project_key, 'default'), COALESCE(tenant_key, ''), COALESCE(migrated_from_local, false), page_number, image_path, width, height, format, media_type, byte_size, created_at
		FROM document_images WHERE document_id = $1 ORDER BY page_number
	`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var images []*models.DocumentImage
	for rows.Next() {
		image, err := scanDocumentImageRows(rows)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}

func (ps *PostgresStorage) GetDocumentImageData(id string) (*models.BinaryAsset, error) {
	row := ps.db.QueryRow(`
		SELECT id, image_blob, media_type, byte_size FROM document_images WHERE id = $1
	`, id)
	asset := &models.BinaryAsset{}
	var data []byte
	var mediaType sql.NullString
	var byteSize sql.NullInt64
	if err := row.Scan(&asset.ID, &data, &mediaType, &byteSize); err != nil {
		return nil, fmt.Errorf("image blob not found: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image blob is empty")
	}
	asset.Data = data
	asset.MediaType = mediaType.String
	asset.ByteSize = byteSize.Int64
	if asset.ByteSize == 0 {
		asset.ByteSize = int64(len(data))
	}
	return asset, nil
}

func (ps *PostgresStorage) upsertDocument(doc *models.PDFDocument) error {
	if doc.UploadDate.IsZero() {
		doc.UploadDate = time.Now()
	}
	_, err := ps.db.Exec(`
		INSERT INTO documents (id, original_name, project_key, tenant_key, migrated_from_local, status, total_pages, converted_pages, conversion_width, conversion_height, conversion_dpi, conversion_format, conversion_quality, pdf_path, thumbnail_path, manifest_url, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NOW())
		ON CONFLICT (id) DO UPDATE SET
			original_name = EXCLUDED.original_name,
			project_key = EXCLUDED.project_key,
			tenant_key = EXCLUDED.tenant_key,
			migrated_from_local = EXCLUDED.migrated_from_local,
			status = EXCLUDED.status,
			total_pages = EXCLUDED.total_pages,
			converted_pages = EXCLUDED.converted_pages,
			conversion_width = EXCLUDED.conversion_width,
			conversion_height = EXCLUDED.conversion_height,
			conversion_dpi = EXCLUDED.conversion_dpi,
			conversion_format = EXCLUDED.conversion_format,
			conversion_quality = EXCLUDED.conversion_quality,
			pdf_path = EXCLUDED.pdf_path,
			thumbnail_path = EXCLUDED.thumbnail_path,
			manifest_url = EXCLUDED.manifest_url,
			updated_at = NOW()
	`, doc.ID, doc.Name, nullString(defaultProject(doc.ProjectKey)), nullString(doc.TenantKey), doc.MigratedFromLocal, doc.Status, doc.TotalPages, doc.ConvertedPages,
		doc.ConversionWidth, doc.ConversionHeight, doc.ConversionDPI, nullString(doc.ConversionFormat), doc.ConversionQuality,
		nullString(doc.FilePath), nullString(doc.ThumbnailURL), nullString(doc.ManifestURL), doc.UploadDate)
	return err
}

func (ps *PostgresStorage) getImagePaths(documentID string) []string {
	rows, err := ps.db.Query("SELECT image_path FROM document_images WHERE document_id = $1 AND image_path IS NOT NULL ORDER BY page_number", documentID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}

func (ps *PostgresStorage) HasDocumentPDFBlob(documentID string) (bool, error) {
	row := ps.db.QueryRow("SELECT COALESCE(pdf_size, 0) FROM documents WHERE id = $1", documentID)
	var pdfSize int64
	if err := row.Scan(&pdfSize); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return pdfSize > 0, nil
}

func (ps *PostgresStorage) HasImageBlob(imageID string) (bool, error) {
	row := ps.db.QueryRow("SELECT COALESCE(byte_size, 0) FROM document_images WHERE id = $1", imageID)
	var byteSize int64
	if err := row.Scan(&byteSize); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return byteSize > 0, nil
}
