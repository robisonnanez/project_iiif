# Docker para Project IIIF

Esta guia muestra como construir una imagen Docker del backend Go y levantarla con MySQL. La imagen incluye el dashboard estatico en `backend/frontend` y usa `config.yaml` montado como volumen.

## 1. Archivos recomendados

Crea un `Dockerfile` en la raiz del proyecto:

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

Si prefieres construir CSS en la imagen, agrega antes de `go build` en el stage builder:

```dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm
RUN cd /src/backend/frontend && npm install && npm run build:css
```

Crea un `.dockerignore` en la raiz:

```gitignore
.git
.gitignore
backend/iiif-server
backend/data
backend/tmp
**/*.log
```

## 2. Configuracion para contenedor

Crea `docker/config.yaml` desde `backend/config.yaml.example`:

```bash
mkdir -p docker
cp backend/config.yaml.example docker/config.yaml
```

Ejemplo para MySQL dentro de `docker compose`:

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

iiif:
  base_url: "http://localhost:8080"
  api_version: "3"

frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"

migration:
  enabled: true
  allowed_local_roots:
    - "/data"
  max_log_lines: 1000
  ssh:
    connect_timeout_sec: 15
    allowed_hosts: []
```

En modo `mysql`, PDFs e imagenes quedan en BLOB dentro de MySQL; `/data/temp` solo se usa para temporales de procesamiento.

## 3. docker-compose.yml

Crea `docker-compose.yml` en la raiz:

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

Las migraciones se ejecutan automaticamente la primera vez que se crea el volumen de MySQL. Si ya existe el volumen, MySQL no vuelve a ejecutar `/docker-entrypoint-initdb.d`.

## 4. Build y ejecucion

```bash
docker compose build
docker compose up -d
docker compose logs -f iiif
```

Para migrar historico local a MySQL BLOB dentro del contenedor:

```bash
docker compose exec iiif /app/migrate-local-to-mysql
```

Pruebas:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/documents
open http://localhost:8080/
```

Para detener:

```bash
docker compose down
```

Para borrar datos y recrear desde cero:

```bash
docker compose down -v
docker compose up -d --build
```

## 5. Migraciones manuales

Si no quieres usar `/docker-entrypoint-initdb.d`, ejecuta:

```bash
docker compose exec -T mysql mysql -u project_iiif -p project_iiif < backend/migrations/001_create_documents.sql
docker compose exec -T mysql mysql -u project_iiif -p project_iiif < backend/migrations/002_add_blob_storage.sql
docker compose exec -T mysql mysql -u project_iiif -p project_iiif < backend/migrations/003_add_projects_multitenant.sql
```

Docker pedira el password configurado para `MYSQL_PASSWORD`.

## 6. Modo local en Docker

Para guardar PDFs e imagenes en filesystem dentro del volumen `/data`:

```yaml
STORAGE_BACKEND: "local"
DB_CONNECTION: "local"

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
```

En este modo puedes quitar el servicio `mysql` del compose si no lo necesitas.

## 7. Notas de produccion

- Cambia todos los passwords antes de desplegar.
- Ajusta `iiif.base_url` al dominio publico real.
- Usa un volumen persistente para MySQL y otro para `/data`.
- Si expones el dashboard, deja `frontend.require_auth: true`.
- Para actualizar version: `docker compose build iiif && docker compose up -d iiif`.
