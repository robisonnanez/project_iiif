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

### Almacenamiento S3 / RustFS

RustFS se usa como almacenamiento de binarios y la base seleccionada continúa siendo el catálogo de metadatos. Los PDFs e imágenes se guardan como objetos S3; `documents.pdf_path` y `document_images.image_path` almacenan referencias `s3://bucket/clave`. En MongoDB se reutilizan los campos `pdf_path` e `image_path` de las colecciones actuales.

```yaml
FILESYSTEM_DISK: "s3"
AWS_ACCESS_KEY_ID: "TU_ACCESS_KEY_DE_RUSTFS"
AWS_SECRET_ACCESS_KEY: "TU_SECRET_KEY_DE_RUSTFS"
AWS_DEFAULT_REGION: "us-east-1"
AWS_BUCKET: "mi-proyecto"
AWS_ENDPOINT: "http://127.0.0.1:9000"
AWS_USE_PATH_STYLE_ENDPOINT: true

binary_storage:
  mode: "s3"
  temp_path: "./data/temp"
```

La estructura de objetos es:

```text
projects/{project}/documents/{document_id}/document.pdf
projects/{project}/documents/{document_id}/images/page_{page}_{image_id}.{format}
projects/{project}/tenants/{tenant}/documents/...
```

Al iniciar, el servidor valida RustFS y crea el bucket configurado si todavía no existe. Puedes ejecutar una prueba no destructiva de escritura, lectura y eliminación:

```bash
cd backend
go run ./cmd/s3-smoke
```

#### Relación entre la base de datos, S3 e IIIF

S3 no reemplaza la base de datos. Cada componente tiene una responsabilidad distinta:

| Componente | Responsabilidad |
| --- | --- |
| MySQL, PostgreSQL o MongoDB | Guarda documentos, páginas, dimensiones, estado, proyecto, tenant y las referencias de almacenamiento. |
| RustFS / S3 | Guarda los bytes de los PDF y de las imágenes. |
| IIIF | Consulta los metadatos, descarga la imagen desde S3 y entrega la representación solicitada. |

Cuando `binary_storage.mode` es `s3`, los campos contienen referencias como estas:

```text
documents.pdf_path = s3://project-iiif/projects/default/tenants/sunat/documents/{document_id}/document.pdf
document_images.image_path = s3://project-iiif/projects/default/tenants/sunat/documents/{document_id}/images/page_000001_{image_id}.jpg
```

Por lo tanto, la galería de imágenes sí usa `document_images.image_path`. El recorrido de una solicitud es:

```text
Navegador -> URL IIIF -> registro document_images -> image_path s3://... -> RustFS -> transformación IIIF -> JPEG/PNG/WebP
```

La URL IIIF no expone credenciales de RustFS ni redirige al navegador hacia el endpoint S3. El backend realiza la lectura internamente.

Puedes comprobar las referencias en MySQL con:

```sql
SELECT id, original_name, pdf_path
FROM documents;

SELECT id, document_id, page_number, image_path
FROM document_images
ORDER BY document_id, page_number;
```

Pruebas rápidas de IIIF:

```bash
curl -f http://127.0.0.1:8080/iiif/3/{document_id}_page_1/info.json
curl -f -o pagina.jpg http://127.0.0.1:8080/iiif/3/{document_id}_page_1/full/600,/0/default.jpg
curl -f http://127.0.0.1:8080/api/iiif/{document_id}/manifest
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

- Subida de PDFs con selección previa de dimensiones, DPI, formato y calidad.
- Galería de imágenes IIIF.
- Edición segura de `config.yaml`.
- Reinicio asistido del servicio.
- Migración local o remota por SSH.

En **Configuración**, `Backend de metadatos` y `Modo binario` son opciones diferentes:

- `Backend de metadatos`: `local`, `mysql`, `postgres` o `mongodb`.
- `Modo binario`: `local`, `database` o `s3`.
- Para RustFS, selecciona la base que guardará los registros y después selecciona `s3` como modo binario.
- Los campos `S3 / RustFS` solo se habilitan cuando el modo binario es `s3`.
- MySQL y PostgreSQL muestran los campos SQL; MongoDB muestra únicamente `Mongo URI`.

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

### Resolución de las imágenes al subir un PDF

Al seleccionar un PDF, el dashboard solicita la configuración antes de iniciar la conversión. Los valores predeterminados son:

| Campo | Valor |
| --- | --- |
| Ancho máximo | `1241 px` |
| Alto máximo | `1754 px` |
| Resolución | `150 DPI` |
| Formato | `JPG` |
| Calidad JPG | `85` |

La imagen conserva la proporción original y se ajusta dentro de esos límites; no se estira para ocupar ambas dimensiones. `1241 × 1754 px` a `150 DPI` es adecuado para visualización web e IIIF. Por ejemplo, una página Carta se genera aproximadamente a `1241 × 1606 px`.

La API recibe estos campos como `multipart/form-data`:

```text
max_width=1241
max_height=1754
dpi=150
format=jpg
quality=85
```

Los límites aceptados son `256-8192 px`, `72-600 DPI` y calidad `1-100`. La configuración usada queda registrada con el documento en MySQL, PostgreSQL o MongoDB.

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
- S3 / RustFS como destino binario

Comportamiento:

- Migra documentos e imágenes.
- Migra binarios a BLOB o GridFS según el motor activo.
- Con `binary_storage.mode: s3`, migra los binarios a RustFS y conserva metadatos en la base activa.
- Es idempotente.
- No borra archivos locales al finalizar.

### Prueba limpia de migración a S3

En un entorno de pruebas puedes vaciar únicamente los datos documentales y conservar `schema_migrations`:

```sql
SET FOREIGN_KEY_CHECKS = 0;
TRUNCATE TABLE document_images;
TRUNCATE TABLE documents;
SET FOREIGN_KEY_CHECKS = 1;
```

Después ejecuta el migrador con `binary_storage.mode: s3`. Una migración verificada en el entorno de prueba produjo 3 PDF y 939 imágenes, todas con referencias `s3://project-iiif/...`, y IIIF devolvió correctamente `info.json`, el manifiesto y una imagen JPEG.

> `TRUNCATE` elimina los registros existentes. Úsalo únicamente cuando quieras reiniciar una prueba y tengas respaldo si los datos son importantes.

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

El manifiesto usa IIIF Presentation 3. Sin parámetros incluye todas las páginas. Para generar una vista parcial, usa `pages` con números y rangos separados por comas:

```text
GET /api/iiif/{document_id}/manifest?pages=1-5,8,10-12
```

- Las páginas se validan contra `total_pages`.
- Los duplicados se eliminan y la respuesta queda ordenada.
- `pages=all` equivale a omitir el parámetro.
- Una selección inválida devuelve HTTP `400`.
- La selección no vuelve a convertir ni duplica imágenes; únicamente determina qué canvases aparecen en el manifiesto.

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

Los documentos migrados con estado `completed` también muestran `Generar manifest` en el dashboard. La acción crea la respuesta IIIF dinámicamente desde las imágenes registradas, aunque el documento no tuviera una URL de manifiesto guardada previamente.
