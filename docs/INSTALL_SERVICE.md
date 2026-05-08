# Instalacion del servicio Project IIIF

Esta guia instala el servidor Go como servicio `systemd` global y deja MySQL como backend de metadata. Los PDFs e imagenes convertidas se guardan en disco local.

## 1. Dependencias

```bash
sudo apt-get update
sudo apt-get install -y build-essential libmupdf-dev mysql-server mysql-client
export PATH=$PATH:/usr/local/go/bin
go version
```

Si `go version` no responde, instala Go en `/usr/local/go` y agrega esta linea a `~/.bashrc`:

```bash
export PATH=$PATH:/usr/local/go/bin
```

## 2. Base de datos

Crea una base y un usuario dedicado. Cambia `PASSWORD_SEGURO` por una clave real.

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

Ejecuta las migraciones:

```bash
cd /opt/project_iiif/backend
mysql -u project_iiif -p project_iiif < migrations/001_create_documents.sql
```

## 3. Configuracion

```bash
sudo mkdir -p /etc/project_iiif /var/lib/project_iiif/{pdfs,images,documents,thumbnails,manifests,temp}
sudo cp /opt/project_iiif/backend/config.yaml.example /etc/project_iiif/config.yaml
sudo nano /etc/project_iiif/config.yaml
```

Ajusta estos valores:

```yaml
storage:
  backend: "mysql"
  data_path: "/var/lib/project_iiif"
  pdfs_path: "/var/lib/project_iiif/pdfs"
  images_path: "/var/lib/project_iiif/images"
  documents_path: "/var/lib/project_iiif/documents"
  thumbnails_path: "/var/lib/project_iiif/thumbnails"
  manifests_path: "/var/lib/project_iiif/manifests"

pdf:
  temp_path: "/var/lib/project_iiif/temp"

database:
  mysql:
    host: "127.0.0.1"
    port: "3306"
    user: "project_iiif"
    password: "PASSWORD_SEGURO"
    database: "project_iiif"
    charset: "utf8mb4"
    parse_time: true
```

No guardes claves reales en el repositorio.

## 4. Build de produccion

```bash
cd /opt/project_iiif/backend
export PATH=$PATH:/usr/local/go/bin
go mod download
CGO_ENABLED=1 go build -ldflags="-s -w" -o iiif-server main.go
```

## 5. Servicio systemd

Copia el unit file incluido en el repositorio:

```bash
sudo cp /opt/project_iiif/deploy/project-iiif.service /etc/systemd/system/project-iiif.service
```

El binario busca `config.yaml` en el directorio de trabajo. Para usar `/etc/project_iiif/config.yaml`, crea un enlace:

```bash
ln -sf /etc/project_iiif/config.yaml /opt/project_iiif/backend/config.yaml
sudo chown -R robison:robison /var/lib/project_iiif /opt/project_iiif/backend/config.yaml
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

## 6. Pruebas rapidas

```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/documents
```

Despues de subir un PDF, consulta MySQL:

```sql
SELECT id, original_name, status, total_pages, converted_pages FROM documents;
SELECT id, document_id, page_number, image_path, width, height FROM document_images;
```

Prueba IIIF con el `id` de una fila en `document_images`:

```bash
curl -I http://localhost:8080/iiif/3/IMAGE_ID/info.json
curl -I http://localhost:8080/iiif/3/IMAGE_ID/0,0,200,200/max/0/default.jpg
curl -I http://localhost:8080/iiif/3/IMAGE_ID/full/800,/0/default.jpg
```

## Troubleshooting

- Si el servicio no inicia, revisa `journalctl -u project-iiif -n 100`.
- Si falta Go, confirma `Environment=PATH=.../usr/local/go/bin...` en el servicio.
- Si falla MuPDF, confirma `libmupdf-dev` y recompila con `CGO_ENABLED=1`.
- Si MySQL falla, revisa usuario, password, base de datos y que las migraciones hayan corrido.
- Si `config.yaml` no existe, copia `config.yaml.example` y ajusta rutas/credenciales.
