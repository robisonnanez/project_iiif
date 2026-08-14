# Docker para Project IIIF

Esta guía explica cómo ejecutar Project IIIF con Docker y Docker Compose en tres escenarios:

- modo local
- modo MySQL / PostgreSQL
- modo MongoDB

La imagen incluye el backend Go, el dashboard en `backend/frontend` y las migraciones SQL del proyecto.

## 1. Archivos recomendados

### Dockerfile

Crea un `Dockerfile` en la raíz del proyecto:

```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.24-bookworm AS builder

WORKDIR /src/backend
RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential libmupdf-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/iiif-server main.go
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/migrate-local-to-mysql ./cmd/migrate-local-to-mysql

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends libmupdf-dev ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/iiif-server /app/iiif-server
COPY --from=builder /out/migrate-local-to-mysql /app/migrate-local-to-mysql
COPY backend/frontend /app/frontend
COPY backend/migrations /app/migrations
COPY backend/config.yaml.example /app/config.yaml.example

RUN mkdir -p /data/pdfs /data/images /data/documents /data/thumbnails /data/manifests /data/temp \
    && useradd -r -u 10001 -g root iiif \
    && chown -R iiif:root /app /data

USER iiif
EXPOSE 8080
CMD ["/app/iiif-server"]
```

Si quieres compilar CSS dentro de la imagen, agrega antes del `go build`:

```dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm
RUN cd /src/backend/frontend && npm install && npm run build:css
```

### .dockerignore

```gitignore
.git
.gitignore
backend/iiif-server
backend/data
backend/tmp
**/*.log
```

## 2. Crear la configuración del contenedor

```bash
mkdir -p docker
cp backend/config.yaml.example docker/config.yaml
```

## 3. Ejemplo de configuración por motor

### Opción A: modo local

```yaml
STORAGE_BACKEND: "local"
DB_CONNECTION: "local"

server:
  port: "8080"
  mode: "release"

storage:
  backend: "local"
  data_path: "/data"
  pdfs_path: "/data/pdfs"
  images_path: "/data/images"
  documents_path: "/data/documents"
  thumbnails_path: "/data/thumbnails"
  manifests_path: "/data/manifests"

binary_storage:
  mode: "local"
  temp_path: "/data/temp"

pdf:
  temp_path: "/data/temp"

frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
```

### Opción B: modo MySQL

```yaml
STORAGE_BACKEND: "mysql"
DB_CONNECTION: "mysql"
DB_HOST: "mysql"
DB_PORT: "3306"
DB_DATABASE: "project_iiif"
DB_USERNAME: "project_iiif"
DB_PASSWORD: "PASSWORD_SEGURO"

server:
  port: "8080"
  mode: "release"

storage:
  backend: "mysql"
  data_path: "/data"
  pdfs_path: "/data/pdfs"
  images_path: "/data/images"
  documents_path: "/data/documents"
  thumbnails_path: "/data/thumbnails"
  manifests_path: "/data/manifests"

binary_storage:
  mode: "database"
  temp_path: "/data/temp"

pdf:
  temp_path: "/data/temp"

frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
```

### Opción C: modo MongoDB

```yaml
STORAGE_BACKEND: "mongodb"
DB_CONNECTION: "mongodb"
DB_HOST: "mongo"
DB_PORT: "27017"
DB_DATABASE: "project_iiif"
DB_USERNAME: ""
DB_PASSWORD: ""

server:
  port: "8080"
  mode: "release"

storage:
  backend: "mongodb"
  data_path: "/data"
  pdfs_path: "/data/pdfs"
  images_path: "/data/images"
  documents_path: "/data/documents"
  thumbnails_path: "/data/thumbnails"
  manifests_path: "/data/manifests"

database:
  mongodb:
    host: "mongo"
    port: "27017"
    user: ""
    password: ""
    database: "project_iiif"
    auth_source: "admin"
    direct_connection: true
    server_selection_timeout_ms: 2000

binary_storage:
  mode: "database"
  temp_path: "/data/temp"

pdf:
  temp_path: "/data/temp"

frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
```

Notas para MongoDB:

- Si el contenedor Mongo no tiene autenticación, deja usuario y contraseña vacíos.
- Si usas autenticación, completa `DB_USERNAME`, `DB_PASSWORD` y `auth_source`.
- El dashboard está pensado para URIs `mongodb://...`.
- El backend todavía no soporta `mongodb+srv://...`.

### Opción D: RustFS / S3 para binarios

Combina esta sección con cualquiera de los motores anteriores:

```yaml
FILESYSTEM_DISK: "s3"
AWS_ACCESS_KEY_ID: "TU_ACCESS_KEY_DE_RUSTFS"
AWS_SECRET_ACCESS_KEY: "TU_SECRET_KEY_DE_RUSTFS"
AWS_DEFAULT_REGION: "us-east-1"
AWS_BUCKET: "mi-proyecto"
AWS_ENDPOINT: "http://rustfs:9000"
AWS_USE_PATH_STYLE_ENDPOINT: true

binary_storage:
  mode: "s3"
  temp_path: "/data/temp"
```

Ejemplo de servicio RustFS para Compose:

```yaml
services:
  rustfs:
    image: rustfs/rustfs:latest
    environment:
      RUSTFS_ACCESS_KEY: TU_ACCESS_KEY_DE_RUSTFS
      RUSTFS_SECRET_KEY: TU_SECRET_KEY_DE_RUSTFS
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - project_iiif_rustfs:/data

volumes:
  project_iiif_rustfs:
```

#### Flujo de metadatos y binarios

El servicio `iiif` debe conectarse tanto al motor de metadatos como a RustFS:

```text
MySQL/PostgreSQL/MongoDB
  documents.pdf_path       -> s3://bucket/clave-del-pdf
  document_images.image_path -> s3://bucket/clave-de-la-imagen

RustFS
  bucket/clave-del-pdf       -> bytes del PDF
  bucket/clave-de-la-imagen  -> bytes JPEG/PNG/WebP
```

La galería y los endpoints IIIF consultan primero la base de datos y después descargan el objeto indicado por `image_path`. No montes el volumen interno de RustFS dentro del contenedor IIIF ni construyas rutas de filesystem con esos valores; deben tratarse como referencias S3.

Para contenedores, usa el nombre del servicio como endpoint:

```yaml
AWS_ENDPOINT: "http://rustfs:9000"
```

`http://127.0.0.1:9000` solo es correcto cuando RustFS comparte la misma red de proceso o se ejecuta directamente en el host accesible desde la aplicación.

Verificación desde el contenedor IIIF:

```bash
docker compose exec iiif ./s3-smoke
curl -f http://127.0.0.1:8080/health
curl -f -o pagina.jpg \
  http://127.0.0.1:8080/iiif/3/{document_id}_page_1/full/600,/0/default.jpg
file pagina.jpg
```

En el dashboard, selecciona la base en `Backend de metadatos` y `s3` en `Modo binario`. El bloque `S3 / RustFS` permanece deshabilitado para los modos `local` y `database`.

## 4. docker-compose.yml

### Compose para MySQL

```yaml
services:
  mysql:
    image: mysql:8.4
    container_name: project-iiif-mysql
    environment:
      MYSQL_DATABASE: project_iiif
      MYSQL_USER: project_iiif
      MYSQL_PASSWORD: PASSWORD_SEGURO
      MYSQL_ROOT_PASSWORD: ROOT_PASSWORD_SEGURO
    ports:
      - "3306:3306"
    volumes:
      - project_iiif_mysql:/var/lib/mysql
      - ./backend/migrations:/docker-entrypoint-initdb.d:ro
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 10

  iiif:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: project-iiif
    depends_on:
      mysql:
        condition: service_healthy
    ports:
      - "8080:8080"
    volumes:
      - ./docker/config.yaml:/app/config.yaml:ro
      - project_iiif_data:/data
    restart: unless-stopped

volumes:
  project_iiif_mysql:
  project_iiif_data:
```

### Compose para MongoDB

```yaml
services:
  mongo:
    image: mongo:8
    container_name: project-iiif-mongo
    ports:
      - "27017:27017"
    volumes:
      - project_iiif_mongo:/data/db

  iiif:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: project-iiif
    depends_on:
      - mongo
    ports:
      - "8080:8080"
    volumes:
      - ./docker/config.yaml:/app/config.yaml:ro
      - project_iiif_data:/data
    restart: unless-stopped

volumes:
  project_iiif_mongo:
  project_iiif_data:
```

### Compose para modo local

```yaml
services:
  iiif:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: project-iiif
    ports:
      - "8080:8080"
    volumes:
      - ./docker/config.yaml:/app/config.yaml:ro
      - project_iiif_data:/data
    restart: unless-stopped

volumes:
  project_iiif_data:
```

## 5. Construcción y arranque

```bash
docker compose build
docker compose up -d
docker compose logs -f iiif
```

Pruebas rápidas:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/documents
```

## 6. Migración de histórico dentro del contenedor

El binario del migrador conserva el nombre histórico `migrate-local-to-mysql`, pero hoy funciona con el motor activo, incluido MongoDB.

```bash
docker compose exec iiif /app/migrate-local-to-mysql
```

Si la configuración activa apunta a MongoDB:

- migra metadatos a colecciones Mongo
- guarda PDFs en GridFS `pdfs`
- guarda imágenes en GridFS `images`

## 7. Reinicio y limpieza

Detener:

```bash
docker compose down
```

Borrar datos y recrear:

```bash
docker compose down -v
docker compose up -d --build
```

## 8. Migraciones SQL manuales

Solo aplica para MySQL:

```bash
docker compose exec -T mysql mysql -u project_iiif -p project_iiif < backend/migrations/001_create_documents.sql
docker compose exec -T mysql mysql -u project_iiif -p project_iiif < backend/migrations/002_add_blob_storage.sql
docker compose exec -T mysql mysql -p project_iiif < backend/migrations/003_add_projects_multitenant.sql
```

## 9. Recomendaciones de producción

- Cambia todos los passwords antes de publicar.
- Ajusta `iiif.base_url` al dominio real.
- Usa volúmenes persistentes para `/data` y para la base de datos.
- Mantén `frontend.require_auth: true` si expones el dashboard.
- En MongoDB con autenticación, valida que el usuario tenga permisos sobre la base configurada.
## Conversión configurable de PDFs

Al seleccionar un PDF en el dashboard se abre un cuadro para definir ancho y alto máximos, DPI, formato y calidad. Los valores recomendados para consulta web e IIIF son `1241 × 1754 px`, `150 DPI`, `JPG` y calidad `85`. Las dimensiones conservan la proporción original.

En instalaciones existentes ejecuta las migraciones después de actualizar la imagen o el código:

```bash
curl -X POST http://localhost:8080/api/v1/admin/db/migrations/run
```

El endpoint requiere la sesión administrativa cuando la autenticación está activa. La migración agrega a `documents` los campos que registran la configuración utilizada; MongoDB los crea automáticamente al guardar cada documento.

Los documentos migrados y completados pueden generar el manifiesto desde el dashboard, tanto para todas las páginas como para una selección (`1-5,8,10-12`).
