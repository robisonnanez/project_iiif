package main

import (
	"database/sql"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestIsMissingImageMetadata(t *testing.T) {
	for name, err := range map[string]error{
		"mysql": fmt.Errorf("image not found: %w", sql.ErrNoRows),
		"mongo": fmt.Errorf("image not found: %w", mongo.ErrNoDocuments),
	} {
		t.Run(name, func(t *testing.T) {
			if !isMissingImageMetadata(err) {
				t.Fatalf("expected %v to be recognized as missing metadata", err)
			}
		})
	}
}

func TestIsMissingImageMetadataRejectsOtherErrors(t *testing.T) {
	if isMissingImageMetadata(fmt.Errorf("connection refused")) {
		t.Fatal("unexpectedly classified a connection error as missing metadata")
	}
}
