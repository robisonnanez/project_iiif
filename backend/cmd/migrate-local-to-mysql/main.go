package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

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
}

func main() {
	log.SetFlags(0)

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("ERROR no se pudo cargar config.yaml: %v", err)
	}
	if strings.ToLower(cfg.Storage.Backend) != "mysql" {
		log.Fatalf("ERROR storage.backend debe ser mysql para migrar a BLOB. valor actual: %s", cfg.Storage.Backend)
	}

	src := readSourceConfig(cfg)
	mysqlStore, err := storage.NewMySQLStorage(cfg)
	if err != nil {
		log.Fatalf("ERROR no se pudo conectar a MySQL: %v", err)
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

	for _, item := range docs {
		if err := migrateDocument(item, cfg, mysqlStore, &s); err != nil {
			s.ErroredDocs++
			log.Printf("ERROR documento %s: %v", item.Doc.ID, err)
		}
	}

	log.Printf("INFO resumen")
	log.Printf("INFO documentos total=%d migrados=%d omitidos=%d errores=%d", s.TotalDocuments, s.MigratedDocs, s.SkippedDocs, s.ErroredDocs)
	log.Printf("INFO pdf total=%d migrados=%d omitidos=%d errores=%d", s.TotalPDFs, s.MigratedPDFs, s.SkippedPDFs, s.ErroredPDFs)
	log.Printf("INFO imagenes total=%d migradas=%d omitidas=%d errores=%d", s.TotalImages, s.MigratedImgs, s.SkippedImgs, s.ErroredImages)
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
		})
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
		})
	}
	return out, nil
}

func migrateDocument(item sourceDocument, cfg *config.Config, mysqlStore storage.Storage, s *stats) error {
	doc := item.Doc
	if err := mysqlStore.SaveDocument(doc); err != nil {
		return fmt.Errorf("no se pudo guardar metadata documento: %w", err)
	}
	s.MigratedDocs++

	s.TotalPDFs++
	if err := migratePDFBlob(doc, item.PDFBytes, mysqlStore, s); err != nil {
		s.ErroredPDFs++
		log.Printf("ERROR pdf documento %s: %v", doc.ID, err)
	}

	if len(item.Images) == 0 {
		log.Printf("WARN documento %s no tiene imagenes para migrar", doc.ID)
		return nil
	}
	for _, img := range item.Images {
		s.TotalImages++
		if strings.TrimSpace(img.ProjectKey) == "" {
			img.ProjectKey = doc.ProjectKey
		}
		if strings.TrimSpace(img.TenantKey) == "" {
			img.TenantKey = doc.TenantKey
		}
		data := item.ImageData[img.ID]
		if err := migrateImageBlob(img, data, mysqlStore); err != nil {
			if err == errSkipImageBlob {
				s.SkippedImgs++
				continue
			}
			s.ErroredImages++
			log.Printf("ERROR imagen %s (doc=%s page=%d): %v", img.ID, doc.ID, img.PageNumber, err)
			continue
		}
		s.MigratedImgs++
	}
	return nil
}

func migratePDFBlob(doc *models.PDFDocument, pdfData []byte, mysqlStore storage.Storage, s *stats) error {
	if ms, ok := mysqlStore.(*storage.MySQLStorage); ok {
		exists, err := ms.HasDocumentPDFBlob(doc.ID)
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
	if err := mysqlStore.SaveDocumentPDF(doc.ID, pdfData, "application/pdf"); err != nil {
		return fmt.Errorf("fallo al guardar pdf_blob: %w", err)
	}
	s.MigratedPDFs++
	return nil
}

func migrateImageBlob(img *models.DocumentImage, data []byte, mysqlStore storage.Storage) error {
	if ms, ok := mysqlStore.(*storage.MySQLStorage); ok {
		exists, err := ms.HasImageBlob(img.ID)
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
	if err := mysqlStore.SaveDocumentImage(img); err != nil {
		return fmt.Errorf("fallo al guardar metadata imagen: %w", err)
	}
	if err := mysqlStore.SaveDocumentImageData(img.ID, data, img.MediaType); err != nil {
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
