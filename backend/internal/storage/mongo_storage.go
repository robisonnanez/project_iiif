package storage

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStorage struct {
	client      *mongo.Client
	db          *mongo.Database
	documents   *mongo.Collection
	images      *mongo.Collection
	pdfBucket   *gridfs.Bucket
	imageBucket *gridfs.Bucket
	basePath    string
}

func NewMongoStorage(cfg *config.Config) (*MongoStorage, error) {
	mongoCfg := cfg.Database.MongoDB
	uri := fmt.Sprintf("mongodb://%s:%s", mongoCfg.Host, mongoCfg.Port)

	clientOptions := options.Client().ApplyURI(uri)
	clientOptions.SetDirect(mongoCfg.DirectConnection)
	if mongoCfg.ServerSelectionTimeoutMS > 0 {
		clientOptions.SetServerSelectionTimeout(time.Duration(mongoCfg.ServerSelectionTimeoutMS) * time.Millisecond)
	}
	if mongoCfg.User != "" {
		authSource := mongoCfg.AuthSource
		if authSource == "" {
			authSource = "admin"
		}
		clientOptions.SetAuth(options.Credential{
			Username:   mongoCfg.User,
			Password:   mongoCfg.Password,
			AuthSource: authSource,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	db := client.Database(mongoCfg.Database)
	pdfBucket, err := gridfs.NewBucket(db, options.GridFSBucket().SetName("pdfs"))
	if err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	imageBucket, err := gridfs.NewBucket(db, options.GridFSBucket().SetName("images"))
	if err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	return &MongoStorage{
		client:      client,
		db:          db,
		documents:   db.Collection("documents"),
		images:      db.Collection("document_images"),
		pdfBucket:   pdfBucket,
		imageBucket: imageBucket,
		basePath:    cfg.Storage.DataPath,
	}, nil
}

func (ms *MongoStorage) SaveDocument(doc *models.PDFDocument) error {
	return ms.upsertDocument(doc)
}

func (ms *MongoStorage) UpdateDocument(doc *models.PDFDocument) error {
	return ms.upsertDocument(doc)
}

func (ms *MongoStorage) GetDocument(id string) (*models.PDFDocument, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var raw bson.M
	if err := ms.documents.FindOne(ctx, bson.M{"id": id}).Decode(&raw); err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	doc := decodeDocument(raw)
	doc.ImagePaths = ms.getImagePaths(doc.ID)
	return doc, nil
}

func (ms *MongoStorage) GetAllDocuments() ([]*models.PDFDocument, error) {
	return ms.GetDocumentsByScope("", "")
}

func (ms *MongoStorage) GetDocumentsByScope(projectKey, tenantKey string) ([]*models.PDFDocument, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{}
	if projectKey != "" {
		filter["project_key"] = projectKey
	}
	if tenantKey != "" {
		filter["tenant_key"] = tenantKey
	}

	cursor, err := ms.documents.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []*models.PDFDocument
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		doc := decodeDocument(raw)
		doc.ImagePaths = ms.getImagePaths(doc.ID)
		docs = append(docs, doc)
	}
	return docs, cursor.Err()
}

func (ms *MongoStorage) DeleteDocument(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var docRaw bson.M
	_ = ms.documents.FindOne(ctx, bson.M{"id": id}).Decode(&docRaw)
	if pdfID := objectIDFromMap(docRaw, "pdf_gridfs_file_id"); !pdfID.IsZero() {
		_ = ms.pdfBucket.Delete(pdfID)
	}

	cursor, err := ms.images.Find(ctx, bson.M{"document_id": id})
	if err == nil {
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var raw bson.M
			if err := cursor.Decode(&raw); err == nil {
				if imageID := objectIDFromMap(raw, "image_gridfs_file_id"); !imageID.IsZero() {
					_ = ms.imageBucket.Delete(imageID)
				}
			}
		}
	}

	if _, err := ms.images.DeleteMany(ctx, bson.M{"document_id": id}); err != nil {
		return err
	}
	_, err = ms.documents.DeleteOne(ctx, bson.M{"id": id})
	return err
}

func (ms *MongoStorage) SaveDocumentPDF(documentID string, data []byte, mediaType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if existing, _ := ms.documentBlobID(ctx, documentID); !existing.IsZero() {
		_ = ms.pdfBucket.Delete(existing)
	}

	fileID, err := ms.pdfBucket.UploadFromStream(
		documentID+".pdf",
		bytes.NewReader(data),
		options.GridFSUpload().SetMetadata(bson.M{
			"document_id": documentID,
			"media_type":  mediaType,
			"byte_size":   len(data),
		}),
	)
	if err != nil {
		return err
	}

	_, err = ms.documents.UpdateOne(ctx,
		bson.M{"id": documentID},
		bson.M{"$set": bson.M{
			"pdf_gridfs_file_id": fileID,
			"pdf_media_type":     mediaType,
			"pdf_size":           len(data),
		}},
	)
	return err
}

func (ms *MongoStorage) GetDocumentPDFData(documentID string) (*models.BinaryAsset, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fileID, err := ms.documentBlobID(ctx, documentID)
	if err != nil || fileID.IsZero() {
		return nil, fmt.Errorf("pdf GridFS no encontrado: %w", err)
	}
	stream, err := ms.pdfBucket.OpenDownloadStream(fileID)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	var buffer bytes.Buffer
	if _, err := buffer.ReadFrom(stream); err != nil {
		return nil, err
	}
	return &models.BinaryAsset{ID: documentID, Data: buffer.Bytes(), MediaType: "application/pdf", ByteSize: int64(buffer.Len())}, nil
}

func (ms *MongoStorage) SaveDocumentImage(image *models.DocumentImage) error {
	if image.CreatedAt.IsZero() {
		image.CreatedAt = time.Now()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := bson.M{
		"id":                  image.ID,
		"document_id":         image.DocumentID,
		"project_key":         defaultProject(image.ProjectKey),
		"tenant_key":          image.TenantKey,
		"migrated_from_local": image.MigratedFromLocal,
		"page_number":         image.PageNumber,
		"image_path":          image.ImagePath,
		"width":               image.Width,
		"height":              image.Height,
		"format":              image.Format,
		"media_type":          image.MediaType,
		"byte_size":           image.ByteSize,
	}

	_, err := ms.images.UpdateOne(
		ctx,
		bson.M{"id": image.ID},
		bson.M{
			"$set":         update,
			"$setOnInsert": bson.M{"created_at": image.CreatedAt},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (ms *MongoStorage) SaveDocumentImageData(imageID string, data []byte, mediaType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if existing, _ := ms.imageBlobID(ctx, imageID); !existing.IsZero() {
		_ = ms.imageBucket.Delete(existing)
	}

	fileID, err := ms.imageBucket.UploadFromStream(
		imageID,
		bytes.NewReader(data),
		options.GridFSUpload().SetMetadata(bson.M{
			"image_id":   imageID,
			"media_type": mediaType,
			"byte_size":  len(data),
		}),
	)
	if err != nil {
		return err
	}

	_, err = ms.images.UpdateOne(ctx,
		bson.M{"id": imageID},
		bson.M{"$set": bson.M{
			"image_gridfs_file_id": fileID,
			"media_type":           mediaType,
			"byte_size":            len(data),
		}},
	)
	return err
}

func (ms *MongoStorage) GetDocumentImage(id string) (*models.DocumentImage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var raw bson.M
	if err := ms.images.FindOne(ctx, bson.M{"id": id}).Decode(&raw); err != nil {
		return nil, fmt.Errorf("image not found: %w", err)
	}
	return decodeImage(raw), nil
}

func (ms *MongoStorage) GetDocumentImageByPage(documentID string, page int) (*models.DocumentImage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var raw bson.M
	if err := ms.images.FindOne(ctx, bson.M{"document_id": documentID, "page_number": page}).Decode(&raw); err != nil {
		return nil, fmt.Errorf("image not found: %w", err)
	}
	return decodeImage(raw), nil
}

func (ms *MongoStorage) GetDocumentImages(documentID string) ([]*models.DocumentImage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := ms.images.Find(ctx, bson.M{"document_id": documentID}, options.Find().SetSort(bson.D{{Key: "page_number", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var images []*models.DocumentImage
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		images = append(images, decodeImage(raw))
	}
	return images, cursor.Err()
}

func (ms *MongoStorage) GetDocumentImageData(id string) (*models.BinaryAsset, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	imageID, err := ms.imageBlobID(ctx, id)
	if err != nil {
		return nil, err
	}
	if imageID.IsZero() {
		return nil, fmt.Errorf("image blob not found")
	}

	stream, err := ms.imageBucket.OpenDownloadStream(imageID)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(stream); err != nil {
		return nil, err
	}

	image, err := ms.GetDocumentImage(id)
	if err != nil {
		return nil, err
	}

	return &models.BinaryAsset{
		ID:        id,
		Data:      buf.Bytes(),
		MediaType: image.MediaType,
		ByteSize:  int64(buf.Len()),
	}, nil
}

func (ms *MongoStorage) HasDocumentPDFBlob(documentID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fileID, err := ms.documentBlobID(ctx, documentID)
	return !fileID.IsZero(), err
}

func (ms *MongoStorage) HasImageBlob(imageID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fileID, err := ms.imageBlobID(ctx, imageID)
	return !fileID.IsZero(), err
}

func (ms *MongoStorage) upsertDocument(doc *models.PDFDocument) error {
	if doc.UploadDate.IsZero() {
		doc.UploadDate = time.Now()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := bson.M{
		"id":                  doc.ID,
		"original_name":       doc.Name,
		"project_key":         defaultProject(doc.ProjectKey),
		"tenant_key":          doc.TenantKey,
		"migrated_from_local": doc.MigratedFromLocal,
		"status":              doc.Status,
		"total_pages":         doc.TotalPages,
		"converted_pages":     doc.ConvertedPages,
		"conversion_width":    doc.ConversionWidth,
		"conversion_height":   doc.ConversionHeight,
		"conversion_dpi":      doc.ConversionDPI,
		"conversion_format":   doc.ConversionFormat,
		"conversion_quality":  doc.ConversionQuality,
		"outline":             doc.Outline,
		"pdf_path":            doc.FilePath,
		"thumbnail_path":      doc.ThumbnailURL,
		"manifest_url":        doc.ManifestURL,
		"updated_at":          time.Now(),
	}

	_, err := ms.documents.UpdateOne(
		ctx,
		bson.M{"id": doc.ID},
		bson.M{
			"$set":         update,
			"$setOnInsert": bson.M{"created_at": doc.UploadDate},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (ms *MongoStorage) getImagePaths(documentID string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := ms.images.Find(ctx,
		bson.M{"document_id": documentID, "image_path": bson.M{"$ne": ""}},
		options.Find().SetSort(bson.D{{Key: "page_number", Value: 1}}).SetProjection(bson.M{"image_path": 1}),
	)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var paths []string
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err == nil {
			if value, ok := raw["image_path"].(string); ok && value != "" {
				paths = append(paths, value)
			}
		}
	}
	return paths
}

func (ms *MongoStorage) documentBlobID(ctx context.Context, documentID string) (primitive.ObjectID, error) {
	var raw bson.M
	if err := ms.documents.FindOne(ctx, bson.M{"id": documentID}, options.FindOne().SetProjection(bson.M{"pdf_gridfs_file_id": 1})).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return primitive.NilObjectID, nil
		}
		return primitive.NilObjectID, err
	}
	return objectIDFromMap(raw, "pdf_gridfs_file_id"), nil
}

func (ms *MongoStorage) imageBlobID(ctx context.Context, imageID string) (primitive.ObjectID, error) {
	var raw bson.M
	if err := ms.images.FindOne(ctx, bson.M{"id": imageID}, options.FindOne().SetProjection(bson.M{"image_gridfs_file_id": 1})).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return primitive.NilObjectID, nil
		}
		return primitive.NilObjectID, err
	}
	return objectIDFromMap(raw, "image_gridfs_file_id"), nil
}

func decodeDocument(raw bson.M) *models.PDFDocument {
	doc := &models.PDFDocument{
		ID:                stringValue(raw, "id"),
		Name:              stringValue(raw, "original_name"),
		ProjectKey:        defaultProject(stringValue(raw, "project_key")),
		TenantKey:         stringValue(raw, "tenant_key"),
		MigratedFromLocal: boolValue(raw, "migrated_from_local"),
		UploadDate:        timeValue(raw, "created_at"),
		Status:            stringValue(raw, "status"),
		TotalPages:        intValue(raw, "total_pages"),
		ConvertedPages:    intValue(raw, "converted_pages"),
		ConversionWidth:   intValue(raw, "conversion_width"),
		ConversionHeight:  intValue(raw, "conversion_height"),
		ConversionDPI:     intValue(raw, "conversion_dpi"),
		ConversionFormat:  stringValue(raw, "conversion_format"),
		ConversionQuality: intValue(raw, "conversion_quality"),
		Outline:           outlineValue(raw, "outline"),
		ManifestURL:       stringValue(raw, "manifest_url"),
		ThumbnailURL:      stringValue(raw, "thumbnail_path"),
		FilePath:          stringValue(raw, "pdf_path"),
	}
	if doc.ProjectKey == "" {
		doc.ProjectKey = "default"
	}
	return doc
}

func outlineValue(raw bson.M, key string) []models.PDFOutlineItem {
	values, ok := raw[key].(primitive.A)
	if !ok {
		if generic, genericOK := raw[key].([]interface{}); genericOK {
			values = primitive.A(generic)
		} else {
			return nil
		}
	}
	result := make([]models.PDFOutlineItem, 0, len(values))
	for _, value := range values {
		item, ok := value.(bson.M)
		if !ok {
			continue
		}
		result = append(result, models.PDFOutlineItem{
			Level:      intValue(item, "level"),
			Title:      stringValue(item, "title"),
			PageNumber: intValue(item, "page_number"),
		})
	}
	return result
}

func decodeImage(raw bson.M) *models.DocumentImage {
	image := &models.DocumentImage{
		ID:                stringValue(raw, "id"),
		DocumentID:        stringValue(raw, "document_id"),
		ProjectKey:        defaultProject(stringValue(raw, "project_key")),
		TenantKey:         stringValue(raw, "tenant_key"),
		MigratedFromLocal: boolValue(raw, "migrated_from_local"),
		PageNumber:        intValue(raw, "page_number"),
		ImagePath:         stringValue(raw, "image_path"),
		Width:             intValue(raw, "width"),
		Height:            intValue(raw, "height"),
		Format:            stringValue(raw, "format"),
		MediaType:         stringValue(raw, "media_type"),
		ByteSize:          int64Value(raw, "byte_size"),
		CreatedAt:         timeValue(raw, "created_at"),
	}
	if image.ProjectKey == "" {
		image.ProjectKey = "default"
	}
	return image
}

func stringValue(raw bson.M, key string) string {
	if value, ok := raw[key].(string); ok {
		return value
	}
	return ""
}

func boolValue(raw bson.M, key string) bool {
	if value, ok := raw[key].(bool); ok {
		return value
	}
	return false
}

func intValue(raw bson.M, key string) int {
	switch value := raw[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}

func int64Value(raw bson.M, key string) int64 {
	switch value := raw[key].(type) {
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	}
	return 0
}

func timeValue(raw bson.M, key string) time.Time {
	if value, ok := raw[key].(primitive.DateTime); ok {
		return value.Time()
	}
	if value, ok := raw[key].(time.Time); ok {
		return value
	}
	return time.Time{}
}

func objectIDFromMap(raw bson.M, key string) primitive.ObjectID {
	if value, ok := raw[key].(primitive.ObjectID); ok {
		return value
	}
	return primitive.NilObjectID
}
