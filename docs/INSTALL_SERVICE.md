# Manual de instalación y actualización de project_iiif en Linux

Guía para Debian 12 o Ubuntu Server 24.04. Instala el backend Go y el frontend React como servicio `systemd`, mantiene la configuración y los datos fuera del repositorio, permite migraciones controladas y conserva releases para rollback.

## 1. Arquitectura recomendada

```text
/opt/project_iiif/
  source/                 repositorio Git usado para compilar
  releases/<commit>/      artefactos inmutables de cada versión
  current -> releases/... release activo

/var/lib/project_iiif/
  config.yaml             configuración activa y secretos
  pdfs/ images/ documents/ thumbnails/ manifests/ temp/
```

El servicio nunca se ejecuta directamente desde `source`. Un `git pull` no modifica el release activo ni los documentos.

## 2. Requisitos

- Linux x86_64 o arm64.
- Go 1.24 o posterior.
- Node.js 22 y pnpm 10.
- Git, compilador C, certificados y curl.
- Tesseract 5 con `osd`, `eng`, `spa`, `fra` y `por` si se utilizará OCR.
- Una base creada previamente cuando se use MySQL, PostgreSQL o MongoDB.
- Acceso de salida a GitHub solamente para comprobar o descargar actualizaciones.

```bash
sudo apt update
sudo apt install -y git curl ca-certificates build-essential pkg-config \
  tesseract-ocr tesseract-ocr-osd tesseract-ocr-eng \
  tesseract-ocr-spa tesseract-ocr-fra tesseract-ocr-por

go version
node --version
corepack enable
corepack prepare pnpm@10.17.1 --activate
pnpm --version
tesseract --version
tesseract --list-langs
```

No continúe si Go es anterior a 1.24 o Node.js es anterior a 22.

## 3. Usuario y directorios

```bash
sudo useradd --system --home /var/lib/project_iiif \
  --shell /usr/sbin/nologin project-iiif

sudo install -d -o root -g root -m 0755 \
  /opt/project_iiif /opt/project_iiif/releases
sudo install -d -o root -g root -m 0755 /opt/project_iiif/source
sudo install -d -o project-iiif -g project-iiif -m 0750 \
  /var/lib/project_iiif \
  /var/lib/project_iiif/pdfs \
  /var/lib/project_iiif/images \
  /var/lib/project_iiif/documents \
  /var/lib/project_iiif/thumbnails \
  /var/lib/project_iiif/manifests \
  /var/lib/project_iiif/temp
```

No ejecute `chown -R` sobre una instalación existente sin revisar previamente los propietarios.

## 4. Clonar la rama estable

La rama estable es `master`. Use `development` solamente en un servidor de pruebas.

```bash
sudo git clone --branch master --single-branch \
  https://github.com/robisonnanez/project_iiif.git \
  /opt/project_iiif/source

cd /opt/project_iiif/source
git branch --show-current
git status --short
git log -1 --oneline
```

## 5. Configuración persistente

```bash
sudo cp backend/config.yaml.example /var/lib/project_iiif/config.yaml
sudo chown project-iiif:project-iiif /var/lib/project_iiif/config.yaml
sudo chmod 0640 /var/lib/project_iiif/config.yaml
sudoedit /var/lib/project_iiif/config.yaml
```

Use rutas absolutas:

```yaml
server:
  port: "8080"
  mode: "production"

storage:
  backend: "local"
  data_path: "/var/lib/project_iiif"
  pdfs_path: "/var/lib/project_iiif/pdfs"
  images_path: "/var/lib/project_iiif/images"
  documents_path: "/var/lib/project_iiif/documents"
  thumbnails_path: "/var/lib/project_iiif/thumbnails"
  manifests_path: "/var/lib/project_iiif/manifests"

pdf:
  temp_path: "/var/lib/project_iiif/temp"

binary_storage:
  mode: "local"
  temp_path: "/var/lib/project_iiif/temp"

frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
  menu_orientation: "horizontal"

iiif:
  base_url: "https://iiif.example.org"
  api_version: "3"
```

No se necesita un archivo `.env`. La unidad `systemd` solo define `CONFIG_PATH`, que indica dónde está el YAML y no contiene secretos.

## 6. Bases de datos

### 6.1 MySQL o MariaDB

Primero cree la base y el usuario desde una cuenta administradora:

```sql
CREATE DATABASE project_iiif CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'project_iiif'@'127.0.0.1' IDENTIFIED BY 'CAMBIAR_PASSWORD';
GRANT ALL PRIVILEGES ON project_iiif.* TO 'project_iiif'@'127.0.0.1';
FLUSH PRIVILEGES;
```

```yaml
storage:
  backend: "mysql"
database:
  auto_migrate: false
  mysql:
    host: "127.0.0.1"
    port: "3306"
    user: "project_iiif"
    password: "CAMBIAR_PASSWORD"
    database: "project_iiif"
    charset: "utf8mb4"
    parse_time: true
```

### 6.2 PostgreSQL

```sql
CREATE ROLE project_iiif LOGIN PASSWORD 'CAMBIAR_PASSWORD';
CREATE DATABASE project_iiif OWNER project_iiif;
```

```yaml
storage:
  backend: "postgres"
database:
  auto_migrate: false
  postgres:
    host: "127.0.0.1"
    port: "5432"
    user: "project_iiif"
    password: "CAMBIAR_PASSWORD"
    database: "project_iiif"
    sslmode: "disable"
    schema: "public"
```

En producción remota use TLS y un `sslmode` apropiado.

### 6.3 MongoDB

MongoDB no crea tablas; las migraciones crean colecciones e índices de forma idempotente.

```javascript
use project_iiif
db.createUser({
  user: "project_iiif",
  pwd: "CAMBIAR_PASSWORD",
  roles: [{role: "readWrite", db: "project_iiif"}]
})
```

```yaml
storage:
  backend: "mongodb"
database:
  auto_migrate: false
  mongodb:
    host: "127.0.0.1"
    port: "27017"
    user: "project_iiif"
    password: "CAMBIAR_PASSWORD"
    database: "project_iiif"
    auth_source: "project_iiif"
    direct_connection: true
```

## 7. Crear tablas, colecciones e índices

Hay dos métodos soportados. Las migraciones son versionadas e idempotentes.

### Método A: desde la interfaz

1. Inicie sesión.
2. Abra `Configuración`.
3. En `Motor de metadatos`, guarde la conexión correcta.
4. Pulse `Ejecutar migraciones ahora` en `Migraciones del esquema`.
5. Revise motor, migraciones aplicadas y omitidas.

MySQL y PostgreSQL registran cada versión en `schema_migrations`. MongoDB usa una colección equivalente y crea índices.

### Método B: al iniciar el servicio

```yaml
database:
  auto_migrate: true
```

El proceso aplica las migraciones pendientes antes de aceptar tráfico. Si una migración falla, el servicio no inicia y el error aparece en `journalctl`. El valor heredado `AUTO_MIGRATE=true` sigue siendo compatible, pero ya no es necesario.

Antes de habilitar migraciones automáticas en una actualización, haga backup. El usuario de base de datos necesita permisos DDL para crear o alterar tablas e índices.

## 8. RustFS o S3

Los metadatos pueden estar en una base y los binarios en RustFS/S3:

```yaml
FILESYSTEM_DISK: "s3"
AWS_ACCESS_KEY_ID: "projectiiif"
AWS_SECRET_ACCESS_KEY: "CAMBIAR_PASSWORD"
AWS_DEFAULT_REGION: "us-east-1"
AWS_BUCKET: "project-iiif"
AWS_ENDPOINT: "http://rustfs.internal:9000"
AWS_USE_PATH_STYLE_ENDPOINT: true
binary_storage:
  mode: "s3"
  temp_path: "/var/lib/project_iiif/temp"
```

Después del primer release:

```bash
cd /opt/project_iiif/current
sudo -u project-iiif ./s3-smoke
```

## 9. Compilar y probar

```bash
cd /opt/project_iiif/source/backend/frontend
pnpm install --frozen-lockfile
pnpm run lint
pnpm test
pnpm run build

cd /opt/project_iiif/source/backend
go mod download
go test ./...
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o project-iiif .
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" \
  -o s3-smoke ./cmd/s3-smoke
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" \
  -o migrate-local-to-db ./cmd/migrate-local-to-mysql
```

## 10. Crear un release

```bash
cd /opt/project_iiif/source
RELEASE="$(git rev-parse --short=12 HEAD)"
RELEASE_DIR="/opt/project_iiif/releases/$RELEASE"
sudo install -d -o root -g root -m 0755 "$RELEASE_DIR"
sudo install -o root -g root -m 0755 \
  backend/project-iiif backend/s3-smoke backend/migrate-local-to-db \
  "$RELEASE_DIR/"
sudo install -d -o root -g root -m 0755 "$RELEASE_DIR/frontend"
sudo cp -a backend/frontend/dist "$RELEASE_DIR/frontend/dist"
sudo cp -a backend/migrations "$RELEASE_DIR/migrations"
sudo ln -sfn "$RELEASE_DIR" /opt/project_iiif/current
```

No copie `config.yaml` dentro del release.

## 11. Servicio systemd

Cree `/etc/systemd/system/project-iiif.service`:

```ini
[Unit]
Description=Project IIIF PDF Server
Documentation=https://github.com/robisonnanez/project_iiif
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=project-iiif
Group=project-iiif
WorkingDirectory=/opt/project_iiif/current
Environment=CONFIG_PATH=/var/lib/project_iiif/config.yaml
ExecStart=/opt/project_iiif/current/project-iiif
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/project_iiif
UMask=0027

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now project-iiif
sudo systemctl status project-iiif --no-pager
curl -f http://127.0.0.1:8080/health
sudo journalctl -u project-iiif -n 100 --no-pager
```

## 12. Verificaciones funcionales

En una instalación sin documentos, el endpoint debe responder `[]`, nunca `null`:

```bash
curl -sS http://127.0.0.1:8080/api/v1/documents
```

Para un documento existente:

```bash
curl -f http://127.0.0.1:8080/api/v1/documents/ID_DOCUMENTO
curl -f http://127.0.0.1:8080/api/v1/iiif/ID_DOCUMENTO/manifest
curl -f -o /tmp/page.jpg \
  http://127.0.0.1:8080/iiif/3/ID_IMAGEN/full/max/0/default.jpg
file /tmp/page.jpg
```

Las rutas públicas documentadas son API administrativa v1 e IIIF v3.

## 13. Actualización segura

No actualice el binario activo dentro del repositorio. Use el mismo proceso de build y cree un release nuevo.

```bash
cd /opt/project_iiif/source
git status --short
git fetch origin master
git log --oneline HEAD..origin/master
git pull --ff-only origin master
```

Si `git status --short` muestra cambios, deténgase y revíselos. No use `git reset --hard` automáticamente.

Después:

1. Respalde configuración, base de datos y almacenamiento.
2. Ejecute pruebas y compilación de la sección 9.
3. Cree el release de la sección 10 sin cambiar todavía `current`.
4. Registre el release anterior.
5. Cambie `current`, reinicie y ejecute el health check.

```bash
OLD_RELEASE="$(readlink -f /opt/project_iiif/current)"
NEW_RELEASE="/opt/project_iiif/releases/COMMIT_NUEVO"
sudo ln -sfn "$NEW_RELEASE" /opt/project_iiif/current
sudo systemctl restart project-iiif
sleep 2
curl -f http://127.0.0.1:8080/health
```

Si el health falla:

```bash
sudo ln -sfn "$OLD_RELEASE" /opt/project_iiif/current
sudo systemctl restart project-iiif
sudo journalctl -u project-iiif -n 100 --no-pager
```

Un rollback de binario no revierte migraciones de base de datos.

## 14. Script de actualización

Instale `/usr/local/sbin/project-iiif-update` con permisos `0750`. El script debe:

1. Exigir un repositorio limpio.
2. Ejecutar `git fetch origin master`.
3. Terminar sin cambios cuando HEAD sea igual a `origin/master`.
4. Guardar el release activo.
5. Descargar con `git pull --ff-only`.
6. Ejecutar frontend y backend tests.
7. Compilar los tres binarios.
8. Crear un release inmutable.
9. Cambiar `current` y reiniciar.
10. Ejecutar `/health` y restaurar el symlink anterior si falla.

No incluya `git reset --hard`, no sobrescriba `/var/lib/project_iiif/config.yaml` y no borre automáticamente releases antiguos.

## 15. Avisos de actualizaciones

### Opción 1: comprobación manual

```bash
cd /home/robison/projects/project_iiif
git fetch origin master
git log --oneline master..origin/master
```

Es la opción más simple y no cambia la rama activa ni el árbol de trabajo.

### Opción 2: systemd timer de solo lectura - recomendada

Un script ejecuta `git fetch origin master`, compara `refs/heads/master` con `refs/remotes/origin/master` y escribe el resultado en el journal. La comparación es independiente de la rama activa, por lo que `development` puede seguir en uso sin producir avisos falsos. Un timer diario puede avisar por correo o webhook sin instalar nada. La cuenta solo necesita lectura del repositorio y acceso de salida a GitHub.

No use el timer para ejecutar `git pull`, compilar, migrar y reiniciar sin supervisión.

### Opción 3: aviso dentro del panel

Requiere un endpoint de versión que compare el commit desplegado contra GitHub y un banner en el frontend. Debe usar un tiempo límite, una caché de varias horas y los estados `disponible`, `actualizado` o `sin conexión`. No debe instalar automáticamente.

### Opción 4: GitHub Releases

Para producción estable, publique versiones etiquetadas y artefactos firmados. El sistema compara la versión instalada con la última release, en vez de seguir cada commit de `master`. Es la alternativa más controlada cuando haya varias instalaciones.

## 16. Backups

```bash
sudo cp -a /var/lib/project_iiif/config.yaml \
  "/root/project-iiif-config-$(date +%Y%m%d-%H%M%S).yaml"
sudo tar -czf "/root/project-iiif-data-$(date +%Y%m%d-%H%M%S).tar.gz" \
  /var/lib/project_iiif
```

Además use `mysqldump`, `pg_dump` o `mongodump` según el motor y respalde el bucket RustFS/S3.

## 17. Reverse proxy y HTTPS

Exponga Apache o Nginx en 443 y mantenga project_iiif en `127.0.0.1:8080`. Ejemplo Apache:

```apache
<VirtualHost *:443>
    ServerName iiif.example.org
    SSLEngine on
    SSLCertificateFile /ruta/fullchain.pem
    SSLCertificateKeyFile /ruta/privkey.pem
    ProxyPreserveHost On
    ProxyPass / http://127.0.0.1:8080/
    ProxyPassReverse / http://127.0.0.1:8080/
    RequestHeader set X-Forwarded-Proto "https"
</VirtualHost>
```

No publique directamente 8080 en Internet.

## 18. Diagnóstico

```bash
sudo systemctl status project-iiif --no-pager
sudo journalctl -u project-iiif -b --no-pager
sudo journalctl -u project-iiif -f
curl -i http://127.0.0.1:8080/health
readlink -f /opt/project_iiif/current
git -C /opt/project_iiif/source log -1 --oneline
```

Si el dashboard muestra `Cannot read properties of null (reading 'map')`, compruebe que está ejecutando una versión que normaliza listas vacías y que `/api/v1/documents` responde `[]`. El error se corrigió tanto en el backend como defensivamente en el frontend.

## 19. Lista final

- Servicio ejecutado por un usuario sin privilegios.
- Configuración y secretos fuera del repositorio.
- Datos persistentes fuera de releases.
- Migraciones manuales desde Configuración.
- Migraciones automáticas opcionales desde `database.auto_migrate`.
- MySQL, PostgreSQL y MongoDB soportados.
- RustFS/S3 comprobable con `s3-smoke`.
- API v1 e IIIF v3 verificadas.
- Releases inmutables, health check y rollback.
- Comprobación de actualizaciones separada de la instalación.

## 20. Script completo de actualización supervisada

Cree `/usr/local/sbin/project-iiif-update`:

```bash
#!/usr/bin/env bash
set -euo pipefail

SOURCE="/opt/project_iiif/source"
RELEASES="/opt/project_iiif/releases"
CURRENT="/opt/project_iiif/current"
CONFIG="/var/lib/project_iiif/config.yaml"
BRANCH="${BRANCH:-master}"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Ejecute este script como root"
  exit 1
fi
if [[ ! -f "${CONFIG}" ]]; then
  echo "No existe ${CONFIG}"
  exit 1
fi

cd "${SOURCE}"
if [[ -n "$(git status --porcelain)" ]]; then
  echo "El repositorio contiene cambios locales; se cancela"
  git status --short
  exit 1
fi

git fetch origin "${BRANCH}"
LOCAL="$(git rev-parse HEAD)"
REMOTE="$(git rev-parse "origin/${BRANCH}")"
if [[ "${LOCAL}" == "${REMOTE}" ]]; then
  echo "No hay actualizaciones"
  exit 0
fi

echo "Cambios pendientes:"
git log --oneline "HEAD..origin/${BRANCH}"
PREVIOUS=""
if [[ -L "${CURRENT}" ]]; then
  PREVIOUS="$(readlink -f "${CURRENT}")"
fi

git pull --ff-only origin "${BRANCH}"

cd "${SOURCE}/backend/frontend"
pnpm install --frozen-lockfile
pnpm run lint
pnpm test
pnpm run build

cd "${SOURCE}/backend"
go mod download
go test ./...
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o project-iiif .
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" \
  -o s3-smoke ./cmd/s3-smoke
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" \
  -o migrate-local-to-db ./cmd/migrate-local-to-mysql

cd "${SOURCE}"
RELEASE="$(git rev-parse --short=12 HEAD)"
RELEASE_DIR="${RELEASES}/${RELEASE}"
if [[ -e "${RELEASE_DIR}" ]]; then
  echo "El release ${RELEASE_DIR} ya existe; revíselo antes de continuar"
  exit 1
fi
install -d -o root -g root -m 0755 \
  "${RELEASE_DIR}" "${RELEASE_DIR}/frontend"
install -o root -g root -m 0755 \
  backend/project-iiif backend/s3-smoke backend/migrate-local-to-db \
  "${RELEASE_DIR}/"
cp -a backend/frontend/dist "${RELEASE_DIR}/frontend/dist"
cp -a backend/migrations "${RELEASE_DIR}/migrations"

ln -sfn "${RELEASE_DIR}" "${CURRENT}"
systemctl restart project-iiif

for attempt in {1..20}; do
  if curl -fsS http://127.0.0.1:8080/health >/dev/null; then
    echo "Actualización completada: ${RELEASE}"
    exit 0
  fi
  sleep 1
done

echo "El health check falló"
journalctl -u project-iiif -n 100 --no-pager || true
if [[ -n "${PREVIOUS}" ]]; then
  echo "Restaurando ${PREVIOUS}"
  ln -sfn "${PREVIOUS}" "${CURRENT}"
  systemctl restart project-iiif
fi
exit 1
```

```bash
sudo chmod 0750 /usr/local/sbin/project-iiif-update
sudo project-iiif-update
```

El rollback restaura el binario y el frontend anteriores, pero no deshace migraciones.

## 21. Timer para avisar sin actualizar

El repositorio incluye el script y las unidades en `deploy/`. Estos artefactos corresponden al perfil de `servidor-prueba`: ejecutan la consulta como `robison`, leen `/home/robison/projects/project_iiif` y supervisan `master` aunque el checkout activo sea `development`.

Instálelos desde la raíz del repositorio sin modificar sus rutas ni la rama supervisada:

```bash
sudo install -o root -g robison -m 0750 \
  deploy/project-iiif-check-update \
  /usr/local/sbin/project-iiif-check-update
sudo install -o root -g root -m 0644 \
  deploy/project-iiif-update-check.service \
  /etc/systemd/system/project-iiif-update-check.service
sudo install -o root -g root -m 0644 \
  deploy/project-iiif-update-check.timer \
  /etc/systemd/system/project-iiif-update-check.timer
```

La unidad ejecuta el comprobador como `robison` y supervisa exclusivamente:

```text
Repositorio: /home/robison/projects/project_iiif
Remoto: origin
Rama: master
```

En una instalación estable bajo `/opt/project_iiif/source`, cree una unidad específica para el usuario propietario de ese clon y ajuste `WorkingDirectory` y `PROJECT_IIIF_SOURCE`. El usuario elegido debe poder actualizar las referencias remotas dentro de `.git`; no reutilice sin revisión el perfil de `servidor-prueba`.

El script registra en `journalctl` si `master` está actualizada, si el remoto está adelantado, si las referencias divergieron o si falló la consulta. El código `10` significa que hay una actualización disponible y systemd lo considera correcto. Cualquier inconsistencia o error de red termina con código `20`.

Habilite el timer y ejecute una primera comprobación manual:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now project-iiif-update-check.timer
sudo systemctl start project-iiif-update-check.service
sudo systemctl status project-iiif-update-check.service --no-pager
sudo systemctl list-timers project-iiif-update-check.timer
sudo journalctl -t project-iiif-update --no-pager
```

La comprobación actualiza únicamente las referencias remotas de Git; no cambia de rama, no mueve `refs/heads/master`, no ejecuta `git pull`, no compila, no migra y no reinicia el servicio. Para correo o webhook, conecte una unidad `OnFailure=` o un monitor externo al resultado. La instalación de la actualización debe seguir siendo supervisada.
