package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

	"github.com/gen2brain/go-fitz"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

type stats struct {
	TotalDocuments int
	MigratedDocs   int
	SkippedDocs    int
	ErroredDocs    int

	TotalPDFs     int
	MigratedPDFs  int
	SkippedPDFs   int
	ErroredPDFs   int
	TotalImages   int
	MigratedImgs  int
	SkippedImgs   int
	ErroredImages int

	DocsFromJSON      int
	DocsFromPDFLayout int
	ImagesFoundOnDisk int
	ImagesFallback    int
}

type sourceConfig struct {
	Type      string
	LocalPath string
	SSH       struct {
		Host       string
		Port       int
		User       string
		Path       string
		PrivateKey string
	}
}

type sourceDocument struct {
	Doc       *models.PDFDocument
	PDFPath   string
	PDFBytes  []byte
	Images    []*models.DocumentImage
	ImageData map[string][]byte
	FromJSON  bool
	SourceKey string
}

func newDatabaseStore(cfg *config.Config, engine string) (storage.Storage, error) {
	switch engine {
	case "mysql":
		return storage.NewMySQLStorage(cfg)
	case "postgres":
		return storage.NewPostgresStorage(cfg)
	case "mongodb":
		return storage.NewMongoStorage(cfg)
	default:
		return nil, fmt.Errorf("motor no soportado para migracion: %s", engine)
	}
}

func main() {
	log.SetFlags(0)

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("ERROR no se pudo cargar config.yaml: %v", err)
	}
	engine := strings.ToLower(strings.TrimSpace(cfg.Storage.Backend))
	if engine == "postgresql" {
		engine = "postgres"
	}
	if engine == "mongo" {
		engine = "mongodb"
	}
	if engine != "mysql" && engine != "postgres" && engine != "mongodb" {
		log.Fatalf("ERROR storage.backend debe ser mysql, postgres o mongodb para migrar a BLOB. valor actual: %s", cfg.Storage.Backend)
	}

	src := readSourceConfig(cfg)
	dbStore, err := newDatabaseStore(cfg, engine)
	if err != nil {
		log.Fatalf("ERROR no se pudo conectar a %s: %v", engine, err)
	}

	var docs []sourceDocument
	switch src.Type {
	case "ssh":
		docs, err = discoverSSHDocuments(cfg, src)
	default:
		docs, err = discoverLocalDocuments(cfg, src.LocalPath)
	}
	if err != nil {
		log.Fatalf("ERROR discovery: %v", err)
	}
	if len(docs) == 0 {
		log.Printf("INFO no se encontraron documentos para migrar")
		return
	}

	var s stats
	s.TotalDocuments = len(docs)
	log.Printf("INFO documentos detectados: %d", len(docs))

	for index, item := range docs {
		log.Printf("PROGRESS_DOC %s|%s|%d|%d|running|iniciado", item.Doc.ID, item.Doc.Name, 0, len(item.Images))
		if item.FromJSON {
			s.DocsFromJSON++
		} else {
			s.DocsFromPDFLayout++
		}
		s.ImagesFoundOnDisk += len(item.Images)
		if err := migrateDocument(item, dbStore, &s); err != nil {
			s.ErroredDocs++
			log.Printf("ERROR documento %s: %v", item.Doc.ID, err)
			log.Printf("PROGRESS_DOC %s|%s|%d|%d|error|%s", item.Doc.ID, item.Doc.Name, 0, len(item.Images), sanitizeProgressMessage(err.Error()))
			log.Printf("METRIC current_doc=%d", index+1)
			continue
		}
		log.Printf("PROGRESS_DOC %s|%s|%d|%d|ok|migracion exitosa", item.Doc.ID, item.Doc.Name, len(item.Images), len(item.Images))
		log.Printf("METRIC current_doc=%d", index+1)
	}

	log.Printf("INFO resumen")
	log.Printf("INFO documentos total=%d migrados=%d omitidos=%d errores=%d", s.TotalDocuments, s.MigratedDocs, s.SkippedDocs, s.ErroredDocs)
	log.Printf("INFO pdf total=%d migrados=%d omitidos=%d errores=%d", s.TotalPDFs, s.MigratedPDFs, s.SkippedPDFs, s.ErroredPDFs)
	log.Printf("INFO imagenes total=%d migradas=%d omitidas=%d errores=%d", s.TotalImages, s.MigratedImgs, s.SkippedImgs, s.ErroredImages)
	log.Printf("METRIC docs_discovered=%d", s.TotalDocuments)
	log.Printf("METRIC docs_from_json=%d", s.DocsFromJSON)
	log.Printf("METRIC docs_from_pdf_layout=%d", s.DocsFromPDFLayout)
	log.Printf("METRIC images_found_on_disk=%d", s.ImagesFoundOnDisk)
	log.Printf("METRIC images_migrated=%d", s.MigratedImgs)
	log.Printf("METRIC images_fallback_converted=%d", s.ImagesFallback)
	log.Printf("METRIC images_skipped_existing_blob=%d", s.SkippedImgs)
	log.Printf("METRIC docs_total=%d", s.TotalDocuments)
}

func readSourceConfig(cfg *config.Config) sourceConfig {
	srcType := strings.ToLower(strings.TrimSpace(os.Getenv("MIGRATION_SOURCE_TYPE")))
	if srcType == "" {
		srcType = "local"
	}
	localPath := strings.TrimSpace(os.Getenv("MIGRATION_SOURCE_LOCAL_PATH"))
	if localPath == "" {
		localPath = cfg.Storage.DataPath
	}

	src := sourceConfig{Type: srcType, LocalPath: localPath}
	src.SSH.Host = strings.TrimSpace(os.Getenv("MIGRATION_SOURCE_SSH_HOST"))
	src.SSH.User = strings.TrimSpace(os.Getenv("MIGRATION_SOURCE_SSH_USER"))
	src.SSH.Path = strings.TrimSpace(os.Getenv("MIGRATION_SOURCE_SSH_PATH"))
	src.SSH.PrivateKey = os.Getenv("MIGRATION_SOURCE_SSH_PRIVATE_KEY")
	port, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("MIGRATION_SOURCE_SSH_PORT")))
	if port <= 0 {
		port = 22
	}
	src.SSH.Port = port
	return src
}

func discoverLocalDocuments(cfg *config.Config, basePath string) ([]sourceDocument, error) {
	localCfg := *cfg
	localCfg.Storage.DataPath = basePath
	localStore := storage.NewFileStorageFromConfig(&localCfg)

	ids, err := discoverDocumentIDs(basePath)
	if err != nil {
		return nil, err
	}
	out := make([]sourceDocument, 0, len(ids))
	for _, id := range ids {
		doc, err := localStore.GetDocument(id)
		if err != nil {
			log.Printf("WARN documento local %s no legible: %v", id, err)
			continue
		}
		if strings.TrimSpace(doc.ProjectKey) == "" {
			doc.ProjectKey = cfg.Projects.DefaultProject
			if doc.ProjectKey == "" {
				doc.ProjectKey = "default"
			}
		}
		images, _ := localStore.GetDocumentImages(id)
		imageData := map[string][]byte{}
		for _, img := range images {
			path := resolveImagePath(doc, img)
			if path == "" {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil || len(data) == 0 {
				continue
			}
			imageData[img.ID] = data
		}
		pdfPath := resolvePDFPath(doc, cfg, basePath)
		pdfBytes := []byte(nil)
		if pdfPath != "" {
			pdfBytes, _ = os.ReadFile(pdfPath)
		}
		out = append(out, sourceDocument{
			Doc:       doc,
			PDFPath:   pdfPath,
			PDFBytes:  pdfBytes,
			Images:    images,
			ImageData: imageData,
			FromJSON:  true,
			SourceKey: normalizeSourcePath(pdfPath),
		})
	}

	layoutDocs, err := discoverPDFLayoutDocuments(cfg, basePath)
	if err != nil {
		log.Printf("WARN no se pudo procesar estructura pdf+images: %v", err)
	}
	existing := map[string]struct{}{}
	for _, item := range out {
		existing[item.Doc.ID] = struct{}{}
	}
	for _, item := range layoutDocs {
		if _, ok := existing[item.Doc.ID]; ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func discoverSSHDocuments(cfg *config.Config, src sourceConfig) ([]sourceDocument, error) {
	client, err := newSSHClient(src)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	ids, err := sshDiscoverDocumentIDs(client, src.SSH.Path)
	if err != nil {
		return nil, err
	}
	out := make([]sourceDocument, 0, len(ids))
	for _, id := range ids {
		doc, docPath, err := sshReadDocument(client, src.SSH.Path, id)
		if err != nil {
			log.Printf("WARN documento remoto %s no legible: %v", id, err)
			continue
		}
		if strings.TrimSpace(doc.ProjectKey) == "" {
			doc.ProjectKey = inferProjectTenantFromDocPath(cfg, docPath).ProjectKey
			if doc.ProjectKey == "" {
				doc.ProjectKey = "default"
			}
		}
		if strings.TrimSpace(doc.TenantKey) == "" {
			doc.TenantKey = inferProjectTenantFromDocPath(cfg, docPath).TenantKey
		}

		images, imageData := sshReadImagesForDoc(client, src.SSH.Path, doc)
		pdfPath := resolvePDFPath(doc, cfg, src.SSH.Path)
		pdfBytes := []byte(nil)
		if pdfPath != "" {
			pdfBytes, _ = sshReadFile(client, pdfPath)
		}

		out = append(out, sourceDocument{
			Doc:       doc,
			PDFPath:   pdfPath,
			PDFBytes:  pdfBytes,
			Images:    images,
			ImageData: imageData,
			SourceKey: normalizeSourcePath(pdfPath),
		})
	}
	return out, nil
}

func migrateDocument(item sourceDocument, dbStore storage.Storage, s *stats) error {
	doc := item.Doc
	originalImageData := item.ImageData
	if strings.TrimSpace(item.SourceKey) == "" {
		item.SourceKey = normalizeSourcePath(doc.FilePath)
	}
	doc.ID = stableDocumentID(doc.ProjectKey, doc.TenantKey, item.SourceKey, doc.Name)
	doc.MigratedFromLocal = true
	for _, img := range item.Images {
		oldID := img.ID
		img.ID = stableImageID(doc.ID, img.PageNumber)
		img.DocumentID = doc.ID
		img.MigratedFromLocal = true
		if data, ok := originalImageData[oldID]; ok {
			if item.ImageData == nil {
				item.ImageData = map[string][]byte{}
			}
			item.ImageData[img.ID] = data
		}
	}

	if err := dbStore.SaveDocument(doc); err != nil {
		return fmt.Errorf("no se pudo guardar metadata documento: %w", err)
	}
	s.MigratedDocs++

	s.TotalPDFs++
	if err := migratePDFBlob(doc, item.PDFBytes, dbStore, s); err != nil {
		s.ErroredPDFs++
		log.Printf("ERROR pdf documento %s: %v", doc.ID, err)
	}

	if len(item.Images) == 0 {
		if err := fillFallbackImagesFromPDF(&item); err != nil {
			log.Printf("WARN documento %s sin imagenes y no se pudo convertir fallback: %v", doc.ID, err)
		} else {
			s.ImagesFallback += len(item.Images)
		}
	}
	for _, img := range item.Images {
		s.TotalImages++
		oldID := img.ID
		img.ID = stableImageID(doc.ID, img.PageNumber)
		img.DocumentID = doc.ID
		img.MigratedFromLocal = true
		if strings.TrimSpace(img.ProjectKey) == "" {
			img.ProjectKey = doc.ProjectKey
		}
		if strings.TrimSpace(img.TenantKey) == "" {
			img.TenantKey = doc.TenantKey
		}
		if data, ok := originalImageData[oldID]; ok {
			if item.ImageData == nil {
				item.ImageData = map[string][]byte{}
			}
			item.ImageData[img.ID] = data
		}
		data := item.ImageData[img.ID]
		if err := migrateImageBlob(img, data, dbStore); err != nil {
			if err == errSkipImageBlob {
				s.SkippedImgs++
				log.Printf("PROGRESS_IMG %s|%d|%d", doc.ID, s.MigratedImgs+s.SkippedImgs+s.ErroredImages, s.TotalImages)
				continue
			}
			s.ErroredImages++
			log.Printf("ERROR imagen %s (doc=%s page=%d): %v", img.ID, doc.ID, img.PageNumber, err)
			log.Printf("PROGRESS_IMG %s|%d|%d", doc.ID, s.MigratedImgs+s.SkippedImgs+s.ErroredImages, s.TotalImages)
			continue
		}
		s.MigratedImgs++
		log.Printf("PROGRESS_IMG %s|%d|%d", doc.ID, s.MigratedImgs+s.SkippedImgs+s.ErroredImages, s.TotalImages)
	}
	return nil
}

func migratePDFBlob(doc *models.PDFDocument, pdfData []byte, dbStore storage.Storage, s *stats) error {
	if checker, ok := dbStore.(storage.DocumentPDFBlobChecker); ok {
		exists, err := checker.HasDocumentPDFBlob(doc.ID)
		if err != nil {
			log.Printf("WARN no se pudo verificar pdf_blob para %s: %v", doc.ID, err)
		} else if exists {
			s.SkippedPDFs++
			return nil
		}
	}
	if len(pdfData) == 0 {
		s.SkippedPDFs++
		return fmt.Errorf("pdf sin bytes para migrar")
	}
	if err := dbStore.SaveDocumentPDF(doc.ID, pdfData, "application/pdf"); err != nil {
		return fmt.Errorf("fallo al guardar pdf_blob: %w", err)
	}
	s.MigratedPDFs++
	return nil
}

func migrateImageBlob(img *models.DocumentImage, data []byte, dbStore storage.Storage) error {
	if checker, ok := dbStore.(storage.ImageBlobChecker); ok {
		exists, err := checker.HasImageBlob(img.ID)
		if err != nil {
			log.Printf("WARN no se pudo verificar image_blob para %s: %v", img.ID, err)
		} else if exists {
			return errSkipImageBlob
		}
	}
	if len(data) == 0 {
		return fmt.Errorf("imagen sin bytes para migrar")
	}
	if img.Width == 0 || img.Height == 0 {
		img.Width, img.Height = imageDimensionsBytes(data)
	}
	if strings.TrimSpace(img.Format) == "" {
		img.Format = "jpg"
	}
	if strings.TrimSpace(img.MediaType) == "" {
		img.MediaType = mediaTypeFromFormat(img.Format)
	}
	img.ByteSize = int64(len(data))
	if err := dbStore.SaveDocumentImage(img); err != nil {
		return fmt.Errorf("fallo al guardar metadata imagen: %w", err)
	}
	if err := dbStore.SaveDocumentImageData(img.ID, data, img.MediaType); err != nil {
		return fmt.Errorf("fallo al guardar image_blob: %w", err)
	}
	return nil
}

var errSkipImageBlob = fmt.Errorf("image_blob ya existe")

func discoverDocumentIDs(basePath string) ([]string, error) {
	ids := map[string]struct{}{}
	err := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) != ".json" || filepath.Base(filepath.Dir(path)) != "documents" {
			return nil
		}
		id := strings.TrimSuffix(d.Name(), ".json")
		if strings.TrimSpace(id) != "" {
			ids[id] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func discoverPDFLayoutDocuments(cfg *config.Config, basePath string) ([]sourceDocument, error) {
	pdfPaths := []string{}
	err := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".pdf") {
			pdfPaths = append(pdfPaths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	docs := []sourceDocument{}
	for _, pdfPath := range pdfPaths {
		pdfData, err := os.ReadFile(pdfPath)
		if err != nil || len(pdfData) == 0 {
			continue
		}
		baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		tenantFromPath := inferTenantFromPath(pdfPath, "pdfs")
		scopeProject := strings.TrimSpace(os.Getenv("MIGRATION_SCOPE_PROJECT"))
		scopeTenant := strings.TrimSpace(os.Getenv("MIGRATION_SCOPE_TENANT"))
		project := scopeProject
		if project == "" {
			project = cfg.Projects.DefaultProject
			if project == "" {
				project = "default"
			}
		}
		tenant := tenantFromPath
		if scopeTenant != "" {
			if tenant != "" && !strings.EqualFold(scopeTenant, tenant) {
				log.Printf("WARN tenant inferido '%s' difiere de tenant modal '%s' para %s", tenant, scopeTenant, baseName)
			}
			tenant = scopeTenant
		}
		doc := &models.PDFDocument{
			ID:             baseName,
			Name:           filepath.Base(pdfPath),
			ProjectKey:     project,
			TenantKey:      tenant,
			UploadDate:     time.Now(),
			Status:         "completed",
			TotalPages:     0,
			ConvertedPages: 0,
		}
		images, imageData := readImagesForPDFLayout(cfg, basePath, tenantFromPath, baseName, doc.ID)
		doc.TotalPages = len(images)
		doc.ConvertedPages = len(images)
		if doc.TotalPages == 0 {
			doc.Status = "processing"
		}
		docs = append(docs, sourceDocument{
			Doc:       doc,
			PDFPath:   pdfPath,
			PDFBytes:  pdfData,
			Images:    images,
			ImageData: imageData,
			FromJSON:  false,
			SourceKey: normalizeSourcePath(pdfPath),
		})
	}
	return docs, nil
}

func readImagesForPDFLayout(cfg *config.Config, basePath, tenant, pdfDir, documentID string) ([]*models.DocumentImage, map[string][]byte) {
	imageData := map[string][]byte{}
	images := []*models.DocumentImage{}

	candidates := []string{
		filepath.Join(cfg.Storage.DataPath, "images", tenant, pdfDir),
		filepath.Join(filepath.Dir(basePath), "images", tenant, pdfDir),
		filepath.Join(basePath, pdfDir),
	}
	var imageRoot string
	for _, c := range candidates {
		if dirExists(c) {
			imageRoot = c
			break
		}
	}
	if imageRoot == "" {
		return images, imageData
	}

	files := []string{}
	_ = filepath.WalkDir(imageRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	for i, path := range files {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		page := inferPageNumber(path, i+1)
		id := fmt.Sprintf("%s_page_%d", documentID, page)
		w, h := imageDimensionsBytes(data)
		format := normalizeExt(filepath.Ext(path))
		images = append(images, &models.DocumentImage{
			ID:         id,
			DocumentID: documentID,
			PageNumber: page,
			Width:      w,
			Height:     h,
			Format:     format,
			MediaType:  mediaTypeFromFormat(format),
			ByteSize:   int64(len(data)),
			CreatedAt:  time.Now(),
		})
		imageData[id] = data
	}
	sort.Slice(images, func(i, j int) bool { return images[i].PageNumber < images[j].PageNumber })
	return images, imageData
}

func resolvePDFPath(doc *models.PDFDocument, cfg *config.Config, root string) string {
	candidates := []string{}
	if strings.TrimSpace(doc.FilePath) != "" {
		candidates = append(candidates, doc.FilePath)
	}
	ext := strings.ToLower(filepath.Ext(doc.Name))
	if ext == "" {
		ext = ".pdf"
	}
	scopeBase := buildScopeBase(root, doc.ProjectKey, doc.TenantKey, cfg.Projects.Enabled)
	candidates = append(candidates,
		filepath.Join(scopeBase, "pdfs", doc.ID+ext),
		filepath.Join(root, "pdfs", doc.ID+ext),
	)
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "/") {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		} else {
			return p
		}
	}
	return ""
}

func resolveImagePath(doc *models.PDFDocument, img *models.DocumentImage) string {
	if strings.TrimSpace(img.ImagePath) != "" {
		if st, err := os.Stat(img.ImagePath); err == nil && !st.IsDir() {
			return img.ImagePath
		}
	}
	if img.PageNumber > 0 && img.PageNumber <= len(doc.ImagePaths) {
		p := doc.ImagePaths[img.PageNumber-1]
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func buildScopeBase(root, project, tenant string, projectsEnabled bool) string {
	if !projectsEnabled || strings.TrimSpace(project) == "" {
		return root
	}
	if strings.TrimSpace(tenant) != "" {
		return filepath.Join(root, "projects", project, "tenants", tenant)
	}
	return filepath.Join(root, "projects", project)
}

func imageDimensionsBytes(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func mediaTypeFromFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func newSSHClient(src sourceConfig) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(src.SSH.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("llave privada invalida: %w", err)
	}
	clientConfig := &ssh.ClientConfig{
		User:            src.SSH.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	addr := fmt.Sprintf("%s:%d", src.SSH.Host, src.SSH.Port)
	return ssh.Dial("tcp", addr, clientConfig)
}

func sshDiscoverDocumentIDs(client *ssh.Client, basePath string) ([]string, error) {
	cmd := fmt.Sprintf("find %s -type f -path '*/documents/*.json' 2>/dev/null", shq(basePath))
	out, err := sshRun(client, cmd)
	if err != nil {
		return nil, err
	}
	lines := splitLines(out)
	ids := map[string]struct{}{}
	for _, path := range lines {
		name := filepath.Base(path)
		if strings.HasSuffix(name, ".json") {
			id := strings.TrimSuffix(name, ".json")
			if id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Strings(list)
	return list, nil
}

func sshReadDocument(client *ssh.Client, basePath, docID string) (*models.PDFDocument, string, error) {
	findCmd := fmt.Sprintf("find %s -type f -path '*/documents/%s.json' | head -1", shq(basePath), docID)
	docPathRaw, err := sshRun(client, findCmd)
	if err != nil {
		return nil, "", err
	}
	docPath := strings.TrimSpace(docPathRaw)
	if docPath == "" {
		return nil, "", fmt.Errorf("document json no encontrado para %s", docID)
	}
	content, err := sshReadFile(client, docPath)
	if err != nil {
		return nil, "", err
	}
	var doc models.PDFDocument
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(doc.ID) == "" {
		doc.ID = docID
	}
	return &doc, docPath, nil
}

func sshReadImagesForDoc(client *ssh.Client, basePath string, doc *models.PDFDocument) ([]*models.DocumentImage, map[string][]byte) {
	findCmd := fmt.Sprintf("find %s -type f -path '*/images/%s/*.json' 2>/dev/null", shq(basePath), doc.ID)
	pathsRaw, _ := sshRun(client, findCmd)
	paths := splitLines(pathsRaw)
	images := []*models.DocumentImage{}
	imageData := map[string][]byte{}
	for _, p := range paths {
		content, err := sshReadFile(client, p)
		if err != nil {
			continue
		}
		var img models.DocumentImage
		if err := json.Unmarshal(content, &img); err != nil {
			continue
		}
		if strings.TrimSpace(img.ID) == "" {
			continue
		}
		dataPath := img.ImagePath
		if strings.TrimSpace(dataPath) == "" && img.PageNumber > 0 && img.PageNumber <= len(doc.ImagePaths) {
			dataPath = doc.ImagePaths[img.PageNumber-1]
		}
		if strings.TrimSpace(dataPath) != "" {
			data, _ := sshReadFile(client, dataPath)
			if len(data) > 0 {
				imageData[img.ID] = data
			}
		}
		images = append(images, &img)
	}

	if len(images) == 0 {
		for idx, p := range doc.ImagePaths {
			id := fmt.Sprintf("%s_page_%d", doc.ID, idx+1)
			data, _ := sshReadFile(client, p)
			if len(data) == 0 {
				continue
			}
			img := &models.DocumentImage{
				ID:         id,
				DocumentID: doc.ID,
				ProjectKey: doc.ProjectKey,
				TenantKey:  doc.TenantKey,
				PageNumber: idx + 1,
				Format:     normalizeExt(filepath.Ext(p)),
			}
			images = append(images, img)
			imageData[id] = data
		}
	}
	sort.Slice(images, func(i, j int) bool { return images[i].PageNumber < images[j].PageNumber })
	return images, imageData
}

func sshReadFile(client *ssh.Client, path string) ([]byte, error) {
	cmd := fmt.Sprintf("cat %s", shq(path))
	out, err := sshRunRaw(client, cmd)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func sshRun(client *ssh.Client, cmd string) (string, error) {
	out, err := sshRunRaw(client, cmd)
	return string(out), err
}

func sshRunRaw(client *ssh.Client, cmd string) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.Output(cmd)
}

func inferProjectTenantFromDocPath(cfg *config.Config, docPath string) models.Scope {
	scope := models.Scope{ProjectKey: cfg.Projects.DefaultProject}
	if scope.ProjectKey == "" {
		scope.ProjectKey = "default"
	}
	parts := strings.Split(filepath.ToSlash(docPath), "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "projects" && i+1 < len(parts) {
			scope.ProjectKey = parts[i+1]
		}
		if parts[i] == "tenants" && i+1 < len(parts) {
			scope.TenantKey = parts[i+1]
		}
	}
	return scope
}

func splitLines(text string) []string {
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func shq(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

func normalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if ext == "jpeg" {
		return "jpg"
	}
	return ext
}

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func inferTenantFromPath(path, marker string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == marker {
			return parts[i+1]
		}
	}
	return ""
}

func inferPageNumber(path string, fallback int) int {
	name := strings.ToLower(filepath.Base(path))
	re := regexp.MustCompile(`(?:page|p)[_\-\s]?(\d+)`)
	m := re.FindStringSubmatch(name)
	if len(m) == 2 {
		if value, err := strconv.Atoi(m[1]); err == nil && value > 0 {
			return value
		}
	}
	reNum := regexp.MustCompile(`(\d+)`)
	m = reNum.FindStringSubmatch(name)
	if len(m) == 2 {
		if value, err := strconv.Atoi(m[1]); err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

func fillFallbackImagesFromPDF(item *sourceDocument) error {
	if len(item.PDFBytes) == 0 {
		return fmt.Errorf("pdf sin bytes")
	}
	tmpFile, err := os.CreateTemp("", "iiif-migrate-*.pdf")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(item.PDFBytes); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	tmpFile.Close()
	defer os.Remove(tmpPath)

	doc, err := fitz.New(tmpPath)
	if err != nil {
		return err
	}
	defer doc.Close()

	total := doc.NumPage()
	images := make([]*models.DocumentImage, 0, total)
	imageData := map[string][]byte{}
	for i := 0; i < total; i++ {
		img, err := doc.Image(i)
		if err != nil {
			continue
		}
		buf := new(bytes.Buffer)
		if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 90}); err != nil {
			continue
		}
		page := i + 1
		id := fmt.Sprintf("%s_page_%d", item.Doc.ID, page)
		w, h := imageDimensionsBytes(buf.Bytes())
		images = append(images, &models.DocumentImage{
			ID:         id,
			DocumentID: item.Doc.ID,
			ProjectKey: item.Doc.ProjectKey,
			TenantKey:  item.Doc.TenantKey,
			PageNumber: page,
			Width:      w,
			Height:     h,
			Format:     "jpg",
			MediaType:  "image/jpeg",
			ByteSize:   int64(buf.Len()),
			CreatedAt:  time.Now(),
		})
		imageData[id] = buf.Bytes()
	}
	item.Images = images
	item.ImageData = imageData
	item.Doc.TotalPages = maxInt(item.Doc.TotalPages, len(images))
	item.Doc.ConvertedPages = len(images)
	if len(images) > 0 {
		item.Doc.Status = "completed"
	}
	return nil
}

func maxInt(a, b int) int {
	if b > a {
		return b
	}
	return a
}

var migrationNamespaceUUID = uuid.MustParse("d26ef5bc-8d99-4f87-a0c2-4c6052a4e2cc")

func stableDocumentID(project, tenant, sourceKey, name string) string {
	key := fmt.Sprintf("%s|%s|%s|%s",
		strings.ToLower(strings.TrimSpace(project)),
		strings.ToLower(strings.TrimSpace(tenant)),
		normalizeSourcePath(sourceKey),
		strings.ToLower(strings.TrimSpace(name)),
	)
	return uuid.NewSHA1(migrationNamespaceUUID, []byte(key)).String()
}

func stableImageID(documentID string, page int) string {
	key := fmt.Sprintf("%s|%d", documentID, page)
	return uuid.NewSHA1(migrationNamespaceUUID, []byte(key)).String()
}

func normalizeSourcePath(path string) string {
	return strings.ToLower(strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(path)), "//", "/"))
}

func sanitizeProgressMessage(message string) string {
	return strings.TrimSpace(strings.ReplaceAll(message, "|", "/"))
}
