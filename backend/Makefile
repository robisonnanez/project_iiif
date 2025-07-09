BINARY_NAME=iiif-server
MAIN_FILE=main.go

.PHONY: build run clean test deps

# Compilar el proyecto
build:
	go build -o $(BINARY_NAME) $(MAIN_FILE)

# Compilar para producción
build-prod:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BINARY_NAME) $(MAIN_FILE)

# Ejecutar en modo desarrollo
run:
	go run $(MAIN_FILE)

# Ejecutar con hot reload (requiere air: go install github.com/cosmtrek/air@latest)
dev:
	air

# Limpiar archivos compilados
clean:
	go clean
	rm -f $(BINARY_NAME)

# Ejecutar tests
test:
	go test -v ./...

# Instalar dependencias
deps:
	go mod download
	go mod tidy

# Verificar código
lint:
	golangci-lint run

# Formatear código
fmt:
	go fmt ./...

# Generar documentación
docs:
	godoc -http=:6060

# Compilar para múltiples plataformas
build-all:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 $(MAIN_FILE)
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64.exe $(MAIN_FILE)
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 $(MAIN_FILE)
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 $(MAIN_FILE)