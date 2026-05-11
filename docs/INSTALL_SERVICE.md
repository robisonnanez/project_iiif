# Instalacion del servicio Project IIIF

Esta guia instala Project IIIF como servicio `systemd` global. El servidor convierte PDFs a imagenes, sirve IIIF v3, incluye dashboard opcional y puede trabajar en modo `local` o `mysql`.

## 1. Dependencias

En Ubuntu/Debian:

```bash
sudo apt-get update
sudo apt-get install -y build-essential libmupdf-dev mysql-server mysql-client git curl
export PATH=$PATH:/usr/local/go/bin
go version
```

Si `go version` no responde, instala Go en `/usr/local/go` y agrega esta linea a `~/.bashrc`:

```bash
export PATH=$PATH:/usr/local/go/bin
source ~/.bashrc
```

## 2. Estructura recomendada

```bash
sudo mkdir -p /opt/project_iiif
sudo mkdir -p /etc/project_iiif
sudo mkdir -p /var/lib/project_iiif/{pdfs,images,documents,thumbnails,manifests,temp}
sudo chown -R robison:robison /opt/project_iiif /var/lib/project_iiif
```

Copia o clona el proyecto en:

```bash
/opt/project_iiif
```

El backend debe quedar en:

```bash
/opt/project_iiif/backend
```

## 3. Base de datos MySQL

Crea una base y usuario dedicado. Cambia `PASSWORD_SEGURO` por una clave real.

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

Ejecuta todas las migraciones en orden:

```bash
cd /opt/project_iiif/backend
mysql -u project_iiif -p project_iiif < migrations/001_create_documents.sql
mysql -u project_iiif -p project_iiif < migrations/002_add_blob_storage.sql
mysql -u project_iiif -p project_iiif < migrations/003_add_projects_multitenant.sql
```

## 4. Configuracion

Crea el archivo activo desde el ejemplo:

```bash
sudo cp /opt/project_iiif/backend/config.yaml.example /etc/project_iiif/config.yaml
sudo nano /etc/project_iiif/config.yaml
```

Para modo MySQL con PDFs e imagenes en BLOB:

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

frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
```

Para modo local:

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

Para proyectos y multitenant:

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

No guardes claves reales en el repositorio.

## 5. Build de produccion

```bash
cd /opt/project_iiif/backend
export PATH=$PATH:/usr/local/go/bin
go mod download
CGO_ENABLED=1 go build -ldflags="-s -w" -o iiif-server main.go
```

El binario busca `config.yaml` en el `WorkingDirectory`. Para usar `/etc/project_iiif/config.yaml`, crea un enlace:

```bash
ln -sf /etc/project_iiif/config.yaml /opt/project_iiif/backend/config.yaml
sudo chown -R robison:robison /var/lib/project_iiif /opt/project_iiif/backend/config.yaml
```

## 6. Servicio systemd

Copia el unit file incluido:

```bash
sudo cp /opt/project_iiif/deploy/project-iiif.service /etc/systemd/system/project-iiif.service
```

Contenido esperado:

```ini
[Unit]
Description=Project IIIF PDF Server
After=network.target mysql.service
Wants=mysql.service

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

Activa el servicio:

```bash
sudo systemctl daemon-reload
sudo systemctl enable project-iiif
sudo systemctl start project-iiif
sudo systemctl status project-iiif
```

Comandos de operacion:

```bash
sudo systemctl stop project-iiif
sudo systemctl restart project-iiif
journalctl -u project-iiif -f
```

## 7. Pruebas rapidas

```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/documents
curl -I http://localhost:8080/dashboard
```

Despues de subir un PDF en modo MySQL:

```sql
SELECT id, original_name, project_key, tenant_key, status, total_pages, converted_pages, pdf_size FROM documents;
SELECT id, document_id, project_key, tenant_key, page_number, byte_size, media_type FROM document_images;
```

Prueba IIIF con el `id` de una fila en `document_images`:

```bash
curl -I http://localhost:8080/iiif/3/IMAGE_ID/info.json
curl -I http://localhost:8080/iiif/3/IMAGE_ID/full/max/0/default.jpg
curl -I http://localhost:8080/iiif/3/metavisor~sunat~IMAGE_ID/full/max/0/default.jpg
```

## 8. Troubleshooting

- Si el servicio no inicia, revisa `journalctl -u project-iiif -n 100`.
- Si falta Go, confirma `Environment=PATH=.../usr/local/go/bin...` en el servicio.
- Si falla MuPDF, confirma `libmupdf-dev` y recompila con `CGO_ENABLED=1`.
- Si MySQL falla, revisa usuario, password, base de datos y migraciones.
- Si `config.yaml` no existe, copia `config.yaml.example` y ajusta rutas/credenciales.
- Si cambias configuracion desde el dashboard, reinicia el servicio para aplicar cambios sensibles.
