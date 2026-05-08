package services

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/go-fitz"
	"github.com/google/uuid"
)

type PDFService struct {
	config  *config.Config
	storage storage.Storage
}

func NewPDFService(config *config.Config, storage storage.Storage) *PDFService {
	return &PDFService{
		config:  config,
		storage: storage,
	}
}

func (s *PDFService) ProcessPDF(sourcePath string, filename string, settings models.ConversionSettings) (*models.PDFDocument, error) {
	if settings.Format == "" {
		settings.Format = s.config.Conversion.DefaultFormat
	}
	if settings.Quality == 0 {
		settings.Quality = s.config.Conversion.DefaultQuality
	}

	documentID := uuid.New().String()
	pdfPath := filepath.Join(s.config.Storage.PDFsPath, documentID+filepath.Ext(filename))
	if err := copyFile(sourcePath, pdfPath); err != nil {
		return nil, fmt.Errorf("error storing original PDF: %w", err)
	}

	// Crear Documento
	doc := &models.PDFDocument{
		ID:             documentID,
		Name:           filename,
		UploadDate:     time.Now(),
		Status:         "processing",
		FilePath:       pdfPath,
		ConvertedPages: 0,
	}

	// Guardar documento inicial
	if err := s.storage.SaveDocument(doc); err != nil {
		return nil, fmt.Errorf("Error saving document: %w", err)
	}

	// Procesar pdf en goroutine
	go s.convertPDFToImages(doc, settings)

	return doc, nil
}

func (s *PDFService) convertPDFToImages(doc *models.PDFDocument, settings models.ConversionSettings) {
	defer func() {
		if r := recover(); r != nil {
			doc.Status = "error"
			s.storage.UpdateDocument(doc)
		}
	}()

	// Abrir PDF
	pdf, err := fitz.New(doc.FilePath)
	if err != nil {
		doc.Status = "error"
		s.storage.UpdateDocument(doc)
		return
	}
	defer pdf.Close()

	doc.TotalPages = pdf.NumPage()
	s.storage.UpdateDocument(doc)

	// Crear directorio para imágenes
	imageDir := filepath.Join(s.config.Storage.ImagesPath, doc.ID)
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		doc.Status = "error"
		s.storage.UpdateDocument(doc)
		return
	}

	var imagePaths []string

	// Convertir cada página
	for i := 0; i < pdf.NumPage(); i++ {
		img, err := pdf.Image(i)
		if err != nil {
			continue
		}
		var pageImage image.Image = img

		// Redimensionar si es necesario
		if settings.MaxWidth > 0 || settings.MaxHeight > 0 {
			pageImage = imaging.Fit(pageImage, settings.MaxWidth, settings.MaxHeight, imaging.Lanczos)
		}

		// Guardar Imagen
		imagePath := filepath.Join(imageDir, fmt.Sprintf("page_%d.%s", i+1, settings.Format))
		if err := s.saveImage(pageImage, imagePath, settings); err != nil {
			continue
		}

		bounds := pageImage.Bounds()
		image := &models.DocumentImage{
			ID:         uuid.New().String(),
			DocumentID: doc.ID,
			PageNumber: i + 1,
			ImagePath:  imagePath,
			Width:      bounds.Dx(),
			Height:     bounds.Dy(),
			Format:     settings.Format,
			CreatedAt:  time.Now(),
		}
		if err := s.storage.SaveDocumentImage(image); err != nil {
			continue
		}

		imagePaths = append(imagePaths, imagePath)
		doc.ConvertedPages++
		s.storage.UpdateDocument(doc)

		// Crear thumbnail de la primera página
		if i == 0 {
			thumbnailPath := filepath.Join(s.config.Storage.ThumbnailsPath, doc.ID+".jpg")
			thumbnail := imaging.Resize(pageImage, 200, 0, imaging.Lanczos)
			if err := s.saveImage(thumbnail, thumbnailPath, models.ConversionSettings{Format: "jpg", Quality: 80}); err == nil {
				doc.ThumbnailURL = fmt.Sprintf("%s/static/thumbnails/%s.jpg", s.config.IIIF.BaseURL, doc.ID)
				s.storage.UpdateDocument(doc)
			}
		}
	}

	doc.ImagePaths = imagePaths
	doc.Status = "completed"
	doc.ManifestURL = fmt.Sprintf("%s/api/iiif/%s/manifest", s.config.IIIF.BaseURL, doc.ID)

	s.storage.UpdateDocument(doc)
}

func (s *PDFService) saveImage(img image.Image, path string, settings models.ConversionSettings) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	switch settings.Format {
	case "jpg", "jpeg":
		return jpeg.Encode(file, img, &jpeg.Options{Quality: settings.Quality})
	case "png":
		return png.Encode(file, img)
	case "webp":
		// Para WebP necesitarías una librería adicional como github.com/chai2010/webp
		// Por ahora usamos JPEG como fallback
		return jpeg.Encode(file, img, &jpeg.Options{Quality: settings.Quality})
	default:
		return jpeg.Encode(file, img, &jpeg.Options{Quality: settings.Quality})
	}
}

func copyFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
