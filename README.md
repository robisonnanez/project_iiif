# IIIF PDF Server

Servidor Go que convierte archivos PDF a imágenes y los sirve usando el protocolo IIIF v3.

## Características

- ✅ Conversión de PDF a imágenes (JPEG, PNG, WebP)
- ✅ API IIIF Image v3 completa
- ✅ Generación de manifiestos IIIF Presentation v3
- ✅ Integración con Universal Viewer
- ✅ Sistema de caché configurable
- ✅ Configuración flexible via YAML
- ✅ API REST para gestión de documentos

## Instalación

1. **Instalar dependencias del sistema:**

```bash
# Ubuntu/Debian
sudo apt-get install libmupdf-dev

# macOS
brew install mupdf-tools

# Windows
# Descargar MuPDF desde https://mupdf.com/downloads/
```

2. **Clonar y compilar:**

```bash
git clone <tu-repo>
cd backend
go mod download
go build -o iiif-server main.go
```

3. **Ejecutar:**

```bash
./iiif-server
```

## Configuración

Edita `config.yaml` para personalizar:

- Puerto del servidor
- Rutas de almacenamiento
- Límites de archivos
- Configuración IIIF
- Parámetros de conversión

## API Endpoints

### Gestión de documentos
- `POST /api/upload` - Subir PDF
- `GET /api/documents` - Listar documentos
- `GET /api/documents/:id` - Obtener documento
- `DELETE /api/documents/:id` - Eliminar documento

### IIIF
- `GET /api/iiif/:id/manifest` - Manifiesto IIIF
- `GET /api/iiif/:id/:page/info.json` - Info de imagen
- `GET /api/iiif/:id/:page/:size/:rotation/:quality.:format` - Imagen IIIF
- `GET /api/iiif/:id/:page/default.jpg` - Imagen por defecto

### Configuración
- `GET /api/properties` - Obtener propiedades
- `PUT /api/properties` - Actualizar propiedades

## Estructura del proyecto

```
backend/
├── main.go                 # Punto de entrada
├── config.yaml            # Configuración
├── internal/
│   ├── config/            # Gestión de configuración
│   ├── models/            # Modelos de datos
│   ├── storage/           # Almacenamiento
│   ├── services/          # Lógica de negocio
│   └── handlers/          # Controladores HTTP
└── data/                  # Datos (creado automáticamente)
    ├── documents/         # Metadatos de documentos
    ├── images/           # Imágenes convertidas
    ├── thumbnails/       # Miniaturas
    └── manifests/        # Manifiestos IIIF
```

## Desarrollo

Para desarrollo, usa:

```bash
go run main.go
```

Para producción:

```bash
go build -ldflags="-s -w" -o iiif-server main.go
```

<!-- ## Docker (Opcional)

```dockerfile
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache musl-dev gcc mupdf-dev
WORKDIR /app
COPY . .
RUN go build -o iiif-server main.go

FROM alpine:latest
RUN apk add --no-cache mupdf
WORKDIR /app
COPY --from=builder /app/iiif-server .
COPY config.yaml .
EXPOSE 8080
CMD ["./iiif-server"]
``` -->
