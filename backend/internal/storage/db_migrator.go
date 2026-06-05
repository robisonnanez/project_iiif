package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"iiif-pdf-server/internal/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MigrationRunResult struct {
	Engine        string   `json:"engine"`
	PendingBefore int      `json:"pending_before"`
	Applied       int      `json:"applied"`
	Skipped       int      `json:"skipped"`
	DurationMS    int64    `json:"duration_ms"`
	Message       string   `json:"message"`
	Errors        []string `json:"errors,omitempty"`
	AppliedFiles  []string `json:"applied_files,omitempty"`
}

type migrationFile struct {
	Version  string
	Name     string
	Path     string
	SQL      string
	Checksum string
}

func RunDBMigrations(cfg *config.Config, baseDir string) (MigrationRunResult, error) {
	start := time.Now()
	engine := strings.ToLower(strings.TrimSpace(cfg.Storage.Backend))
	if engine == "postgresql" {
		engine = "postgres"
	}
	if engine == "mongo" {
		engine = "mongodb"
	}
	result := MigrationRunResult{Engine: engine}
	if engine == "mongodb" {
		result, err := runMongoMigrations(cfg, start)
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if engine != "mysql" && engine != "postgres" {
		result.Message = "motor no soportado para migraciones"
		return result, errors.New(result.Message)
	}

	db, err := openMigrationDB(cfg, engine)
	if err != nil {
		result.Message = "no se pudo abrir conexion de migraciones"
		result.Errors = []string{err.Error()}
		return result, err
	}
	defer db.Close()

	if err := ensureSchemaMigrationsTable(db, engine); err != nil {
		result.Message = "no se pudo crear tabla schema_migrations"
		result.Errors = []string{err.Error()}
		return result, err
	}

	files, err := discoverMigrationFiles(baseDir, engine)
	if err != nil {
		result.Message = "no se pudieron cargar archivos de migracion"
		result.Errors = []string{err.Error()}
		return result, err
	}
	appliedMap, err := loadAppliedMigrations(db)
	if err != nil {
		result.Message = "no se pudo consultar schema_migrations"
		result.Errors = []string{err.Error()}
		return result, err
	}

	pending := make([]migrationFile, 0, len(files))
	for _, f := range files {
		if _, ok := appliedMap[f.Version]; ok {
			result.Skipped++
			continue
		}
		pending = append(pending, f)
	}
	result.PendingBefore = len(pending)

	tx, err := db.Begin()
	if err != nil {
		result.Message = "no se pudo iniciar transaccion de migraciones"
		result.Errors = []string{err.Error()}
		return result, err
	}
	defer tx.Rollback()

	for _, m := range pending {
		if _, err := tx.Exec(m.SQL); err != nil {
			result.Message = "error ejecutando migraciones"
			result.Errors = []string{fmt.Sprintf("%s: %v", m.Name, err)}
			result.DurationMS = time.Since(start).Milliseconds()
			return result, err
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES ($1,$2,$3,NOW())", m.Version, m.Name, m.Checksum); err != nil {
			// mysql placeholders
			if engine == "mysql" {
				if _, err2 := tx.Exec("INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?,?,?,NOW())", m.Version, m.Name, m.Checksum); err2 != nil {
					result.Message = "error registrando migracion aplicada"
					result.Errors = []string{fmt.Sprintf("%s: %v", m.Name, err2)}
					result.DurationMS = time.Since(start).Milliseconds()
					return result, err2
				}
			} else {
				result.Message = "error registrando migracion aplicada"
				result.Errors = []string{fmt.Sprintf("%s: %v", m.Name, err)}
				result.DurationMS = time.Since(start).Milliseconds()
				return result, err
			}
		}
		result.Applied++
		result.AppliedFiles = append(result.AppliedFiles, m.Name)
	}

	if err := tx.Commit(); err != nil {
		result.Message = "no se pudo confirmar migraciones"
		result.Errors = []string{err.Error()}
		result.DurationMS = time.Since(start).Milliseconds()
		return result, err
	}

	result.DurationMS = time.Since(start).Milliseconds()
	if result.Applied == 0 {
		result.Message = "sin migraciones pendientes"
	} else {
		result.Message = fmt.Sprintf("migraciones aplicadas: %d", result.Applied)
	}
	return result, nil
}

func openMigrationDB(cfg *config.Config, engine string) (*sql.DB, error) {
	if engine == "postgres" {
		pg := cfg.Database.Postgres
		sslmode := pg.SSLMode
		if sslmode == "" {
			sslmode = "disable"
		}
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", pg.Host, pg.Port, pg.User, pg.Password, pg.Database, sslmode)
		if strings.TrimSpace(pg.Schema) != "" {
			dsn += " search_path=" + pg.Schema
		}
		return sql.Open("pgx", dsn)
	}
	mysql := cfg.Database.MySQL
	charset := mysql.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	parseTime := "false"
	if mysql.ParseTime {
		parseTime = "true"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=%s&loc=Local", mysql.User, mysql.Password, mysql.Host, mysql.Port, mysql.Database, charset, parseTime)
	return sql.Open("mysql", dsn)
}

func ensureSchemaMigrationsTable(db *sql.DB, engine string) error {
	if engine == "postgres" {
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version VARCHAR(32) PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				checksum VARCHAR(64) NOT NULL,
				applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)
		`)
		return err
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(32) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			checksum VARCHAR(64) NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func discoverMigrationFiles(baseDir, engine string) ([]migrationFile, error) {
	dir := filepath.Join(baseDir, "migrations", engine)
	if _, err := os.Stat(dir); err != nil {
		legacy := filepath.Join(baseDir, "migrations")
		if engine == "mysql" {
			dir = legacy
		} else {
			return nil, fmt.Errorf("no existe carpeta de migraciones: %s", dir)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]migrationFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version := strings.SplitN(e.Name(), "_", 2)[0]
		if version == "" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		out = append(out, migrationFile{
			Version:  version,
			Name:     e.Name(),
			Path:     path,
			SQL:      string(b),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func loadAppliedMigrations(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := map[string]struct{}{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		res[v] = struct{}{}
	}
	return res, rows.Err()
}

type mongoMigration struct {
	Version  string
	Name     string
	Checksum string
	Apply    func(context mongoMigrationContext) error
}

type mongoMigrationContext struct {
	db *mongo.Database
}

func runMongoMigrations(cfg *config.Config, start time.Time) (MigrationRunResult, error) {
	result := MigrationRunResult{Engine: "mongodb"}

	storage, err := NewMongoStorage(cfg)
	if err != nil {
		result.Message = "no se pudo abrir conexion de migraciones"
		result.Errors = []string{err.Error()}
		return result, err
	}
	defer storage.client.Disconnect(contextBackground())

	ctx, cancel := mongoTimeout()
	defer cancel()

	collection := storage.db.Collection("schema_migrations")
	appliedMap, err := loadAppliedMongoMigrations(ctx, collection)
	if err != nil {
		result.Message = "no se pudo consultar schema_migrations"
		result.Errors = []string{err.Error()}
		return result, err
	}

	migrations := mongoMigrations()
	pending := make([]mongoMigration, 0, len(migrations))
	for _, migration := range migrations {
		if _, ok := appliedMap[migration.Version]; ok {
			result.Skipped++
			continue
		}
		pending = append(pending, migration)
	}
	result.PendingBefore = len(pending)

	for _, migration := range pending {
		if err := migration.Apply(mongoMigrationContext{db: storage.db}); err != nil {
			result.Message = "error ejecutando migraciones"
			result.Errors = []string{fmt.Sprintf("%s: %v", migration.Name, err)}
			result.DurationMS = time.Since(start).Milliseconds()
			return result, err
		}
		if _, err := collection.UpdateOne(ctx,
			bson.M{"version": migration.Version},
			bson.M{"$set": bson.M{
				"version":    migration.Version,
				"name":       migration.Name,
				"checksum":   migration.Checksum,
				"applied_at": time.Now(),
			}},
			options.Update().SetUpsert(true),
		); err != nil {
			result.Message = "error registrando migracion aplicada"
			result.Errors = []string{fmt.Sprintf("%s: %v", migration.Name, err)}
			result.DurationMS = time.Since(start).Milliseconds()
			return result, err
		}
		result.Applied++
		result.AppliedFiles = append(result.AppliedFiles, migration.Name)
	}

	result.DurationMS = time.Since(start).Milliseconds()
	if result.Applied == 0 {
		result.Message = "sin migraciones pendientes"
	} else {
		result.Message = fmt.Sprintf("migraciones aplicadas: %d", result.Applied)
	}
	return result, nil
}

func loadAppliedMongoMigrations(ctx context.Context, collection *mongo.Collection) (map[string]struct{}, error) {
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		if strings.Contains(err.Error(), "NamespaceNotFound") {
			return map[string]struct{}{}, nil
		}
		return nil, err
	}
	defer cursor.Close(ctx)

	res := map[string]struct{}{}
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		if version, ok := raw["version"].(string); ok && version != "" {
			res[version] = struct{}{}
		}
	}
	return res, cursor.Err()
}

func mongoMigrations() []mongoMigration {
	return []mongoMigration{
		{
			Version:  "001",
			Name:     "001_create_documents",
			Checksum: checksumString("mongodb:001:create_documents"),
			Apply: func(context mongoMigrationContext) error {
				return ensureMongoCollection(context.db, "documents")
			},
		},
		{
			Version:  "002",
			Name:     "002_create_document_images",
			Checksum: checksumString("mongodb:002:create_document_images"),
			Apply: func(context mongoMigrationContext) error {
				return ensureMongoCollection(context.db, "document_images")
			},
		},
		{
			Version:  "003",
			Name:     "003_create_gridfs_buckets_metadata",
			Checksum: checksumString("mongodb:003:create_gridfs_buckets_metadata"),
			Apply: func(context mongoMigrationContext) error {
				if err := ensureMongoCollection(context.db, "pdfs.files"); err != nil {
					return err
				}
				if err := ensureMongoCollection(context.db, "pdfs.chunks"); err != nil {
					return err
				}
				if err := ensureMongoCollection(context.db, "images.files"); err != nil {
					return err
				}
				return ensureMongoCollection(context.db, "images.chunks")
			},
		},
		{
			Version:  "004",
			Name:     "004_add_projects_multitenant_indexes",
			Checksum: checksumString("mongodb:004:add_projects_multitenant_indexes"),
			Apply: func(context mongoMigrationContext) error {
				if err := createMongoIndex(context.db.Collection("documents"), mongo.IndexModel{
					Keys:    bson.D{{Key: "id", Value: 1}},
					Options: options.Index().SetUnique(true).SetName("documents_id_unique"),
				}); err != nil {
					return err
				}
				if err := createMongoIndex(context.db.Collection("documents"), mongo.IndexModel{
					Keys:    bson.D{{Key: "project_key", Value: 1}, {Key: "tenant_key", Value: 1}, {Key: "created_at", Value: -1}},
					Options: options.Index().SetName("documents_scope_created_at"),
				}); err != nil {
					return err
				}
				if err := createMongoIndex(context.db.Collection("document_images"), mongo.IndexModel{
					Keys:    bson.D{{Key: "id", Value: 1}},
					Options: options.Index().SetUnique(true).SetName("document_images_id_unique"),
				}); err != nil {
					return err
				}
				return createMongoIndex(context.db.Collection("document_images"), mongo.IndexModel{
					Keys:    bson.D{{Key: "document_id", Value: 1}, {Key: "page_number", Value: 1}},
					Options: options.Index().SetUnique(true).SetName("document_images_document_page_unique"),
				})
			},
		},
		{
			Version:  "005",
			Name:     "005_add_migrated_flags_indexes",
			Checksum: checksumString("mongodb:005:add_migrated_flags_indexes"),
			Apply: func(context mongoMigrationContext) error {
				if err := createMongoIndex(context.db.Collection("document_images"), mongo.IndexModel{
					Keys:    bson.D{{Key: "project_key", Value: 1}, {Key: "tenant_key", Value: 1}, {Key: "document_id", Value: 1}, {Key: "page_number", Value: 1}},
					Options: options.Index().SetName("document_images_scope_document_page"),
				}); err != nil {
					return err
				}
				return createMongoIndex(context.db.Collection("documents"), mongo.IndexModel{
					Keys:    bson.D{{Key: "migrated_from_local", Value: 1}, {Key: "created_at", Value: -1}},
					Options: options.Index().SetName("documents_migrated_created_at"),
				})
			},
		},
	}
}

func ensureMongoCollection(db *mongo.Database, name string) error {
	ctx, cancel := mongoTimeout()
	defer cancel()
	collections, err := db.ListCollectionNames(ctx, bson.M{"name": name})
	if err != nil {
		return err
	}
	if len(collections) > 0 {
		return nil
	}
	return db.CreateCollection(ctx, name)
}

func createMongoIndex(collection *mongo.Collection, model mongo.IndexModel) error {
	ctx, cancel := mongoTimeout()
	defer cancel()
	_, err := collection.Indexes().CreateOne(ctx, model)
	return err
}

func checksumString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mongoTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}

func contextBackground() context.Context {
	return context.Background()
}
