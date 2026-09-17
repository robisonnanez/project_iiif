package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"iiif-pdf-server/internal/models"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrAnnotationNotFound = errors.New("anotación no encontrada")
	ErrAnnotationChanged  = errors.New("la anotación fue modificada")
)

// AnnotationStorage is intentionally separate from Storage: file and Mongo
// backends do not pretend to provide relational constraints they cannot honor.
type AnnotationStorage interface {
	CreateAnnotation(context.Context, *models.DocumentAnnotation) (*models.DocumentAnnotation, bool, error)
	GetAnnotationByClientReference(context.Context, string, string) (*models.DocumentAnnotation, error)
	GetAnnotation(context.Context, string, string, string, bool) (*models.DocumentAnnotation, error)
	GetAnnotationsByIDs(context.Context, []string, string, string, string) ([]*models.DocumentAnnotation, error)
	UpdateAnnotation(context.Context, *models.DocumentAnnotation, int64) (*models.DocumentAnnotation, error)
	SoftDeleteAnnotation(context.Context, string, string, string) error
}

func ResolveAnnotationStorage(store Storage) AnnotationStorage {
	if repository, ok := store.(AnnotationStorage); ok {
		return repository
	}
	if s3Store, ok := store.(*S3Storage); ok {
		if repository, ok := s3Store.Storage.(AnnotationStorage); ok {
			return repository
		}
	}
	return nil
}

func (ms *MySQLStorage) CreateAnnotation(ctx context.Context, annotation *models.DocumentAnnotation) (*models.DocumentAnnotation, bool, error) {
	return createAnnotation(ctx, ms.db, "mysql", annotation)
}
func (ps *PostgresStorage) CreateAnnotation(ctx context.Context, annotation *models.DocumentAnnotation) (*models.DocumentAnnotation, bool, error) {
	return createAnnotation(ctx, ps.db, "postgres", annotation)
}
func (ms *MySQLStorage) GetAnnotationByClientReference(ctx context.Context, tenant, clientReference string) (*models.DocumentAnnotation, error) {
	return getAnnotationByClientReference(ctx, ms.db, "mysql", tenant, clientReference)
}
func (ps *PostgresStorage) GetAnnotationByClientReference(ctx context.Context, tenant, clientReference string) (*models.DocumentAnnotation, error) {
	return getAnnotationByClientReference(ctx, ps.db, "postgres", tenant, clientReference)
}
func (ms *MySQLStorage) GetAnnotation(ctx context.Context, id, tenant, project string, includeDeleted bool) (*models.DocumentAnnotation, error) {
	return getAnnotation(ctx, ms.db, "mysql", id, tenant, project, includeDeleted)
}
func (ps *PostgresStorage) GetAnnotation(ctx context.Context, id, tenant, project string, includeDeleted bool) (*models.DocumentAnnotation, error) {
	return getAnnotation(ctx, ps.db, "postgres", id, tenant, project, includeDeleted)
}
func (ms *MySQLStorage) GetAnnotationsByIDs(ctx context.Context, ids []string, tenant, project, pageID string) ([]*models.DocumentAnnotation, error) {
	return getAnnotationsByIDs(ctx, ms.db, "mysql", ids, tenant, project, pageID)
}
func (ps *PostgresStorage) GetAnnotationsByIDs(ctx context.Context, ids []string, tenant, project, pageID string) ([]*models.DocumentAnnotation, error) {
	return getAnnotationsByIDs(ctx, ps.db, "postgres", ids, tenant, project, pageID)
}
func (ms *MySQLStorage) UpdateAnnotation(ctx context.Context, annotation *models.DocumentAnnotation, version int64) (*models.DocumentAnnotation, error) {
	return updateAnnotation(ctx, ms.db, "mysql", annotation, version)
}
func (ps *PostgresStorage) UpdateAnnotation(ctx context.Context, annotation *models.DocumentAnnotation, version int64) (*models.DocumentAnnotation, error) {
	return updateAnnotation(ctx, ps.db, "postgres", annotation, version)
}
func (ms *MySQLStorage) SoftDeleteAnnotation(ctx context.Context, id, tenant, project string) error {
	return softDeleteAnnotation(ctx, ms.db, "mysql", id, tenant, project)
}
func (ps *PostgresStorage) SoftDeleteAnnotation(ctx context.Context, id, tenant, project string) error {
	return softDeleteAnnotation(ctx, ps.db, "postgres", id, tenant, project)
}

const annotationColumns = `id, project_key, tenant_id, document_id, page_id, page_number,
	ocr_generation_id, layer_sha256, client_reference_id, request_fingerprint,
	selected_text, note, selector, color, version, created_at, updated_at, deleted_at`

func createAnnotation(ctx context.Context, db *sql.DB, dialect string, a *models.DocumentAnnotation) (*models.DocumentAnnotation, bool, error) {
	selector, err := json.Marshal(a.Selector)
	if err != nil {
		return nil, false, err
	}
	query := `INSERT INTO document_annotations
		(id, project_key, tenant_id, document_id, page_id, page_number, ocr_generation_id,
		 layer_sha256, client_reference_id, request_fingerprint, selected_text, note, selector, color, version)
		VALUES (` + placeholders(dialect, 15) + `)`
	_, err = db.ExecContext(ctx, query, a.ID, a.ProjectKey, a.TenantID, a.DocumentID, a.PageID,
		a.PageNumber, a.OCRGenerationID, a.LayerSHA256, a.ClientReferenceID, a.RequestFingerprint,
		a.SelectedText, nullableString(a.Note), string(selector), nullableString(a.Color), a.Version)
	if err != nil {
		if !isUniqueViolation(err) {
			return nil, false, err
		}
		existing, findErr := getAnnotationByClientReference(ctx, db, dialect, a.TenantID, a.ClientReferenceID)
		return existing, true, findErr
	}
	created, err := getAnnotation(ctx, db, dialect, a.ID, a.TenantID, a.ProjectKey, false)
	return created, false, err
}

func getAnnotation(ctx context.Context, db *sql.DB, dialect, id, tenant, project string, includeDeleted bool) (*models.DocumentAnnotation, error) {
	query := `SELECT ` + annotationColumns + ` FROM document_annotations WHERE id = ` + placeholder(dialect, 1) +
		` AND tenant_id = ` + placeholder(dialect, 2) + ` AND project_key = ` + placeholder(dialect, 3)
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	return scanAnnotation(db.QueryRowContext(ctx, query, id, tenant, project))
}

func getAnnotationByClientReference(ctx context.Context, db *sql.DB, dialect, tenant, clientReference string) (*models.DocumentAnnotation, error) {
	query := `SELECT ` + annotationColumns + ` FROM document_annotations WHERE tenant_id = ` + placeholder(dialect, 1) +
		` AND client_reference_id = ` + placeholder(dialect, 2)
	return scanAnnotation(db.QueryRowContext(ctx, query, tenant, clientReference))
}

func getAnnotationsByIDs(ctx context.Context, db *sql.DB, dialect string, ids []string, tenant, project, pageID string) ([]*models.DocumentAnnotation, error) {
	if len(ids) == 0 {
		return []*models.DocumentAnnotation{}, nil
	}
	args := make([]any, 0, len(ids)+3)
	args = append(args, tenant, project)
	parts := make([]string, len(ids))
	for index, id := range ids {
		parts[index] = placeholder(dialect, index+3)
		args = append(args, id)
	}
	query := `SELECT ` + annotationColumns + ` FROM document_annotations WHERE tenant_id = ` + placeholder(dialect, 1) +
		` AND project_key = ` + placeholder(dialect, 2) + ` AND deleted_at IS NULL AND id IN (` + strings.Join(parts, ",") + `)`
	if pageID != "" {
		query += ` AND page_id = ` + placeholder(dialect, len(args)+1)
		args = append(args, pageID)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[string]*models.DocumentAnnotation, len(ids))
	for rows.Next() {
		annotation, scanErr := scanAnnotation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		byID[annotation.ID] = annotation
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ordered := make([]*models.DocumentAnnotation, 0, len(byID))
	seen := map[string]struct{}{}
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if annotation := byID[id]; annotation != nil {
			ordered = append(ordered, annotation)
		}
	}
	return ordered, nil
}

func updateAnnotation(ctx context.Context, db *sql.DB, dialect string, a *models.DocumentAnnotation, expectedVersion int64) (*models.DocumentAnnotation, error) {
	selector, err := json.Marshal(a.Selector)
	if err != nil {
		return nil, err
	}
	now := "CURRENT_TIMESTAMP(6)"
	if dialect == "postgres" {
		now = "NOW()"
	}
	query := `UPDATE document_annotations SET selected_text = ` + placeholder(dialect, 1) +
		`, note = ` + placeholder(dialect, 2) + `, selector = ` + placeholder(dialect, 3) +
		`, color = ` + placeholder(dialect, 4) + `, ocr_generation_id = ` + placeholder(dialect, 5) +
		`, layer_sha256 = ` + placeholder(dialect, 6) + `, version = version + 1, updated_at = ` + now +
		` WHERE id = ` + placeholder(dialect, 7) + ` AND tenant_id = ` + placeholder(dialect, 8) +
		` AND project_key = ` + placeholder(dialect, 9) + ` AND version = ` + placeholder(dialect, 10) + ` AND deleted_at IS NULL`
	result, err := db.ExecContext(ctx, query, a.SelectedText, nullableString(a.Note), string(selector), nullableString(a.Color),
		a.OCRGenerationID, a.LayerSHA256, a.ID, a.TenantID, a.ProjectKey, expectedVersion)
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed == 0 {
		if _, findErr := getAnnotation(ctx, db, dialect, a.ID, a.TenantID, a.ProjectKey, false); findErr != nil {
			return nil, findErr
		}
		return nil, ErrAnnotationChanged
	}
	return getAnnotation(ctx, db, dialect, a.ID, a.TenantID, a.ProjectKey, false)
}

func softDeleteAnnotation(ctx context.Context, db *sql.DB, dialect, id, tenant, project string) error {
	now := "CURRENT_TIMESTAMP(6)"
	if dialect == "postgres" {
		now = "NOW()"
	}
	query := `UPDATE document_annotations SET deleted_at = ` + now + `, updated_at = ` + now +
		`, version = version + 1 WHERE id = ` + placeholder(dialect, 1) + ` AND tenant_id = ` + placeholder(dialect, 2) +
		` AND project_key = ` + placeholder(dialect, 3) + ` AND deleted_at IS NULL`
	result, err := db.ExecContext(ctx, query, id, tenant, project)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed > 0 {
		return err
	}
	// A repeated delete in the same scope is idempotent. Cross-scope IDs remain hidden.
	_, err = getAnnotation(ctx, db, dialect, id, tenant, project, true)
	return err
}

type rowScanner interface{ Scan(...any) error }

func scanAnnotation(row rowScanner) (*models.DocumentAnnotation, error) {
	annotation := &models.DocumentAnnotation{}
	var note, color sql.NullString
	var selector []byte
	var deleted sql.NullTime
	err := row.Scan(&annotation.ID, &annotation.ProjectKey, &annotation.TenantID, &annotation.DocumentID,
		&annotation.PageID, &annotation.PageNumber, &annotation.OCRGenerationID, &annotation.LayerSHA256,
		&annotation.ClientReferenceID, &annotation.RequestFingerprint, &annotation.SelectedText, &note,
		&selector, &color, &annotation.Version, &annotation.CreatedAt, &annotation.UpdatedAt, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAnnotationNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(selector, &annotation.Selector); err != nil {
		return nil, fmt.Errorf("selector persistido inválido: %w", err)
	}
	if note.Valid {
		annotation.Note = &note.String
	}
	if color.Valid {
		annotation.Color = &color.String
	}
	if deleted.Valid {
		annotation.DeletedAt = &deleted.Time
	}
	return annotation, nil
}

func placeholders(dialect string, count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = placeholder(dialect, index+1)
	}
	return strings.Join(values, ",")
}

func placeholder(dialect string, index int) string {
	if dialect == "postgres" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func isUniqueViolation(err error) bool {
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) {
		return mysqlError.Number == 1062
	}
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
