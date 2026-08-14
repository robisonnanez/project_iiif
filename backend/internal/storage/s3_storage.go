package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	appconfig "iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	Storage
	client *s3.Client
	bucket string
}

func NewS3Storage(cfg *appconfig.Config, metadata Storage) (*S3Storage, error) {
	if strings.TrimSpace(cfg.AWSBucket) == "" {
		return nil, fmt.Errorf("AWS_BUCKET es obligatorio cuando FILESYSTEM_DISK=s3")
	}
	if strings.TrimSpace(cfg.AWSAccessKeyID) == "" || strings.TrimSpace(cfg.AWSSecretAccessKey) == "" {
		return nil, fmt.Errorf("AWS_ACCESS_KEY_ID y AWS_SECRET_ACCESS_KEY son obligatorios para S3")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.AWSDefaultRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AWSAccessKeyID,
			cfg.AWSSecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear cliente S3: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.AWSUsePathStyleEndpoint
		if strings.TrimSpace(cfg.AWSEndpoint) != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(cfg.AWSEndpoint, "/"))
		}
	})
	store := &S3Storage{Storage: metadata, client: client, bucket: cfg.AWSBucket}
	if err := store.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *S3Storage) ensureBucket(ctx context.Context) error {
	if err := s.CheckConnection(ctx); err == nil {
		return nil
	}
	if _, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("no se pudo acceder ni crear el bucket S3 %q: %w", s.bucket, err)
	}
	return s.CheckConnection(ctx)
}

func (s *S3Storage) CheckConnection(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		return fmt.Errorf("no se pudo acceder al bucket S3 %q: %w", s.bucket, err)
	}
	return nil
}

func (s *S3Storage) SmokeTest(ctx context.Context) error {
	key := "_health/project-iiif-smoke.txt"
	payload := []byte("project-iiif s3 ok")
	if err := s.putObject(key, payload, "text/plain"); err != nil {
		return err
	}
	defer s.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	asset, err := s.getObject("s3-smoke", s.reference(key))
	if err != nil {
		return err
	}
	if !bytes.Equal(asset.Data, payload) {
		return fmt.Errorf("el contenido leído desde S3 no coincide con el enviado")
	}
	return nil
}

func (s *S3Storage) SaveDocumentPDF(documentID string, data []byte, mediaType string) error {
	doc, err := s.Storage.GetDocument(documentID)
	if err != nil {
		return err
	}
	key := documentObjectKey(doc)
	if err := s.putObject(key, data, mediaType); err != nil {
		return err
	}
	doc.FilePath = s.reference(key)
	return s.Storage.UpdateDocument(doc)
}

func (s *S3Storage) SaveDocumentImageData(imageID string, data []byte, mediaType string) error {
	image, err := s.Storage.GetDocumentImage(imageID)
	if err != nil {
		return err
	}
	key := imageObjectKey(image)
	if err := s.putObject(key, data, mediaType); err != nil {
		return err
	}
	image.ImagePath = s.reference(key)
	image.MediaType = mediaType
	image.ByteSize = int64(len(data))
	return s.Storage.SaveDocumentImage(image)
}

func (s *S3Storage) GetDocumentPDFData(documentID string) (*models.BinaryAsset, error) {
	doc, err := s.Storage.GetDocument(documentID)
	if err != nil {
		return nil, err
	}
	return s.getObject(documentID, doc.FilePath)
}

func (s *S3Storage) GetDocumentImageData(imageID string) (*models.BinaryAsset, error) {
	image, err := s.Storage.GetDocumentImage(imageID)
	if err != nil {
		return nil, err
	}
	return s.getObject(imageID, image.ImagePath)
}

func (s *S3Storage) HasDocumentPDFBlob(documentID string) (bool, error) {
	doc, err := s.Storage.GetDocument(documentID)
	if err != nil {
		return false, err
	}
	return s.objectExists(doc.FilePath)
}

func (s *S3Storage) HasImageBlob(imageID string) (bool, error) {
	image, err := s.Storage.GetDocumentImage(imageID)
	if err != nil {
		return false, err
	}
	return s.objectExists(image.ImagePath)
}

func (s *S3Storage) DeleteDocument(id string) error {
	doc, _ := s.Storage.GetDocument(id)
	if doc != nil {
		_ = s.deletePrefix(scopePrefix(doc.ProjectKey, doc.TenantKey) + "/documents/" + safeSegment(doc.ID) + "/")
	}
	return s.Storage.DeleteDocument(id)
}

func (s *S3Storage) putObject(key string, data []byte, mediaType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		return fmt.Errorf("no se pudo guardar s3://%s/%s: %w", s.bucket, key, err)
	}
	return nil
}

func (s *S3Storage) getObject(id, reference string) (*models.BinaryAsset, error) {
	key, err := s.keyFromReference(reference)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer s3://%s/%s: %w", s.bucket, key, err)
	}
	defer result.Body.Close()
	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	mediaType := "application/octet-stream"
	if result.ContentType != nil {
		mediaType = *result.ContentType
	}
	return &models.BinaryAsset{ID: id, Data: data, MediaType: mediaType, ByteSize: int64(len(data))}, nil
}

func (s *S3Storage) objectExists(reference string) (bool, error) {
	key, err := s.keyFromReference(reference)
	if err != nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (s *S3Storage) deletePrefix(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var token *string
	for {
		result, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), Prefix: aws.String(prefix), ContinuationToken: token})
		if err != nil {
			return err
		}
		for _, object := range result.Contents {
			_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: object.Key})
		}
		if !aws.ToBool(result.IsTruncated) || result.NextContinuationToken == nil {
			return nil
		}
		token = result.NextContinuationToken
	}
}

func (s *S3Storage) reference(key string) string {
	return "s3://" + s.bucket + "/" + key
}

func (s *S3Storage) keyFromReference(reference string) (string, error) {
	prefix := "s3://" + s.bucket + "/"
	if !strings.HasPrefix(reference, prefix) {
		return "", fmt.Errorf("referencia S3 invalida: %s", reference)
	}
	return strings.TrimPrefix(reference, prefix), nil
}

func documentObjectKey(doc *models.PDFDocument) string {
	return path.Join(scopePrefix(doc.ProjectKey, doc.TenantKey), "documents", safeSegment(doc.ID), "document.pdf")
}

func imageObjectKey(image *models.DocumentImage) string {
	ext := strings.TrimPrefix(strings.ToLower(image.Format), ".")
	if ext == "" {
		ext = "jpg"
	}
	name := fmt.Sprintf("page_%06d_%s.%s", image.PageNumber, safeSegment(image.ID), ext)
	return path.Join(scopePrefix(image.ProjectKey, image.TenantKey), "documents", safeSegment(image.DocumentID), "images", name)
}

func scopePrefix(projectKey, tenantKey string) string {
	projectKey = safeSegment(projectKey)
	if projectKey == "" {
		projectKey = "default"
	}
	if tenant := safeSegment(tenantKey); tenant != "" {
		return path.Join("projects", projectKey, "tenants", tenant)
	}
	return path.Join("projects", projectKey)
}

func safeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "..", "")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	return value
}
