package services

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

	"github.com/disintegration/imaging"
	"github.com/patrickmn/go-cache"
)

type IIIFService struct {
	config  *config.Config
	storage storage.Storage
	cache   *cache.Cache
}

func NewIIIFService(config *config.Config, storage storage.Storage) *IIIFService {
	var c *cache.Cache
	if config.IIIF.CacheEnabled {
		c = cache.New(cache.DefaultExpiration, cache.DefaultExpiration)
	}

	return &IIIFService{
		config:  config,
		storage: storage,
		cache:   c,
	}
}

type InvalidPageSelectionError struct {
	Selection string
	Reason    string
}

func (e *InvalidPageSelectionError) Error() string {
	return fmt.Sprintf("seleccion de paginas invalida %q: %s", e.Selection, e.Reason)
}

func (s *IIIFService) GetManifest(documentID, pageSelection string) (*models.IIIFManifestV2, error) {
	doc, err := s.storage.GetDocument(documentID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	if doc.Status != "completed" {
		return nil, fmt.Errorf("document not ready")
	}
	pages, normalizedSelection, err := manifestPages(pageSelection, doc.TotalPages)
	if err != nil {
		return nil, err
	}
	cacheKey := "manifest_v2_" + documentID + "_" + normalizedSelection
	if s.cache != nil {
		if cached, found := s.cache.Get(cacheKey); found {
			return cached.(*models.IIIFManifestV2), nil
		}
	}

	manifestID := fmt.Sprintf("%s/api/iiif/%s/manifest", s.config.IIIF.BaseURL, documentID)
	if normalizedSelection != "all" {
		manifestID += "?pages=" + url.QueryEscape(normalizedSelection)
	}

	manifest := &models.IIIFManifestV2{
		Context: "http://iiif.io/api/presentation/2/context.json",
		ID:      manifestID,
		Type:    "sc:Manifest",
		Label:   doc.Name,
		Metadata: []models.IIIFMetadataV2{
			{Label: "Pages", Value: strconv.Itoa(doc.TotalPages)},
		},
		Sequences: []models.IIIFSequenceV2{
			{
				ID:          manifestID + "/sequence/normal",
				Type:        "sc:Sequence",
				Label:       "Current page order",
				ViewingHint: "paged",
				Canvases:    make([]models.IIIFCanvasV2, 0, len(pages)),
			},
		},
	}
	if isHTTPURL(doc.ThumbnailURL) {
		manifest.Thumbnail = &models.IIIFResourceV2{ID: doc.ThumbnailURL, Type: "dctypes:Image", Format: "image/jpeg"}
	}

	for _, page := range pages {
		canvas := s.createCanvasV2(documentID, page, doc.Name)
		manifest.Sequences[0].Canvases = append(manifest.Sequences[0].Canvases, canvas)
	}
	manifest.Structures = s.createRangesV2(documentID, doc.Outline, pages, doc.TotalPages)
	if len(manifest.Structures) == 0 {
		manifest.Structures = nil
	}

	// Guardar en caché
	if s.cache != nil {
		s.cache.Set(cacheKey, manifest, cache.DefaultExpiration)
	}

	return manifest, nil
}

// GetManifestV3 preserves the previous Presentation API 3 representation for
// clients that already consumed it. New integrations should use GetManifest.
func (s *IIIFService) GetManifestV3(documentID, pageSelection string) (*models.IIIFManifest, error) {
	doc, err := s.storage.GetDocument(documentID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}
	if doc.Status != "completed" {
		return nil, fmt.Errorf("document not ready")
	}
	pages, normalizedSelection, err := manifestPages(pageSelection, doc.TotalPages)
	if err != nil {
		return nil, err
	}
	cacheKey := "manifest_v3_" + documentID + "_" + normalizedSelection
	if s.cache != nil {
		if cached, found := s.cache.Get(cacheKey); found {
			return cached.(*models.IIIFManifest), nil
		}
	}

	manifestID := fmt.Sprintf("%s/api/iiif/v3/%s/manifest", s.config.IIIF.BaseURL, documentID)
	if normalizedSelection != "all" {
		manifestID += "?pages=" + url.QueryEscape(normalizedSelection)
	}
	manifest := &models.IIIFManifest{
		Context: []string{"http://iiif.io/api/presentation/3/context.json"},
		ID:      manifestID,
		Type:    "Manifest",
		Label:   map[string][]string{"es": {doc.Name}},
		Items:   make([]models.IIIFCanvas, 0, len(pages)),
	}
	for _, page := range pages {
		manifest.Items = append(manifest.Items, s.createCanvasV3(documentID, page, doc.Name))
	}
	if s.cache != nil {
		s.cache.Set(cacheKey, manifest, cache.DefaultExpiration)
	}
	return manifest, nil
}

func manifestPages(selection string, totalPages int) ([]int, string, error) {
	selection = strings.TrimSpace(selection)
	if selection == "" || strings.EqualFold(selection, "all") {
		pages := make([]int, totalPages)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages, "all", nil
	}

	selected := make(map[int]struct{})
	for _, rawPart := range strings.Split(selection, ",") {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			return nil, "", &InvalidPageSelectionError{Selection: selection, Reason: "segmento vacio"}
		}
		bounds := strings.Split(part, "-")
		if len(bounds) > 2 {
			return nil, "", &InvalidPageSelectionError{Selection: selection, Reason: "rango no valido"}
		}
		start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil {
			return nil, "", &InvalidPageSelectionError{Selection: selection, Reason: "se esperaba un numero"}
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, "", &InvalidPageSelectionError{Selection: selection, Reason: "fin de rango no valido"}
			}
		}
		if start < 1 || end < 1 || start > totalPages || end > totalPages {
			return nil, "", &InvalidPageSelectionError{Selection: selection, Reason: fmt.Sprintf("las paginas deben estar entre 1 y %d", totalPages)}
		}
		if start > end {
			return nil, "", &InvalidPageSelectionError{Selection: selection, Reason: "el inicio del rango supera el final"}
		}
		for page := start; page <= end; page++ {
			selected[page] = struct{}{}
		}
	}

	pages := make([]int, 0, len(selected))
	for page := range selected {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	normalized := make([]string, len(pages))
	for i, page := range pages {
		normalized[i] = strconv.Itoa(page)
	}
	return pages, strings.Join(normalized, ","), nil
}

func (s *IIIFService) createCanvasV2(documentID string, page int, title string) models.IIIFCanvasV2 {
	canvasID := s.canvasIDV2(documentID, page)
	imageIdentifier := s.imageIdentifier(documentID, page, title)
	imageServiceID := fmt.Sprintf("%s/iiif/2/%s", s.config.IIIF.BaseURL, imageIdentifier)
	width, height := s.getImageDimensions(documentID, page)
	format := "image/jpeg"
	if image, err := s.storage.GetDocumentImageByPage(documentID, page); err == nil {
		format = mediaTypeForIIIF(image.Format, image.MediaType)
	}
	return models.IIIFCanvasV2{
		ID:     canvasID,
		Type:   "sc:Canvas",
		Label:  fmt.Sprintf("Page %d", page),
		Height: height,
		Width:  width,
		Images: []models.IIIFAnnotationV2{
			{
				ID:         canvasID + "/annotations/painting",
				Type:       "oa:Annotation",
				Motivation: "sc:painting",
				Resource: models.IIIFResourceV2{
					ID:     imageServiceID + "/full/full/0/default.jpg",
					Type:   "dctypes:Image",
					Format: format,
					Height: height,
					Width:  width,
					Service: &models.IIIFServiceV2{
						Context:  "http://iiif.io/api/image/2/context.json",
						ID:       imageServiceID,
						Profile:  "http://iiif.io/api/image/2/level1.json",
						Protocol: "http://iiif.io/api/image",
					},
				},
				On: canvasID,
			},
		},
	}
}

func (s *IIIFService) createCanvasV3(documentID string, page int, title string) models.IIIFCanvas {
	canvasID := fmt.Sprintf("%s/api/iiif/%s/canvas/%d", s.config.IIIF.BaseURL, documentID, page)

	imageIdentifier := s.imageIdentifier(documentID, page, title)

	imageID := fmt.Sprintf("%s/iiif/%s/%s", s.config.IIIF.BaseURL, s.config.IIIF.APIVersion, imageIdentifier)

	// Obtener dimensiones de la imagen
	width, height := s.getImageDimensions(documentID, page)

	return models.IIIFCanvas{
		ID:   canvasID,
		Type: "Canvas",
		Label: map[string][]string{
			"es": {fmt.Sprintf("Página %d", page)},
		},
		Height: height,
		Width:  width,
		Items: []models.IIIFAnnotationPage{
			{
				ID:   canvasID + "/page",
				Type: "AnnotationPage",
				Items: []models.IIIFAnnotation{
					{
						ID:         canvasID + "/annotation",
						Type:       "Annotation",
						Motivation: "painting",
						Body: models.IIIFBody{
							ID:     imageID + "/full/max/0/default.jpg",
							Type:   "Image",
							Format: "image/jpeg",
							Service: []models.IIIFService{
								{
									Context: "http://iiif.io/api/image/3/context.json",
									ID:      imageID,
									Type:    "ImageService3",
									Profile: "level2",
								},
							},
						},
						Target: canvasID,
					},
				},
			},
		},
	}
}

func (s *IIIFService) canvasIDV2(documentID string, page int) string {
	return fmt.Sprintf("%s/api/iiif/%s/canvases/%s_%04d", s.config.IIIF.BaseURL, documentID, documentID, page)
}

type rangeCandidate struct {
	index    int
	item     models.PDFOutlineItem
	endPage  int
	children []*rangeCandidate
}

func (s *IIIFService) createRangesV2(documentID string, outline []models.PDFOutlineItem, pages []int, totalPages int) []models.IIIFRangeV2 {
	if len(outline) == 0 || totalPages < 1 {
		return nil
	}
	selected := make(map[int]struct{}, len(pages))
	for _, page := range pages {
		selected[page] = struct{}{}
	}

	valid := make([]rangeCandidate, 0, len(outline))
	for index, item := range outline {
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" || item.PageNumber < 1 || item.PageNumber > totalPages {
			continue
		}
		if item.Level < 1 {
			item.Level = 1
		}
		valid = append(valid, rangeCandidate{index: index + 1, item: item, endPage: totalPages})
	}
	for i := range valid {
		for j := i + 1; j < len(valid); j++ {
			if valid[j].item.Level <= valid[i].item.Level {
				valid[i].endPage = valid[j].item.PageNumber - 1
				break
			}
		}
		if valid[i].endPage < valid[i].item.PageNumber {
			valid[i].endPage = valid[i].item.PageNumber
		}
	}

	roots := make([]*rangeCandidate, 0)
	stack := make([]*rangeCandidate, 0)
	for i := range valid {
		node := &valid[i]
		for len(stack) > 0 && stack[len(stack)-1].item.Level >= node.item.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, node)
		} else {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, node)
		}
		stack = append(stack, node)
	}

	result := make([]models.IIIFRangeV2, 0, len(roots))
	for _, root := range roots {
		if item, ok := s.materializeRangeV2(documentID, root, "", selected); ok {
			result = append(result, item)
		}
	}
	return result
}

func (s *IIIFService) materializeRangeV2(documentID string, node *rangeCandidate, parentID string, selected map[int]struct{}) (models.IIIFRangeV2, bool) {
	id := fmt.Sprintf("%s/api/iiif/%s/ranges/LOG_%04d", s.config.IIIF.BaseURL, documentID, node.index)
	rangeValue := models.IIIFRangeV2{ID: id, Type: "sc:Range", Label: node.item.Title, Within: parentID}
	for page := node.item.PageNumber; page <= node.endPage; page++ {
		if _, ok := selected[page]; ok {
			rangeValue.Canvases = append(rangeValue.Canvases, s.canvasIDV2(documentID, page))
		}
	}
	for _, child := range node.children {
		if childRange, ok := s.materializeRangeV2(documentID, child, id, selected); ok {
			rangeValue.Ranges = append(rangeValue.Ranges, childRange)
		}
	}
	return rangeValue, len(rangeValue.Canvases) > 0 || len(rangeValue.Ranges) > 0
}

func mediaTypeForIIIF(format, stored string) string {
	if strings.HasPrefix(stored, "image/") {
		return stored
	}
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func (s *IIIFService) GetImageInfo(documentID string, page int) (*models.IIIFImageInfo, error) {
	cacheKey := fmt.Sprintf("info_%s_%d", documentID, page)
	if s.cache != nil {
		if cached, found := s.cache.Get(cacheKey); found {
			return cached.(*models.IIIFImageInfo), nil
		}
	}

	image, err := s.resolveImage(documentID, page)
	if err != nil {
		return nil, fmt.Errorf("image not found: %w", err)
	}

	width, height := image.Width, image.Height
	if width == 0 || height == 0 {
		width, height = s.getImageDimensions(documentID, page)
	}
	if width == 0 || height == 0 {
		return nil, fmt.Errorf("image not found")
	}

	imageIdentifier := image.ID
	if imageIdentifier == "" {
		imageIdentifier = s.imageIdentifier(documentID, page, documentID)
	}

	imageID := fmt.Sprintf("%s/iiif/%s/%s", s.config.IIIF.BaseURL, s.config.IIIF.APIVersion, imageIdentifier)

	info := &models.IIIFImageInfo{
		Context:  "http://iiif.io/api/image/3/context.json",
		ID:       imageID,
		Type:     "ImageService3",
		Protocol: "http://iiif.io/api/image",
		Profile:  "level2",
		Width:    width,
		Height:   height,
		Sizes: []models.IIIFSize{
			{Width: width / 4, Height: height / 4},
			{Width: width / 2, Height: height / 2},
			{Width: width, Height: height},
		},
		Tiles: []models.IIIFTile{
			{
				Width:        s.config.IIIF.TileSize,
				Height:       s.config.IIIF.TileSize,
				ScaleFactors: s.config.IIIF.ScaleFactors,
			},
		},
	}

	if s.cache != nil {
		s.cache.Set(cacheKey, info, cache.DefaultExpiration)
	}

	return info, nil
}

func (s *IIIFService) GetImageInfoV2(documentID string, page int) (*models.IIIFImageInfoV2, error) {
	image, err := s.resolveImage(documentID, page)
	if err != nil {
		return nil, fmt.Errorf("image not found: %w", err)
	}
	width, height := image.Width, image.Height
	if width == 0 || height == 0 {
		width, height = s.getImageDimensions(documentID, page)
	}
	if width == 0 || height == 0 {
		return nil, fmt.Errorf("image not found")
	}
	identifier := image.ID
	if identifier == "" {
		identifier = s.imageIdentifier(documentID, page, documentID)
	}
	imageID := fmt.Sprintf("%s/iiif/2/%s", s.config.IIIF.BaseURL, identifier)
	return &models.IIIFImageInfoV2{
		Context:  "http://iiif.io/api/image/2/context.json",
		ID:       imageID,
		Protocol: "http://iiif.io/api/image",
		Profile:  "http://iiif.io/api/image/2/level1.json",
		Width:    width,
		Height:   height,
		Sizes: []models.IIIFSize{
			{Width: width / 4, Height: height / 4},
			{Width: width / 2, Height: height / 2},
			{Width: width, Height: height},
		},
		Tiles: []models.IIIFTile{{Width: s.config.IIIF.TileSize, Height: s.config.IIIF.TileSize, ScaleFactors: s.config.IIIF.ScaleFactors}},
	}, nil
}

func (s *IIIFService) GetImage(documentID string, page int, size, rotation, quality, format string) ([]byte, string, error) {
	return s.GetImageWithRegion(documentID, page, "full", size, rotation, quality, format)
}

func (s *IIIFService) GetImageWithRegion(documentID string, page int, region, size, rotation, quality, format string) ([]byte, string, error) {
	docImage, err := s.resolveImage(documentID, page)
	if err != nil {
		return nil, "", err
	}
	img, err := s.openImage(docImage)
	if err != nil {
		return nil, "", fmt.Errorf("error opening image: %w", err)
	}

	// Procesar región
	img = s.processRegion(img, region)

	// Procesar tamaño
	img = s.processSize(img, size)

	// Procesar rotación
	if rotation != "0" {
		if rot, err := strconv.Atoi(rotation); err == nil {
			img = imaging.Rotate(img, float64(rot), image.Black)
		}
	}

	// Crear archivo temporal para la respuesta
	var encoded bytes.Buffer

	// Codificar imagen según formato
	contentType := "image/jpeg"
	switch format {
	case "jpg", "jpeg":
		if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		contentType = "image/jpeg"
	case "png":
		// Para PNG necesitarías importar image/png
		if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		contentType = "image/jpeg" // Fallback a JPEG
	default:
		if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		contentType = "image/jpeg"
	}

	// Leer archivo
	return encoded.Bytes(), contentType, nil
}

func (s *IIIFService) processRegion(img image.Image, region string) image.Image {
	if region == "full" {
		return img
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Parsear región: x,y,w,h
	if strings.Contains(region, ",") {
		parts := strings.Split(region, ",")
		if len(parts) == 4 {
			x, err1 := strconv.Atoi(parts[0])
			y, err2 := strconv.Atoi(parts[1])
			w, err3 := strconv.Atoi(parts[2])
			h, err4 := strconv.Atoi(parts[3])

			if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
				// Validar límites
				if x < 0 {
					x = 0
				}
				if y < 0 {
					y = 0
				}
				if x+w > width {
					w = width - x
				}
				if y+h > height {
					h = height - y
				}

				if w > 0 && h > 0 {
					rect := image.Rect(x, y, x+w, y+h)
					return imaging.Crop(img, rect)
				}
			}
		}
	}

	return img
}

func (s *IIIFService) processSize(img image.Image, size string) image.Image {
	if size == "full" || size == "max" {
		return img
	}

	// Parsear tamaño (ej: "800,", "400,600", "!800,600")
	if strings.HasSuffix(size, ",") {
		// Solo ancho especificado
		if width, err := strconv.Atoi(size[:len(size)-1]); err == nil {
			return imaging.Resize(img, width, 0, imaging.Lanczos)
		}
	} else if strings.Contains(size, ",") {
		parts := strings.Split(size, ",")
		if len(parts) == 2 {
			width, err1 := strconv.Atoi(parts[0])
			height, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				if strings.HasPrefix(size, "!") {
					// Fit dentro de las dimensiones
					return imaging.Fit(img, width, height, imaging.Lanczos)
				} else {
					// Redimensionar exacto
					return imaging.Resize(img, width, height, imaging.Lanczos)
				}
			}
		}
	}

	return img
}

func (s *IIIFService) getImageDimensions(documentID string, page int) (int, int) {
	image, err := s.resolveImage(documentID, page)
	if err == nil {
		if image.Width > 0 && image.Height > 0 {
			return image.Width, image.Height
		}
		img, err := s.openImage(image)
		if err != nil {
			return 0, 0
		}
		bounds := img.Bounds()
		return bounds.Dx(), bounds.Dy()
	}

	// Determinar la ruta de la imagen basada en el documentID
	var imagePath string
	if strings.Contains(documentID, ".") && !strings.Contains(documentID, ".pdf") {
		// Verificar si documentID contiene &
		if strings.Contains(documentID, "&") {
			// Obtener el texto antes del &
			parts := strings.Split(documentID, "&")
			if len(parts) == 3 {
				directory_1 := parts[0]
				directory_2 := parts[1]
				nameImage := parts[2]
				// baseID := strings.TrimSuffix(parts[2], filepath.Ext(parts[2]))
				imagePath = filepath.Join(s.config.Storage.ImagesPath, directory_1, directory_2, nameImage)
			} else if len(parts) == 2 {
				directory_1 := parts[0]
				nameImage := parts[1]
				imagePath = filepath.Join(s.config.Storage.ImagesPath, directory_1, nameImage)
			}
		} else {
			// Para imágenes individuales
			imagePath = filepath.Join(s.config.Storage.ImagesPath, documentID)
		}
		// imagePath = filepath.Join(s.config.Storage.ImagesPath, baseID, fmt.Sprintf("page_%d.jpeg", page))
	} else {
		// Para PDFs o IDs sin extensión
		cleanID := strings.TrimSuffix(documentID, ".pdf")
		imagePath = filepath.Join(s.config.Storage.ImagesPath, cleanID, fmt.Sprintf("page_%d.jpeg", page))
	}

	img, err := imaging.Open(imagePath)
	if err != nil {
		return 0, 0
	}

	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy()
}

func (s *IIIFService) resolveImage(identifier string, page int) (*models.DocumentImage, error) {
	image, err := s.storage.GetDocumentImage(identifier)
	if err == nil {
		return image, nil
	}

	image, err = s.storage.GetDocumentImageByPage(identifier, page)
	if err == nil {
		return image, nil
	}

	imagePath := s.legacyImagePath(identifier, page)
	if _, statErr := os.Stat(imagePath); statErr != nil {
		return nil, fmt.Errorf("image not found: %w", err)
	}

	width, height := imageDimensions(imagePath)
	return &models.DocumentImage{
		ID:         identifier,
		DocumentID: identifier,
		PageNumber: page,
		ImagePath:  imagePath,
		Width:      width,
		Height:     height,
		Format:     strings.TrimPrefix(filepath.Ext(imagePath), "."),
	}, nil
}

func (s *IIIFService) legacyImagePath(documentID string, page int) string {
	if strings.Contains(documentID, ".") && !strings.Contains(documentID, ".pdf") {
		if strings.Contains(documentID, "&") {
			parts := strings.Split(documentID, "&")
			if len(parts) == 3 {
				return filepath.Join(s.config.Storage.ImagesPath, parts[0], parts[1], parts[2])
			}
			if len(parts) == 2 {
				return filepath.Join(s.config.Storage.ImagesPath, parts[0], parts[1])
			}
		}
		return filepath.Join(s.config.Storage.ImagesPath, documentID)
	}

	cleanID := strings.TrimSuffix(documentID, ".pdf")
	return filepath.Join(s.config.Storage.ImagesPath, cleanID, fmt.Sprintf("page_%d.jpg", page))
}

func (s *IIIFService) imageIdentifier(documentID string, page int, title string) string {
	if image, err := s.storage.GetDocumentImageByPage(documentID, page); err == nil && image.ID != "" {
		return image.ID
	}

	if strings.HasSuffix(strings.ToLower(title), ".pdf") {
		baseID := strings.TrimSuffix(documentID, ".pdf")
		return fmt.Sprintf("%s.pdf_page_%d", baseID, page)
	}
	if page == 1 {
		return documentID
	}

	baseID := strings.TrimSuffix(documentID, filepath.Ext(documentID))
	ext := filepath.Ext(documentID)
	return fmt.Sprintf("%s_page_%d%s", baseID, page, ext)
}

func imageDimensions(imagePath string) (int, int) {
	img, err := imaging.Open(imagePath)
	if err != nil {
		return 0, 0
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy()
}

func (s *IIIFService) openImage(docImage *models.DocumentImage) (image.Image, error) {
	asset, err := s.storage.GetDocumentImageData(docImage.ID)
	if err == nil && len(asset.Data) > 0 {
		return imaging.Decode(bytes.NewReader(asset.Data))
	}
	if docImage.ImagePath == "" {
		return nil, fmt.Errorf("image blob not found")
	}
	if _, err := os.Stat(docImage.ImagePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("image not found: %s", docImage.ImagePath)
	}
	return imaging.Open(docImage.ImagePath)
}
