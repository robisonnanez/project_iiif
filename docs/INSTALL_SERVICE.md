# Instalación del servicio Project IIIF

Esta guía explica cómo instalar Project IIIF como servicio `systemd` en Linux. Cubre los tres modos principales:

- local
- MySQL / PostgreSQL
- MongoDB

## 1. Dependencias

En Ubuntu o Debian:

```bash
sudo apt-get update
sudo apt-get install -y build-essential libmupdf-dev git curl
```

Si vas a usar MySQL:

```bash
sudo apt-get install -y mysql-server mysql-client
```

Si vas a usar MongoDB, instala tu versión objetivo y verifica que el servicio responda:

```bash
sudo systemctl status mongod
```

Instala Go y asegúrate de tenerlo en el `PATH`:

```bash
export PATH=$PATH:/usr/local/go/bin
go version
```

Si necesitas Swagger:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
export PATH=$PATH:$(go env GOPATH)/bin
```

## 2. Estructura recomendada

```bash
sudo mkdir -p /opt/project_iiif
sudo mkdir -p /etc/project_iiif
sudo mkdir -p /var/lib/project_iiif/{pdfs,images,documents,thumbnails,manifests,temp}
sudo chown -R robison:robison /opt/project_iiif /var/lib/project_iiif
```

Clona o copia el proyecto en:

```text
/opt/project_iiif
```

El backend debe quedar en:

```text
/opt/project_iiif/backend
```

## 3. Base de datos según el motor

### MySQL

```bash
sudo mysql
```

```sql
CREATE DATABASE IF NOT EXISTS project_iiif CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'project_iiif'@'localhost' IDENTIFIED BY 'PASSWORD_SEGURO';
GRANT ALL PRIVILEGES ON project_iiif.* TO 'project_iiif'@'localhost';
FLUSH PRIVILEGES;
EXIT;
```

Ejecuta las migraciones SQL:

```bash
cd /opt/project_iiif/backend
mysql -u project_iiif -p project_iiif < migrations/001_create_documents.sql
mysql -u project_iiif -p project_iiif < migrations/002_add_blob_storage.sql
mysql -u project_iiif -p project_iiif < migrations/003_add_projects_multitenant.sql
```

### PostgreSQL

Crea la base, el usuario y los permisos equivalentes según tu instalación. Luego aplica las migraciones que correspondan si tu flujo las requiere.

### MongoDB

Si usas MongoDB sin autenticación, basta con tener el servicio activo y la base accesible:

```bash
sudo systemctl status mongod
mongosh --eval 'db.runCommand({ ping: 1 })'
```

Si usas autenticación, crea un usuario con permisos sobre la base `project_iiif` o la que definas en la configuración.

Importante:

- si MongoDB no usa autenticación, deja usuario y contraseña vacíos
- si MongoDB usa autenticación, completa `DB_USERNAME`, `DB_PASSWORD` y `auth_source`
- el backend actual soporta URIs `mongodb://...`
- el backend actual no soporta `mongodb+srv://...`

## 4. Configuración

Crea el archivo activo:

```bash
sudo cp /opt/project_iiif/backend/config.yaml.example /etc/project_iiif/config.yaml
sudo nano /etc/project_iiif/config.yaml
```

### Modo local

```yaml
STORAGE_BACKEND: "local"
DB_CONNECTION: "local"

storage:
  backend: "local"
  data_path: "/var/lib/project_iiif"
  pdfs_path: "/var/lib/project_iiif/pdfs"
  images_path: "/var/lib/project_iiif/images"
  documents_path: "/var/lib/project_iiif/documents"
  thumbnails_path: "/var/lib/project_iiif/thumbnails"
  manifests_path: "/var/lib/project_iiif/manifests"

binary_storage:
  mode: "local"
  temp_path: "/var/lib/project_iiif/temp"
```

### Modo MySQL

```yaml
STORAGE_BACKEND: "mysql"
DB_CONNECTION: "mysql"
DB_HOST: "127.0.0.1"
DB_PORT: "3306"
DB_DATABASE: "project_iiif"
DB_USERNAME: "project_iiif"
DB_PASSWORD: "PASSWORD_SEGURO"

storage:
  backend: "mysql"
  data_path: "/var/lib/project_iiif"

binary_storage:
  mode: "database"
  temp_path: "/var/lib/project_iiif/temp"
```

### Modo MongoDB

```yaml
STORAGE_BACKEND: "mongodb"
DB_CONNECTION: "mongodb"
DB_HOST: "127.0.0.1"
DB_PORT: "27017"
DB_DATABASE: "project_iiif"
DB_USERNAME: ""
DB_PASSWORD: ""

storage:
  backend: "mongodb"
  data_path: "/var/lib/project_iiif"

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
  temp_path: "/var/lib/project_iiif/temp"
```

### Binarios en RustFS / S3

Esta configuración puede combinarse con MySQL, PostgreSQL, MongoDB o metadata local:

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
  temp_path: "/var/lib/project_iiif/temp"
```

Verifica las operaciones S3 antes de reiniciar el servicio:

```bash
cd /opt/project_iiif/backend
go run ./cmd/s3-smoke
```

La migración desde BLOB/GridFS hacia RustFS está disponible en el dashboard seleccionando `Base de datos activa (BLOB/GridFS) a S3`.

#### Cómo se leen los documentos y las imágenes

La base activa continúa siendo el catálogo. En MySQL y PostgreSQL se utilizan los campos `documents.pdf_path` y `document_images.image_path`; en MongoDB se usan los campos equivalentes de sus colecciones. Con S3 activo, su valor es una referencia `s3://bucket/clave`, no el contenido binario.

```text
documents.pdf_path
  -> s3://project-iiif/projects/default/tenants/sunat/documents/{document_id}/document.pdf

document_images.image_path
  -> s3://project-iiif/projects/default/tenants/sunat/documents/{document_id}/images/page_000001_{image_id}.jpg
```

Cuando se abre una imagen en el dashboard, el endpoint IIIF busca el registro por documento/página o identificador, obtiene `image_path`, descarga el objeto desde RustFS y devuelve la imagen transformada. El navegador no necesita acceso directo al bucket ni conoce las credenciales S3.

Verifica el flujo completo después de instalar:

```bash
curl -f http://127.0.0.1:8080/health
curl -f http://127.0.0.1:8080/iiif/3/{document_id}_page_1/info.json
curl -f -o /tmp/pagina.jpg \
  http://127.0.0.1:8080/iiif/3/{document_id}_page_1/full/600,/0/default.jpg
file /tmp/pagina.jpg
```

El resultado final debe ser HTTP `200` y un archivo `image/jpeg`.

#### Selección en el dashboard

- En `Backend de metadatos`, selecciona MySQL, PostgreSQL, MongoDB o local. S3 no aparece aquí porque no almacena el catálogo.
- En `Modo binario`, selecciona `s3` para guardar PDF e imágenes en RustFS.
- Al seleccionar `s3`, se habilitan endpoint, región, bucket, access key, secret key y path-style.
- Para MySQL/PostgreSQL se muestran host, puerto, base, usuario y contraseña.
- Para MongoDB se ocultan los campos SQL y se muestra únicamente `Mongo URI`.

#### Reiniciar una prueba de migración en MySQL

Si necesitas empezar desde cero, conserva la tabla `schema_migrations` y vacía solamente las tablas documentales:

```sql
SET FOREIGN_KEY_CHECKS = 0;
TRUNCATE TABLE document_images;
TRUNCATE TABLE documents;
SET FOREIGN_KEY_CHECKS = 1;
```

Esta operación elimina los registros actuales. No la ejecutes en producción sin respaldo.

Si usas autenticación en MongoDB:

```yaml
DB_USERNAME: "usuario_mongo"
DB_PASSWORD: "PASSWORD_SEGURA"

database:
  mongodb:
    user: "usuario_mongo"
    password: "PASSWORD_SEGURA"
    auth_source: "admin"
```

### Dashboard

```yaml
frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
```

### Proyectos y tenants

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

### Migración desde dashboard

```yaml
migration:
  enabled: true
  allowed_local_roots:
    - "/var/lib/project_iiif"
    - "/home/robison/projects/project_iiif/backend/data"
  max_log_lines: 1000
  ssh:
    connect_timeout_sec: 15
    allowed_hosts: []
```

## 5. Compilación de producción

```bash
cd /opt/project_iiif/backend
export PATH=$PATH:/usr/local/go/bin
go mod download
CGO_ENABLED=1 go build -ldflags="-s -w" -o iiif-server main.go
CGO_ENABLED=1 go build -ldflags="-s -w" -o migrate-local-to-mysql ./cmd/migrate-local-to-mysql
```

Genera Swagger antes de desplegar:

```bash
cd /opt/project_iiif/backend
swag init -g main.go -o docs
```

Si vas a compilar estilos:

```bash
cd /opt/project_iiif/backend/frontend
npm install
npm run build:css
```

## 6. Enlace al archivo de configuración

El binario busca `config.yaml` dentro del `WorkingDirectory`. Crea el enlace:

```bash
ln -sf /etc/project_iiif/config.yaml /opt/project_iiif/backend/config.yaml
sudo chown -R robison:robison /var/lib/project_iiif /opt/project_iiif/backend/config.yaml
```

## 7. Servicio systemd

Copia el unit file:

```bash
sudo cp /opt/project_iiif/deploy/project-iiif.service /etc/systemd/system/project-iiif.service
```

Ejemplo:

```ini
[Unit]
Description=Project IIIF PDF Server
After=network.target

[Service]
Type=simple
User=robison
Group=robison
WorkingDirectory=/opt/project_iiif/backend
Environment=PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=/opt/project_iiif/backend/iiif-server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Si dependes de MySQL o MongoDB, puedes ampliar `After=` y `Wants=` según tu entorno.

Activa el servicio:

```bash
sudo systemctl daemon-reload
sudo systemctl enable project-iiif
sudo systemctl start project-iiif
sudo systemctl status project-iiif
```

Comandos útiles:

```bash
sudo systemctl stop project-iiif
sudo systemctl restart project-iiif
journalctl -u project-iiif -f
```

## 8. Pruebas rápidas

```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/documents
curl -I http://localhost:8080/dashboard
```

Con sesión iniciada, valida Swagger:

```bash
curl -I http://localhost:8080/swagger/index.html
```

## 9. Migración de histórico

El ejecutable mantiene el nombre histórico `migrate-local-to-mysql`, pero actualmente funciona con el motor activo, incluido MongoDB.

```bash
cd /opt/project_iiif/backend
./migrate-local-to-mysql
```

Si el motor activo es MongoDB:

- los documentos se guardan en la colección `documents`
- las imágenes se guardan en `document_images`
- los PDFs se guardan en GridFS `pdfs`
- las imágenes binarias se guardan en GridFS `images`

## 10. Troubleshooting

- Si el servicio no inicia, revisa `journalctl -u project-iiif -n 100`.
- Si falta Go, revisa `Environment=PATH=.../usr/local/go/bin...`.
- Si falla MuPDF, confirma `libmupdf-dev` y recompila con `CGO_ENABLED=1`.
- Si MySQL falla, valida usuario, contraseña, base y migraciones.
- Si MongoDB falla, valida host, puerto, credenciales y `auth_source`.
- Si MongoDB no tiene autenticación, deja usuario y contraseña vacíos.
- Si el dashboard muestra errores por documento durante la migración, revisa el modal de progreso y los logs.
- Si cambias configuración desde el dashboard, puedes reiniciar el servicio desde el modal o con `systemctl restart project-iiif`.
- Si Swagger no refleja cambios recientes, regenera `backend/docs` con `swag init -g main.go -o docs`.
## Actualización para dimensiones y DPI configurables

Después de desplegar esta versión, aplica las migraciones de base de datos antes de reiniciar el binario. Se agregan los campos `conversion_width`, `conversion_height`, `conversion_dpi`, `conversion_format` y `conversion_quality` a `documents` para MySQL/PostgreSQL. MongoDB no requiere una migración estructural.

La configuración predeterminada del formulario es `1241 × 1754 px` a `150 DPI`. El render conserva la proporción de cada página y registra tiempos agregados de render, redimensionamiento, codificación y almacenamiento en el log del servicio.

```bash
sudo systemctl restart project-iiif
sudo systemctl status project-iiif --no-pager
journalctl -u project-iiif -n 100 --no-pager
```

En **Documentos**, cualquier registro con estado `completed`, incluidos los migrados a S3/RustFS, dispone del botón **Generar manifest**.
