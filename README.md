# project_iiif

Servidor Go para convertir PDF en imágenes, almacenar metadatos en MySQL, PostgreSQL o MongoDB y publicar contenido mediante IIIF. Los binarios pueden residir en disco, en la base de datos o en un servicio S3 compatible como RustFS. Incluye dashboard React, migración de históricos y OpenAPI.

## Arquitectura

```text
Dashboard React / cliente IIIF
              |
          API Go
        /         \
metadatos         binarios
MySQL             local
PostgreSQL        BLOB/GridFS
MongoDB           RustFS/S3
```

La base de metadatos conserva documentos, páginas, dimensiones, estado, configuración de conversión, outline y referencias. S3 conserva los bytes de PDF e imágenes. Las referencias `s3://bucket/key` son internas y nunca se publican en el manifiesto.

## Funcionalidad

- IIIF Presentation API v2 en `/api/iiif/{id}/manifest` y compatibilidad v3 en `/api/iiif/v3/{id}/manifest`.
- IIIF Image API v2 y v3.
- `structures` v2 generadas únicamente desde bookmarks/outlines reales del PDF, con jerarquía y IDs deterministas.
- Manifiestos parciales mediante `?pages=1-5,8,10-12`, con rangos podados y sin referencias rotas.
- Conversión proporcional con valores iniciales 1241×1754, 150 DPI, JPG y calidad 85.
- Dashboard React/TypeScript con navegación horizontal, configuración persistente, carga de PDF y generación de manifiestos.
- Migración local, base de datos o SSH hacia S3, idempotente y con estado final coherente.

## Requisitos

- Go 1.24 o posterior y toolchain C para desarrollo backend.
- Node.js 22 y pnpm 10 para el frontend.
- Docker Engine con Compose para el entorno integral.

## Quick start con Docker

```bash
cp .env.example .env
# Cambia todos los valores change-this-* antes de continuar.
docker compose --profile mysql build --no-cache
docker compose --profile mysql up -d --wait
curl -f http://127.0.0.1:18080/health
```

Perfiles disponibles: `mysql`, `postgres` y `mongodb`. La aplicación se publica por defecto en `18080`, la API S3 de RustFS en `19000` y su consola en `19001`; pueden cambiarse mediante `APP_PORT`, `RUSTFS_API_PORT` y `RUSTFS_CONSOLE_PORT`.

Consulta [docs/DOCKER.md](docs/DOCKER.md) para la matriz completa, volúmenes, backups y troubleshooting.

## Desarrollo

Backend:

```bash
cd backend
cp config.yaml.example config.yaml
go test ./...
go run .
```

Frontend:

```bash
cd backend/frontend
corepack enable
pnpm install --frozen-lockfile
pnpm run lint
pnpm test
pnpm run build
```

El frontend compilado se sirve desde `backend/frontend/dist`. No depende de PrimeReact, PrimeIcons ni PrimeFlex.

## Configuración

`CONFIG_PATH` selecciona el YAML activo. En Docker es `/app/config/config.yaml`. Las variables de entorno prevalecen para motor, conexión, S3, URL pública IIIF y credenciales del dashboard.

Claves principales:

- `STORAGE_BACKEND` / `DB_CONNECTION`: `local`, `mysql`, `postgres` o `mongodb`.
- `binary_storage.mode`: `local`, `database` o `s3`.
- `FILESYSTEM_DISK=s3` y `AWS_*`: endpoint, bucket, región, claves y path style.
- `conversion.*`: ancho/alto máximos, DPI, formato y calidad.
- `frontend.*`: habilitación, autenticación y ruta del bundle.

La UI muestra una sola Mongo URI para MongoDB y conserva contraseñas existentes cuando el campo secreto se deja vacío. Las respuestas administrativas devuelven máscaras e indicadores `*_configured`, no secretos.

## IIIF y TOC

```bash
curl -f http://127.0.0.1:18080/api/iiif/{document_id}/manifest
curl -f 'http://127.0.0.1:18080/api/iiif/{document_id}/manifest?pages=1-5,8,10-12'
curl -f http://127.0.0.1:18080/iiif/2/{document_id}_page_1/info.json
curl -f -o page.jpg http://127.0.0.1:18080/iiif/2/{document_id}_page_1/full/600,/0/default.jpg
```

Si el PDF no contiene outline, `structures` se omite. Si contiene bookmarks, cada entrada conserva su label y página; los bookmarks anidados se representan como ranges anidados. Una selección inválida devuelve HTTP 400.

## S3 / RustFS

Con `binary_storage.mode: s3`, los metadatos y los binarios permanecen desacoplados. Para comprobar el cliente contra el endpoint configurado:

```bash
cd backend
CONFIG_PATH=config.yaml go run ./cmd/s3-smoke
```

El smoke test crea un objeto temporal, lo lee y lo elimina. No borra documentos del proyecto.

Cada elemento de `projects.items` admite `bulk_upload: true`. Esta opción solo se aplica con S3/RustFS: convierte primero todas las páginas en `binary_storage.temp_path` y después las sube en paralelo. `security.max_concurrent_uploads` limita globalmente las cargas simultáneas de todos los documentos; su valor debe estar entre 1 y 100 y requiere reiniciar el servicio para cambiar el límite. Sin `bulk_upload`, las imágenes se almacenan secuencialmente. El PDF original se guarda al inicio en ambos modos.

La carga masiva necesita espacio temporal suficiente para todas las imágenes del PDF. Si una página no puede convertirse, escribirse temporalmente o subirse, el documento termina con estado `error`, conserva las páginas correctas, limpia los temporales y no inicia el OCR automático.

## Migraciones

`AUTO_MIGRATE=true` aplica migraciones SQL al iniciar en Docker. El migrador admite origen local, base activa y SSH, y destino MySQL/PostgreSQL/MongoDB más S3. Para SSH es obligatorio `MIGRATION_SOURCE_SSH_KNOWN_HOSTS`; no se aceptan hosts sin verificar.

## API y despliegue

- Swagger UI: `/swagger/index.html`.
- Especificación: `backend/docs/swagger.json` y `backend/docs/swagger.yaml`.
- Servicio Linux: [docs/INSTALL_SERVICE.md](docs/INSTALL_SERVICE.md).
- Docker: [docs/DOCKER.md](docs/DOCKER.md).
- Avisos de actualización: `deploy/project-iiif-check-update` y las unidades systemd asociadas consultan diariamente `origin/master`, escriben el resultado en `journalctl` y nunca instalan cambios automáticamente. Consulta la sección 21 del manual del servicio Linux.
- Autocompletado OCR: `GET /api/v1/ocr/autocomplete?q=func&project=default&limit=10` sugiere palabras reales por prefijo, sin duplicados y con normalización de mayúsculas y acentos. La ruta administrativa equivalente requiere sesión; consulta [docs/OCR.md](docs/OCR.md).

## Pruebas

```bash
cd backend && go test ./...
cd backend && go test -race ./...
cd backend/frontend && pnpm run lint && pnpm test && pnpm run build
docker compose --profile mysql config --quiet
docker compose --profile postgres config --quiet
docker compose --profile mongodb config --quiet
```
