package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"iiif-pdf-server/internal/config"
	"iiif-pdf-server/internal/storage"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("no se pudo cargar config.yaml: %v", err)
	}
	store, err := storage.NewS3Storage(cfg, nil)
	if err != nil {
		log.Fatalf("no se pudo conectar a S3/RustFS: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.SmokeTest(ctx); err != nil {
		log.Fatalf("prueba S3/RustFS fallida: %v", err)
	}
	fmt.Println("S3/RustFS OK: escritura, lectura y eliminación verificadas")
}
