# Instalación Linux con systemd

Esta guía instala `project_iiif` como servicio sin reemplazar configuraciones ni datos existentes.

## 1. Usuario y directorios

```bash
sudo useradd --system --home /var/lib/project_iiif --shell /usr/sbin/nologin project-iiif
sudo install -d -o project-iiif -g project-iiif -m 0750 \
  /opt/project_iiif /etc/project_iiif /var/lib/project_iiif \
  /var/lib/project_iiif/{pdfs,images,documents,thumbnails,manifests,temp}
```

En una instalación existente, primero registra propietario/permisos y no ejecutes `chown -R` a ciegas.

## 2. Compilar artefactos

Desde un checkout de release:

```bash
cd /home/usuario/projects/project_iiif/backend/frontend
corepack enable
pnpm install --frozen-lockfile
pnpm run lint
pnpm test
pnpm run build

cd /home/usuario/projects/project_iiif/backend
go test ./...
CGO_ENABLED=1 go build -trimpath -o project-iiif .
CGO_ENABLED=1 go build -trimpath -o s3-smoke ./cmd/s3-smoke
CGO_ENABLED=1 go build -trimpath -o migrate-local-to-db ./cmd/migrate-local-to-mysql
```

Instala en un directorio versionado para permitir rollback:

```bash
release="$(git rev-parse --short HEAD)"
sudo install -d -o root -g root -m 0755 "/opt/project_iiif/releases/$release"
sudo install -o root -g root -m 0755 project-iiif s3-smoke migrate-local-to-db \
  "/opt/project_iiif/releases/$release/"
sudo cp -a frontend/dist migrations "/opt/project_iiif/releases/$release/"
sudo ln -sfn "/opt/project_iiif/releases/$release" /opt/project_iiif/current
```

## 3. Configuración y secretos

Si no existe configuración activa:

```bash
sudo install -o root -g project-iiif -m 0640 \
  config.yaml.example /etc/project_iiif/config.yaml
```

Si ya existe, respáldala antes de editar:

```bash
sudo cp -a /etc/project_iiif/config.yaml \
  "/etc/project_iiif/config.yaml.$(date +%Y%m%d-%H%M%S).bak"
```

Configura `STORAGE_BACKEND`/`DB_CONNECTION` para MySQL, PostgreSQL o MongoDB. Para RustFS/S3 usa `binary_storage.mode: s3`, endpoint, región, bucket y path style. Prefiere un archivo root-only para secretos:

```bash
sudo install -o root -g project-iiif -m 0640 /dev/null /etc/project_iiif/project-iiif.env
sudoedit /etc/project_iiif/project-iiif.env
```

```dotenv
CONFIG_PATH=/etc/project_iiif/config.yaml
DB_PASSWORD=CAMBIAR
AWS_ACCESS_KEY_ID=CAMBIAR
AWS_SECRET_ACCESS_KEY=CAMBIAR
FRONTEND_PASSWORD=CAMBIAR
IIIF_BASE_URL=https://iiif.example.org
AUTO_MIGRATE=true
```

## 4. Bases de datos y migraciones

Crea la base y usuario con privilegios mínimos. `AUTO_MIGRATE=true` aplica versiones pendientes al iniciar. Para revisión manual consulta `backend/migrations`; no vuelvas a ejecutar scripts sin comprobar `schema_migrations`.

`005_add_conversion_settings.sql` conserva parámetros de conversión y `006_add_pdf_outline.sql` añade el outline. MongoDB usa inicialización idempotente de colecciones e índices.

## 5. RustFS / S3

RustFS puede ejecutarse en otro host o como servicio separado. Antes del cambio de producción:

```bash
sudo -u project-iiif env \
  CONFIG_PATH=/etc/project_iiif/config.yaml \
  /opt/project_iiif/current/s3-smoke
```

El smoke crea, lee y elimina solo un objeto temporal. No expongas consola ni API S3 públicamente sin TLS, autenticación y controles de red.

## 6. Unidad systemd

`/etc/systemd/system/project-iiif.service`:

```ini
[Unit]
Description=Project IIIF PDF server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=project-iiif
Group=project-iiif
WorkingDirectory=/opt/project_iiif/current
EnvironmentFile=/etc/project_iiif/project-iiif.env
ExecStart=/opt/project_iiif/current/project-iiif
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/project_iiif /etc/project_iiif
UMask=0027

[Install]
WantedBy=multi-user.target
```

Si la UI guarda configuración, `/etc/project_iiif` debe ser escribible por el servicio; alternativamente ubica el YAML mutable bajo `/var/lib/project_iiif`.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now project-iiif
sudo systemctl status project-iiif --no-pager
curl -f http://127.0.0.1:8080/health
```

## 7. Logs y operación

```bash
sudo journalctl -u project-iiif -n 200 --no-pager
sudo journalctl -u project-iiif -f
sudo systemctl restart project-iiif
```

No registres archivos de entorno ni configuración con secretos. Las respuestas administrativas deben mostrar máscaras e indicadores de secreto configurado.

## 8. Actualización segura

1. Registra `readlink -f /opt/project_iiif/current` y `git rev-parse HEAD`.
2. Respalda configuración, base y bucket/objetos.
3. Compila y prueba en el checkout, no en `/opt`.
4. Instala un nuevo directorio `releases/<commit>`.
5. Cambia el symlink y reinicia.
6. Comprueba health, login, documento existente, manifest e imagen.

```bash
sudo ln -sfn /opt/project_iiif/releases/NUEVO /opt/project_iiif/current
sudo systemctl restart project-iiif
curl -f http://127.0.0.1:8080/health
```

## 9. Rollback básico

```bash
sudo ln -sfn /opt/project_iiif/releases/ANTERIOR /opt/project_iiif/current
sudo systemctl restart project-iiif
```

Las migraciones de esquema/datos requieren su propio plan de reversión o restauración. No borres tablas, volúmenes ni buckets para hacer rollback.

## 10. Migración local/base/SSH

El dashboard inicia migraciones hacia la base activa/S3. Para origen SSH proporciona clave privada y un archivo `known_hosts` legible. Es obligatorio:

```dotenv
MIGRATION_SOURCE_TYPE=ssh
MIGRATION_SOURCE_SSH_HOST=origen.example.org
MIGRATION_SOURCE_SSH_PORT=22
MIGRATION_SOURCE_SSH_USER=usuario
MIGRATION_SOURCE_SSH_PATH=/ruta/datos
MIGRATION_SOURCE_SSH_PRIVATE_KEY=...
MIGRATION_SOURCE_SSH_KNOWN_HOSTS=/etc/project_iiif/known_hosts
```

Genera `known_hosts` desde una fuente confiable y verifica el fingerprint fuera de banda. El migrador no usa `InsecureIgnoreHostKey`.

## 11. Comprobación final

```bash
curl -f http://127.0.0.1:8080/health
curl -f http://127.0.0.1:8080/api/v1/documents/{id}
curl -f http://127.0.0.1:8080/api/iiif/{id}/manifest
curl -f -o /tmp/page.jpg \
  http://127.0.0.1:8080/iiif/2/{id}_page_1/full/600,/0/default.jpg
file /tmp/page.jpg
```

Antes de modificar una instalación en `/opt/project_iiif`, revisa `git status`, servicio, procesos, versión y diferencias de configuración. Si hay cambios locales no identificados, detente y coordina su respaldo en vez de sobrescribirlos.
