# IIIF PDF Server

Servidor en Go para convertir archivos PDF a imágenes y publicarlas usando IIIF Image API v3. Incluye dashboard administrativo, soporte de proyectos y tenants, y almacenamiento en filesystem local, MySQL, PostgreSQL o MongoDB.

## Características

- Conversión de PDF a imágenes JPEG, PNG y WebP.
- API IIIF Image v3 y manifiestos IIIF Presentation v3.
- Dashboard web con login, configuración editable y monitoreo de migraciones.
- Soporte para proyectos, tenants y separación multitenant.
- Almacenamiento binario local, en BLOB de MySQL/PostgreSQL o en GridFS de MongoDB.
- Swagger / OpenAPI para documentación de endpoints.
- Migración de histórico local o remoto por SSH hacia la base de datos activa.

## Requisitos

En Ubuntu o Debian:

```bash
sudo apt-get update
sudo apt-get install -y build-essential libmupdf-dev
```

Para desarrollo también necesitas Go. Si vas a trabajar el dashboard con Tailwind, instala Node.js y npm.

## Instalación rápida

1. Clona el repositorio.
2. Entra a `backend/`.
3. Descarga dependencias.
4. Crea `config.yaml` desde `config.yaml.example`.
5. Compila y ejecuta.

```bash
git clone <tu-repo>
cd project_iiif_remote/backend
cp config.yaml.example config.yaml
go mod download
go build -o iiif-server main.go
./iiif-server
```

## Configuración

El servidor lee `backend/config.yaml`. Los campos principales son:

- `STORAGE_BACKEND`: `local`, `mysql`, `postgres` o `mongodb`.
- `DB_CONNECTION`: motor activo para metadatos.
- `storage.*`: rutas locales.
- `binary_storage.mode`: `local` o `database`.
- `frontend.*`: dashboard y autenticación.
- `projects.*`: configuración de proyectos y tenants.

### Modo local

Usa filesystem para metadatos y binarios:

```yaml
STORAGE_BACKEND: "local"
DB_CONNECTION: "local"

storage:
  backend: "local"
  data_path: "./data"
  pdfs_path: "./data/pdfs"
  images_path: "./data/images"
  documents_path: "./data/documents"
  thumbnails_path: "./data/thumbnails"
  manifests_path: "./data/manifests"

binary_storage:
  mode: "local"
  temp_path: "./data/temp"
```

### Modo MySQL o PostgreSQL

Usa la base de datos para metadatos y BLOB cuando `binary_storage.mode` sea `database`.

```yaml
STORAGE_BACKEND: "mysql"
DB_CONNECTION: "mysql"
DB_HOST: "127.0.0.1"
DB_PORT: "3306"
DB_DATABASE: "project_iiif"
DB_USERNAME: "project_iiif"
DB_PASSWORD: "CAMBIAR_PASSWORD"

binary_storage:
  mode: "database"
  temp_path: "./data/temp"
```

### Modo MongoDB

Usa MongoDB para metadatos y GridFS para PDFs e imágenes cuando `binary_storage.mode` sea `database`.

```yaml
STORAGE_BACKEND: "mongodb"
DB_CONNECTION: "mongodb"
DB_HOST: "127.0.0.1"
DB_PORT: "27017"
DB_DATABASE: "project_iiif"
DB_USERNAME: ""
DB_PASSWORD: ""

database:
  mongodb:
    host: "127.0.0.1"
    port: "27017"
    user: ""
    password: ""
    database: "project_iiif"
    auth_source: "admin"
    direct_connection: true
    server_selection_timeout_ms: 2000

binary_storage:
  mode: "database"
  temp_path: "./data/temp"
```

Notas importantes para MongoDB:

- El dashboard ahora muestra una sola caja de `Mongo URI` cuando eliges `mongodb`.
- Está pensado para pegar la misma URI que usas en MongoDB Compass.
- Actualmente el backend soporta URIs `mongodb://...`.
- Actualmente no soporta `mongodb+srv://...`.
- Si tu servidor Mongo no usa autenticación, deja `DB_USERNAME` y `DB_PASSWORD` vacíos.

Ejemplos válidos:

```text
mongodb://127.0.0.1:27017/project_iiif
mongodb://usuario:password@127.0.0.1:27017/project_iiif?authSource=admin
```

## Dashboard

Activa el panel así:

```yaml
frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
```

Rutas relevantes:

- `/`: bienvenida pública.
- `/dashboard`: dashboard autenticado.
- `/swagger/index.html`: documentación OpenAPI autenticada.

Funciones del dashboard:

- Subida de PDFs.
- Galería de imágenes IIIF.
- Edición segura de `config.yaml`.
- Reinicio asistido del servicio.
- Migración local o remota por SSH.

## Proyectos y multitenant

```yaml
projects:
  enabled: true
  default_project: "default"
  require_project: false
  allow_dynamic_tenants: false
  items:
    - key: "default"
      name: "Proyecto por defecto"
      multitenant: false
      tenants: []
    - key: "metavisor"
      name: "Metavisor"
      multitenant: true
      tenants:
        - "sunat"
        - "demo"
        - "uniguajira"
```

`POST /api/v1/documents/upload` acepta `project` y `tenant` por `form-data` o headers `X-IIIF-Project` y `X-IIIF-Tenant`.

## Migración local o SSH hacia la base de datos activa

El migrador sirve para copiar histórico desde filesystem local o desde un servidor remoto por SSH hacia el motor activo.

```bash
cd backend
go run ./cmd/migrate-local-to-mysql
```

Aunque el comando histórico mantiene el nombre `migrate-local-to-mysql`, hoy soporta:

- MySQL
- PostgreSQL
- MongoDB

Comportamiento:

- Migra documentos e imágenes.
- Migra binarios a BLOB o GridFS según el motor activo.
- Es idempotente.
- No borra archivos locales al finalizar.

Para habilitar migración desde el dashboard:

```yaml
migration:
  enabled: true
  allowed_local_roots:
    - "./data"
    - "/var/lib/project_iiif"
  max_log_lines: 1000
  ssh:
    connect_timeout_sec: 15
    allowed_hosts: []
```

## Swagger / OpenAPI

Genera o actualiza la documentación:

```bash
cd backend
swag init -g main.go -o docs
```

También puedes usar:

```bash
./scripts/generate-swagger.sh
```

En PowerShell:

```powershell
.\scripts\generate-swagger.ps1
```

## Frontend y estilos

Para compilar estilos del dashboard:

```bash
cd backend/frontend
npm install
npm run build:css
```

Para desarrollo:

```bash
npm run watch:css
```

## Endpoints principales

Autenticación:

- `POST /auth/login`
- `POST /auth/logout`
- `GET /auth/me`

Documentos:

- `POST /api/v1/documents/upload`
- `GET /api/v1/documents`
- `GET /api/v1/documents/:id`
- `DELETE /api/v1/documents/:id`

Administración:

- `GET /api/v1/admin/config`
- `PUT /api/v1/admin/config`
- `GET /api/v1/admin/projects`
- `POST /api/v1/admin/service/restart`
- `GET /api/v1/admin/migrations/sources/local/browse`
- `POST /api/v1/admin/migrations/local-to-db/start`
- `GET /api/v1/admin/migrations/local-to-db/status`
- `POST /api/v1/admin/db/migrations/run`
- `GET /api/v1/admin/db/migrations/status`

IIIF:

- `GET /iiif/3/:identifier/info.json`
- `GET /iiif/3/:identifier/:region/:size/:rotation/:quality.:format`
- `GET /iiif/3/:identifier/default.jpg`
- `GET /api/iiif/:id/manifest`

## Estructura del proyecto

```text
backend/
├── cmd/
├── docs/
├── frontend/
├── internal/
├── migrations/
├── scripts/
├── config.yaml.example
├── go.mod
├── go.sum
└── main.go
```

## Documentación adicional

- [Docker](docs/DOCKER.md)
- [Instalación como servicio](docs/INSTALL_SERVICE.md)

## Troubleshooting

- Si el servicio no responde, revisa `/health` y los logs del proceso.
- Si MongoDB falla al arrancar, valida host, puerto, base y credenciales.
- Si usas MongoDB sin autenticación, deja usuario y contraseña vacíos.
- Si la migración muestra errores por documento, revisa el modal de progreso y los logs.
- Si Swagger no refleja cambios recientes, regenera `backend/docs`.
