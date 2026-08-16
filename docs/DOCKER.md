# Docker y Docker Compose

Esta guía levanta `project_iiif` desde cero con RustFS y uno de los tres motores de metadata. Compose usa perfiles para evitar duplicación.

## 1. Requisitos

- Docker Engine 24 o posterior.
- Docker Compose v2.
- Puertos libres: aplicación `18080`, RustFS API `19000` y consola `19001` por defecto.

No se necesita Go, Node ni pnpm en el host: el `Dockerfile` multi-stage compila React con Node 22, compila los tres binarios Go y crea una imagen Debian mínima que ejecuta como UID 10001.

## 2. Configuración inicial y `.env`

```bash
cp .env.example .env
chmod 600 .env
```

Edita todos los valores:

```dotenv
DB_PASSWORD=una-clave-de-desarrollo-no-reutilizada
RUSTFS_ACCESS_KEY=una-access-key-no-reutilizada
RUSTFS_SECRET_KEY=una-secret-key-larga
FRONTEND_USERNAME=admin
FRONTEND_PASSWORD=una-clave-del-dashboard
```

Variables opcionales: `APP_PORT`, `APP_BASE_URL`, `RUSTFS_API_PORT` y `RUSTFS_CONSOLE_PORT`. No confirmes `.env`; `.env.example` contiene únicamente placeholders.

## 3. Build

```bash
docker compose --profile mysql build --no-cache app-mysql
```

El mismo tag `project-iiif:development` sirve para los tres perfiles; no es necesario reconstruir al cambiar de motor.

## 4. Elegir base de datos

```bash
# MySQL 8.4
docker compose --profile mysql up -d --wait

# PostgreSQL 17
docker compose --profile mysql down
docker compose --profile postgres up -d --wait

# MongoDB 8
docker compose --profile postgres down
docker compose --profile mongodb up -d --wait
```

`down` sin `-v` elimina contenedores y red, pero conserva los volúmenes. Usa un perfil de app a la vez porque todos publican el mismo puerto.

## 5. Servicios y healthchecks

Cada perfil inicia:

- `app-{motor}`: backend y dashboard, healthcheck `/health`.
- `{motor}`: base con healthcheck nativo.
- `rustfs`: S3 compatible con volumen y healthcheck HTTP.
- `config-init`: inicializa una sola vez el volumen de configuración.
- `rustfs-permissions`: prepara permisos de los volúmenes.

```bash
docker compose --profile mysql ps
curl -f http://127.0.0.1:18080/health
```

Compose usa `depends_on.condition` para esperar servicios saludables; no sustituye healthchecks por sleeps fijos.

## 6. Configuración activa y UI

`deploy/docker/config.yaml` se copia a `app_config:/app/config/config.yaml` solo si el volumen aún no tiene configuración. `CONFIG_PATH` apunta a ese archivo. Las variables del perfil seleccionan motor y credenciales sin escribir secretos en Git.

El dashboard está en `http://127.0.0.1:18080/dashboard`. Desde allí se configura motor, Mongo URI, almacenamiento S3, conversión y migraciones. El guardado llega al backend y se escribe atómicamente en `app_config`. Para cambios de conexión que requieren reinicio:

```bash
docker compose --profile mysql restart app-mysql
docker compose --profile mysql up -d --wait app-mysql
```

## 7. RustFS / S3

Dentro de Compose el endpoint es `http://rustfs:9000`; desde el host es `http://127.0.0.1:19000`. El bucket predeterminado es `project-iiif`, path-style está habilitado y la consola está en `http://127.0.0.1:19001`.

Smoke test real:

```bash
docker compose --profile mysql exec -T app-mysql \
  env CONFIG_PATH=/app/config/config.yaml s3-smoke
```

Debe imprimir `S3/RustFS OK: escritura, lectura y eliminación verificadas`.

## 8. Migraciones

`AUTO_MIGRATE=true` ejecuta migraciones pendientes antes de aceptar tráfico. SQL registra versiones en `schema_migrations`; MongoDB inicializa colecciones e índices de forma idempotente.

```bash
docker compose --profile mysql logs app-mysql
```

`005_add_conversion_settings.sql` conserva parámetros de conversión y `006_add_pdf_outline.sql` añade el outline.

## 9. Subir un PDF y probar IIIF

```bash
curl -f -F 'pdf=@documento.pdf;type=application/pdf' \
  -F 'project=default' \
  -F 'max_width=1241' -F 'max_height=1754' \
  -F 'dpi=150' -F 'format=jpg' -F 'quality=85' \
  http://127.0.0.1:18080/api/v1/documents/upload
```

La respuesta inicial puede tener `status=processing`. Consulta `/api/v1/documents/{id}` hasta `completed` y luego:

```bash
curl -f http://127.0.0.1:18080/api/iiif/{id}/manifest
curl -f 'http://127.0.0.1:18080/api/iiif/{id}/manifest?pages=1-5,8,10-12'
curl -f http://127.0.0.1:18080/iiif/2/{id}_page_1/info.json
curl -f -o page.jpg http://127.0.0.1:18080/iiif/2/{id}_page_1/full/600,/0/default.jpg
```

Los PDF con bookmarks incluyen `structures`; los PDF sin bookmarks omiten el campo.

## 10. Persistencia y volúmenes

- `app_config`: configuración guardada desde la UI.
- `app_data`: temporales y datos locales permitidos.
- `rustfs_data` y `rustfs_logs`: objetos y logs RustFS.
- `mysql_data`, `postgres_data`, `mongodb_data`: catálogo de cada motor.

```bash
docker compose --profile mysql restart
docker compose --profile mysql up -d --wait
curl -f http://127.0.0.1:18080/api/v1/documents/{id}
curl -f http://127.0.0.1:18080/api/iiif/{id}/manifest
```

## 11. Backups

Detén escrituras antes de una copia consistente. Usa `mysqldump`, `pg_dump` o `mongodump` para metadata. Para configuración y objetos:

```bash
docker run --rm -v project-iiif_rustfs_data:/source:ro \
  -v "$PWD/backups":/backup busybox \
  tar -czf /backup/rustfs-data.tgz -C /source .
docker run --rm -v project-iiif_app_config:/source:ro \
  -v "$PWD/backups":/backup busybox \
  tar -czf /backup/app-config.tgz -C /source .
```

En producción aplica una política de backup consistente del motor y del almacén de objetos; no dependas solo de una copia de volumen en caliente.

## 12. Actualización y rollback

```bash
git pull --ff-only
docker compose --profile mysql build
docker compose --profile mysql up -d --wait
```

Para rollback, vuelve al tag/commit anterior, reconstruye y levanta sin `-v`. Respalda antes de migraciones sin reversión automática.

## 13. Logs y troubleshooting

```bash
docker compose --profile mysql ps
docker compose --profile mysql logs --tail=200 app-mysql mysql rustfs
docker compose --profile mysql config --quiet
```

- `variable is required`: falta una clave en `.env`.
- puerto ocupado: cambia `APP_PORT`, `RUSTFS_API_PORT` o `RUSTFS_CONSOLE_PORT`.
- S3 `AccessDenied`: app y RustFS deben usar las mismas claves y bucket.
- manifest 404: espera `status=completed`.
- manifest parcial 400: revisa sintaxis y páginas existentes.
- conexión no cambia tras guardar: reinicia solo `app-{motor}`.

## 14. Reset del entorno

Operación destructiva, únicamente para desarrollo y tras verificar el proyecto Compose correcto:

```bash
docker compose --profile mysql down -v --remove-orphans
```

Esto elimina bases, configuración y objetos RustFS del proyecto. Sin `-v`, los datos se conservan.
