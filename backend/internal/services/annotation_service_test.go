package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	contract "iiif-pdf-server/internal/api"
	"iiif-pdf-server/internal/models"
	"iiif-pdf-server/internal/storage"
)

const (
	testDocumentID = "00000000-0000-4000-8000-000000000001"
	testGeneration = "00000000-0000-4000-8000-000000000002"
	testClientID   = "0199b252-3c6f-7f6a-9a13-6b7c8d9e0001"
	testLayerHash  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

type fakeAnnotationRepository struct {
	mu    sync.Mutex
	items map[string]*models.DocumentAnnotation
}

func newFakeAnnotationRepository() *fakeAnnotationRepository {
	return &fakeAnnotationRepository{items: map[string]*models.DocumentAnnotation{}}
}

func (r *fakeAnnotationRepository) CreateAnnotation(_ context.Context, annotation *models.DocumentAnnotation) (*models.DocumentAnnotation, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.TenantID == annotation.TenantID && existing.ClientReferenceID == annotation.ClientReferenceID {
			copy := *existing
			return &copy, true, nil
		}
	}
	copy := *annotation
	copy.CreatedAt, copy.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	r.items[copy.ID] = &copy
	return &copy, false, nil
}

func (r *fakeAnnotationRepository) GetAnnotationByClientReference(_ context.Context, tenant, clientReference string) (*models.DocumentAnnotation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.TenantID == tenant && existing.ClientReferenceID == clientReference {
			copy := *existing
			return &copy, nil
		}
	}
	return nil, storage.ErrAnnotationNotFound
}

func (r *fakeAnnotationRepository) GetAnnotation(_ context.Context, id, tenant, project string, includeDeleted bool) (*models.DocumentAnnotation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := r.items[id]
	if item == nil || item.TenantID != tenant || item.ProjectKey != project || (!includeDeleted && item.DeletedAt != nil) {
		return nil, storage.ErrAnnotationNotFound
	}
	copy := *item
	return &copy, nil
}

func (r *fakeAnnotationRepository) GetAnnotationsByIDs(ctx context.Context, ids []string, tenant, project, pageID string) ([]*models.DocumentAnnotation, error) {
	result := []*models.DocumentAnnotation{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		item, err := r.GetAnnotation(ctx, id, tenant, project, false)
		if err == nil && (pageID == "" || item.PageID == pageID) {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *fakeAnnotationRepository) UpdateAnnotation(_ context.Context, annotation *models.DocumentAnnotation, version int64) (*models.DocumentAnnotation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.items[annotation.ID]
	if current == nil || current.Version != version {
		return nil, storage.ErrAnnotationChanged
	}
	copy := *annotation
	copy.Version++
	copy.UpdatedAt = time.Now().UTC()
	r.items[copy.ID] = &copy
	return &copy, nil
}

func (r *fakeAnnotationRepository) SoftDeleteAnnotation(_ context.Context, id, tenant, project string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := r.items[id]
	if item == nil || item.TenantID != tenant || item.ProjectKey != project {
		return storage.ErrAnnotationNotFound
	}
	if item.DeletedAt == nil {
		now := time.Now().UTC()
		item.DeletedAt = &now
	}
	return nil
}

type fakeAnnotationMetadata struct {
	document *models.PDFDocument
	page     *models.DocumentImage
}

func (f fakeAnnotationMetadata) GetDocument(id string) (*models.PDFDocument, error) {
	if id != f.document.ID {
		return nil, errors.New("not found")
	}
	return f.document, nil
}
func (f fakeAnnotationMetadata) GetDocumentImage(id string) (*models.DocumentImage, error) {
	if id != f.page.ID {
		return nil, errors.New("not found")
	}
	return f.page, nil
}

type fakeAnnotationTextLayer struct{ active string }

func (f fakeAnnotationTextLayer) GetTextLayerPage(documentID, generation string, page int) (*contract.TextLayerPage, error) {
	if documentID != testDocumentID || generation != testGeneration || page != 1 {
		return nil, errors.New("not found")
	}
	return &contract.TextLayerPage{DocumentID: documentID, Generation: generation, PageNumber: page,
		Canvas: contract.Canvas{ID: "https://iiif.test/canvas/1", Width: 1241, Height: 1754}, LayerSHA256: testLayerHash}, nil
}
func (f fakeAnnotationTextLayer) ListOCRGenerations(documentID string) (*contract.OCRGenerationList, error) {
	if documentID != testDocumentID {
		return nil, errors.New("not found")
	}
	return &contract.OCRGenerationList{DocumentID: documentID, ActiveGeneration: f.active}, nil
}

func newAnnotationServiceTest() (*AnnotationService, *fakeAnnotationRepository) {
	repository := newFakeAnnotationRepository()
	metadata := fakeAnnotationMetadata{
		document: &models.PDFDocument{ID: testDocumentID, ProjectKey: "project-a", TenantKey: "tenant-a", Status: "completed", TotalPages: 1},
		page:     &models.DocumentImage{ID: "stable-page-1", DocumentID: testDocumentID, ProjectKey: "project-a", TenantKey: "tenant-a", PageNumber: 1},
	}
	return NewAnnotationService(repository, metadata, fakeAnnotationTextLayer{active: testGeneration}), repository
}

func validCreateAnnotationInput() CreateAnnotationInput {
	note, color := "nota", "yellow"
	return CreateAnnotationInput{ProjectKey: "project-a", TenantID: "tenant-a", DocumentID: testDocumentID,
		PageID: "stable-page-1", ClientReferenceID: testClientID, OCRGenerationID: testGeneration,
		LayerSHA256: testLayerHash, SelectedText: "fragmento seleccionado", Note: &note, Color: &color,
		Selector: models.AnnotationSelector{Type: "MultiRectangleSelector", CoordinateSpace: "canvas_normalized",
			Canvas:     models.AnnotationCanvas{ID: "https://iiif.test/canvas/1", Width: 1241, Height: 1754},
			Rectangles: []models.AnnotationRectangle{{X: .23, Y: .41, Width: .32, Height: .018}},
			Quote:      models.AnnotationQuote{Exact: "fragmento   seleccionado", Prefix: "antes", Suffix: "después"}}}
}

func TestAnnotationCreateAndIdempotentReplay(t *testing.T) {
	service, repository := newAnnotationServiceTest()
	input := validCreateAnnotationInput()
	created, replayed, err := service.Create(context.Background(), input)
	if err != nil || replayed {
		t.Fatalf("create replayed=%v err=%v", replayed, err)
	}
	if created.Version != 1 || created.Historical || !created.GenerationActive {
		t.Fatalf("unexpected annotation: %#v", created)
	}
	replayedAnnotation, replayed, err := service.Create(context.Background(), input)
	if err != nil || !replayed || replayedAnnotation.ID != created.ID {
		t.Fatalf("replay=%v annotation=%#v err=%v", replayed, replayedAnnotation, err)
	}
	if len(repository.items) != 1 {
		t.Fatalf("records=%d", len(repository.items))
	}
}

func TestAnnotationConcurrentIdempotencyCreatesOneRecord(t *testing.T) {
	service, repository := newAnnotationServiceTest()
	input := validCreateAnnotationInput()
	var wait sync.WaitGroup
	errorsFound := make(chan error, 12)
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, err := service.Create(context.Background(), input)
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(repository.items) != 1 {
		t.Fatalf("records=%d", len(repository.items))
	}
}

func TestAnnotationIdempotencyRejectsDifferentPayload(t *testing.T) {
	service, _ := newAnnotationServiceTest()
	input := validCreateAnnotationInput()
	if _, _, err := service.Create(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	input.SelectedText, input.Selector.Quote.Exact = "otro texto", "otro texto"
	if _, _, err := service.Create(context.Background(), input); !errors.Is(err, ErrAnnotationIdempotencyConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestAnnotationMultiRectangleIsPreserved(t *testing.T) {
	service, _ := newAnnotationServiceTest()
	input := validCreateAnnotationInput()
	input.Selector.Rectangles = append(input.Selector.Rectangles, models.AnnotationRectangle{X: .2, Y: .44, Width: .36, Height: .02})
	created, _, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Selector.Rectangles) != 2 {
		t.Fatalf("rectangles=%d", len(created.Selector.Rectangles))
	}
}

func TestAnnotationSelectorValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreateAnnotationInput)
	}{
		{"negative", func(input *CreateAnnotationInput) { input.Selector.Rectangles[0].X = -.1 }},
		{"nan", func(input *CreateAnnotationInput) { input.Selector.Rectangles[0].X = math.NaN() }},
		{"overflow", func(input *CreateAnnotationInput) {
			input.Selector.Rectangles[0].X, input.Selector.Rectangles[0].Width = .8, .3
		}},
		{"empty", func(input *CreateAnnotationInput) { input.Selector.Rectangles = nil }},
		{"too-many", func(input *CreateAnnotationInput) {
			input.Selector.Rectangles = make([]models.AnnotationRectangle, 201)
			for i := range input.Selector.Rectangles {
				input.Selector.Rectangles[i] = models.AnnotationRectangle{Width: .01, Height: .01}
			}
		}},
		{"quote", func(input *CreateAnnotationInput) { input.Selector.Quote.Exact = "texto distinto" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newAnnotationServiceTest()
			input := validCreateAnnotationInput()
			test.mutate(&input)
			if _, _, err := service.Create(context.Background(), input); !errors.Is(err, ErrInvalidAnnotationSelector) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestAnnotationRejectsWrongPageGenerationAndHash(t *testing.T) {
	mutations := []func(*CreateAnnotationInput){
		func(input *CreateAnnotationInput) { input.PageID = "other-page" },
		func(input *CreateAnnotationInput) { input.OCRGenerationID = "00000000-0000-4000-8000-000000000099" },
		func(input *CreateAnnotationInput) { input.LayerSHA256 = fmt.Sprintf("%064d", 9) },
	}
	for index, mutate := range mutations {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			service, _ := newAnnotationServiceTest()
			input := validCreateAnnotationInput()
			mutate(&input)
			_, _, err := service.Create(context.Background(), input)
			if index == 0 && !errors.Is(err, ErrAnnotationResourceNotFound) {
				t.Fatalf("err=%v", err)
			}
			if index > 0 && !errors.Is(err, ErrAnnotationGenerationMismatch) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestAnnotationTenantIsolationBatchPatchAndDelete(t *testing.T) {
	service, _ := newAnnotationServiceTest()
	input := validCreateAnnotationInput()
	created, _, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(context.Background(), created.ID, "tenant-b", "project-a"); !errors.Is(err, ErrAnnotationResourceNotFound) {
		t.Fatalf("cross tenant err=%v", err)
	}
	batch, err := service.Batch(context.Background(), []string{created.ID, "00000000-0000-4000-8000-000000000099"}, "tenant-a", "project-a", "")
	if err != nil || len(batch) != 1 {
		t.Fatalf("batch=%d err=%v", len(batch), err)
	}
	if _, err := service.Patch(context.Background(), created.ID, "tenant-a", "project-a", 99, PatchAnnotationInput{ColorSet: true}); !errors.Is(err, ErrAnnotationChanged) {
		t.Fatalf("stale err=%v", err)
	}
	newColor := "blue"
	updated, err := service.Patch(context.Background(), created.ID, "tenant-a", "project-a", 1, PatchAnnotationInput{ColorSet: true, Color: &newColor})
	if err != nil || updated.Version != 2 || updated.Color == nil || *updated.Color != "blue" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	if err := service.Delete(context.Background(), created.ID, "tenant-a", "project-a"); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(context.Background(), created.ID, "tenant-a", "project-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(context.Background(), created.ID, "tenant-a", "project-a"); !errors.Is(err, ErrAnnotationResourceNotFound) {
		t.Fatalf("after delete err=%v", err)
	}
}

func TestAnnotationHistoricalStateChangesWithoutDeleting(t *testing.T) {
	service, repository := newAnnotationServiceTest()
	input := validCreateAnnotationInput()
	created, _, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	service.textLayer = fakeAnnotationTextLayer{active: "00000000-0000-4000-8000-000000000099"}
	found, err := service.Get(context.Background(), created.ID, "tenant-a", "project-a")
	if err != nil || !found.Historical || found.GenerationActive {
		t.Fatalf("found=%#v err=%v", found, err)
	}
	if len(repository.items) != 1 {
		t.Fatal("historical annotation disappeared")
	}
}
