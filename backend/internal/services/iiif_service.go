package services

import (
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
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

func (s *IIIFService) GetManifest(documentID string) (*models.IIIFManifest, error) {
	// Verificar caché
	if s.cache != nil {
		if cached, found := s.cache.Get("manifest_" + documentID); found {
			return cached.(*models.IIIFManifest), nil
		}
	}

	doc, err := s.storage.GetDocument(documentID)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	if doc.Status != "completed" {
		return nil, fmt.Errorf("document not ready")
	}

	manifest := &models.IIIFManifest{
		Context: []string{
			"http://iiif.io/api/presentation/3/context.json",
		},
		ID:   fmt.Sprintf("%s/api/iiif/%s/manifest", s.config.IIIF.BaseURL, documentID),
		Type: "Manifest",
		Label: map[string][]string{
			"es": {doc.Name},
		},
		Items: make([]models.IIIFCanvas, 0, doc.TotalPages),
	}

	// Crear canvas para cada página
	for i := 1; i <= doc.TotalPages; i++ {
		canvas := s.createCanvas(documentID, i, doc.Name)
		manifest.Items = append(manifest.Items, canvas)
	}

	// Guardar en caché
	if s.cache != nil {
		s.cache.Set("manifest_"+documentID, manifest, cache.DefaultExpiration)
	}

	return manifest, nil
}

func (s *IIIFService) createCanvas(documentID string, page int, title string) models.IIIFCanvas {
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

func (s *IIIFService) GetImage(documentID string, page int, size, rotation, quality, format string) ([]byte, string, error) {
	return s.GetImageWithRegion(documentID, page, "full", size, rotation, quality, format)
}

func (s *IIIFService) GetImageWithRegion(documentID string, page int, region, size, rotation, quality, format string) ([]byte, string, error) {
	docImage, err := s.resolveImage(documentID, page)
	if err != nil {
		return nil, "", err
	}
	imagePath := docImage.ImagePath

	// Verificar si el archivo existe
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("image not found: %s", imagePath)
	}

	// Abrir imagen
	img, err := imaging.Open(imagePath)
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
	tempFile, err := os.CreateTemp("", "iiif_*.jpg")
	if err != nil {
		return nil, "", err
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Codificar imagen según formato
	contentType := "image/jpeg"
	switch format {
	case "jpg", "jpeg":
		if err := jpeg.Encode(tempFile, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		contentType = "image/jpeg"
	case "png":
		// Para PNG necesitarías importar image/png
		if err := jpeg.Encode(tempFile, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		contentType = "image/jpeg" // Fallback a JPEG
	default:
		if err := jpeg.Encode(tempFile, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		contentType = "image/jpeg"
	}

	// Leer archivo
	tempFile.Seek(0, 0)
	data := make([]byte, 0)
	buffer := make([]byte, 1024)
	for {
		n, err := tempFile.Read(buffer)
		if n > 0 {
			data = append(data, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}

	return data, contentType, nil
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
		img, err := imaging.Open(image.ImagePath)
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
