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

	_ "github.com/go-sql-driver/mysql"
)

type MySQLStorage struct {
	db       *sql.DB
	basePath string
}

func NewMySQLStorage(cfg *config.Config) (*MySQLStorage, error) {
	mysql := cfg.Database.MySQL
	charset := mysql.Charset
	if charset == "" {
		charset = "utf8mb4"
	}

	parseTime := "false"
	if mysql.ParseTime {
		parseTime = "true"
	}

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=%s&loc=Local",
		mysql.User,
		mysql.Password,
		mysql.Host,
		mysql.Port,
		mysql.Database,
		charset,
		parseTime,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return &MySQLStorage{db: db, basePath: cfg.Storage.DataPath}, nil
}

func (ms *MySQLStorage) SaveDocument(doc *models.PDFDocument) error {
	return ms.upsertDocument(doc)
}

func (ms *MySQLStorage) UpdateDocument(doc *models.PDFDocument) error {
	return ms.upsertDocument(doc)
}

func (ms *MySQLStorage) GetDocument(id string) (*models.PDFDocument, error) {
	row := ms.db.QueryRow(`
		SELECT id, original_name, status, total_pages, converted_pages, pdf_path, thumbnail_path, manifest_url, created_at
		FROM documents
		WHERE id = ?
	`, id)

	doc := &models.PDFDocument{}
	var pdfPath, thumbnailPath, manifestURL sql.NullString
	if err := row.Scan(
		&doc.ID,
		&doc.Name,
		&doc.Status,
		&doc.TotalPages,
		&doc.ConvertedPages,
		&pdfPath,
		&thumbnailPath,
		&manifestURL,
		&doc.UploadDate,
	); err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	doc.FilePath = pdfPath.String
	doc.ThumbnailURL = thumbnailPath.String
	doc.ManifestURL = manifestURL.String
	doc.ImagePaths = ms.getImagePaths(doc.ID)
	return doc, nil
}

func (ms *MySQLStorage) GetAllDocuments() ([]*models.PDFDocument, error) {
	rows, err := ms.db.Query(`
		SELECT id, original_name, status, total_pages, converted_pages, pdf_path, thumbnail_path, manifest_url, created_at
		FROM documents
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []*models.PDFDocument
	for rows.Next() {
		doc := &models.PDFDocument{}
		var pdfPath, thumbnailPath, manifestURL sql.NullString
		if err := rows.Scan(
			&doc.ID,
			&doc.Name,
			&doc.Status,
			&doc.TotalPages,
			&doc.ConvertedPages,
			&pdfPath,
			&thumbnailPath,
			&manifestURL,
			&doc.UploadDate,
		); err != nil {
			return nil, err
		}
		doc.FilePath = pdfPath.String
		doc.ThumbnailURL = thumbnailPath.String
		doc.ManifestURL = manifestURL.String
		doc.ImagePaths = ms.getImagePaths(doc.ID)
		docs = append(docs, doc)
	}

	return docs, rows.Err()
}

func (ms *MySQLStorage) DeleteDocument(id string) error {
	doc, _ := ms.GetDocument(id)

	_, err := ms.db.Exec("DELETE FROM documents WHERE id = ?", id)
	if err != nil {
		return err
	}

	if doc != nil {
		_ = os.Remove(doc.FilePath)
	}
	_ = os.RemoveAll(filepath.Join(ms.basePath, "images", id))
	_ = os.Remove(filepath.Join(ms.basePath, "thumbnails", id+".jpg"))
	_ = os.Remove(filepath.Join(ms.basePath, "manifests", id+".json"))

	return nil
}

func (ms *MySQLStorage) SaveDocumentPDF(documentID string, data []byte, mediaType string) error {
	_, err := ms.db.Exec(`
		UPDATE documents
		SET pdf_blob = ?, pdf_media_type = ?, pdf_size = ?
		WHERE id = ?
	`, data, mediaType, len(data), documentID)
	return err
}

func (ms *MySQLStorage) SaveDocumentImage(image *models.DocumentImage) error {
	if image.CreatedAt.IsZero() {
		image.CreatedAt = time.Now()
	}

	_, err := ms.db.Exec(`
		INSERT INTO document_images (id, document_id, page_number, image_path, width, height, format, media_type, byte_size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			document_id = VALUES(document_id),
			page_number = VALUES(page_number),
			image_path = VALUES(image_path),
			width = VALUES(width),
			height = VALUES(height),
			format = VALUES(format),
			media_type = VALUES(media_type),
			byte_size = VALUES(byte_size)
	`, image.ID, image.DocumentID, image.PageNumber, nullString(image.ImagePath), image.Width, image.Height, image.Format, nullString(image.MediaType), image.ByteSize, image.CreatedAt)

	return err
}

func (ms *MySQLStorage) SaveDocumentImageData(imageID string, data []byte, mediaType string) error {
	_, err := ms.db.Exec(`
		UPDATE document_images
		SET image_blob = ?, media_type = ?, byte_size = ?
		WHERE id = ?
	`, data, mediaType, len(data), imageID)
	return err
}

func (ms *MySQLStorage) GetDocumentImage(id string) (*models.DocumentImage, error) {
	row := ms.db.QueryRow(`
		SELECT id, document_id, page_number, image_path, width, height, format, media_type, byte_size, created_at
		FROM document_images
		WHERE id = ?
	`, id)
	return scanDocumentImage(row)
}

func (ms *MySQLStorage) GetDocumentImageByPage(documentID string, page int) (*models.DocumentImage, error) {
	row := ms.db.QueryRow(`
		SELECT id, document_id, page_number, image_path, width, height, format, media_type, byte_size, created_at
		FROM document_images
		WHERE document_id = ? AND page_number = ?
	`, documentID, page)
	return scanDocumentImage(row)
}

func (ms *MySQLStorage) GetDocumentImages(documentID string) ([]*models.DocumentImage, error) {
	// Entrega las paginas del documento ordenadas para la galeria administrativa.
	rows, err := ms.db.Query(`
		SELECT id, document_id, page_number, image_path, width, height, format, media_type, byte_size, created_at
		FROM document_images
		WHERE document_id = ?
		ORDER BY page_number
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

func (ms *MySQLStorage) GetDocumentImageData(id string) (*models.BinaryAsset, error) {
	row := ms.db.QueryRow(`
		SELECT id, image_blob, media_type, byte_size
		FROM document_images
		WHERE id = ?
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

func (ms *MySQLStorage) upsertDocument(doc *models.PDFDocument) error {
	if doc.UploadDate.IsZero() {
		doc.UploadDate = time.Now()
	}

	_, err := ms.db.Exec(`
		INSERT INTO documents (id, original_name, status, total_pages, converted_pages, pdf_path, thumbnail_path, manifest_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			original_name = VALUES(original_name),
			status = VALUES(status),
			total_pages = VALUES(total_pages),
			converted_pages = VALUES(converted_pages),
			pdf_path = VALUES(pdf_path),
			thumbnail_path = VALUES(thumbnail_path),
			manifest_url = VALUES(manifest_url),
			updated_at = NOW()
	`, doc.ID, doc.Name, doc.Status, doc.TotalPages, doc.ConvertedPages, nullString(doc.FilePath), nullString(doc.ThumbnailURL), nullString(doc.ManifestURL), doc.UploadDate)

	return err
}

func (ms *MySQLStorage) getImagePaths(documentID string) []string {
	rows, err := ms.db.Query("SELECT image_path FROM document_images WHERE document_id = ? AND image_path IS NOT NULL ORDER BY page_number", documentID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

func scanDocumentImage(row *sql.Row) (*models.DocumentImage, error) {
	image := &models.DocumentImage{}
	var imagePath, mediaType sql.NullString
	var byteSize sql.NullInt64
	if err := row.Scan(
		&image.ID,
		&image.DocumentID,
		&image.PageNumber,
		&imagePath,
		&image.Width,
		&image.Height,
		&image.Format,
		&mediaType,
		&byteSize,
		&image.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("image not found: %w", err)
	}
	image.ImagePath = imagePath.String
	image.MediaType = mediaType.String
	image.ByteSize = byteSize.Int64
	return image, nil
}

func scanDocumentImageRows(rows *sql.Rows) (*models.DocumentImage, error) {
	image := &models.DocumentImage{}
	var imagePath, mediaType sql.NullString
	var byteSize sql.NullInt64
	if err := rows.Scan(
		&image.ID,
		&image.DocumentID,
		&image.PageNumber,
		&imagePath,
		&image.Width,
		&image.Height,
		&image.Format,
		&mediaType,
		&byteSize,
		&image.CreatedAt,
	); err != nil {
		return nil, err
	}
	image.ImagePath = imagePath.String
	image.MediaType = mediaType.String
	image.ByteSize = byteSize.Int64
	return image, nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: strings.TrimSpace(value) != ""}
}
