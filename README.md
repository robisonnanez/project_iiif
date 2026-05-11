# IIIF PDF Server

Servidor Go que convierte archivos PDF a imágenes y los sirve usando el protocolo IIIF v3.

## Características

- ✅ Conversión de PDF a imágenes (JPEG, PNG, WebP)
- ✅ API IIIF Image v3 completa
- ✅ Generación de manifiestos IIIF Presentation v3
- ✅ Integración con Universal Viewer
- ✅ Sistema de caché configurable
- ✅ Configuración flexible via YAML
- ✅ API REST para gestión de documentos
- ✅ Dashboard web protegido por sesión con login/logout
- ✅ Galería de imágenes IIIF por documento
- ✅ Configuración editable desde el dashboard con secretos enmascarados
- ✅ Almacenamiento MySQL BLOB para PDFs e imágenes convertidas
- ✅ Organización opcional por proyecto y tenant

## Instalación

1. **Instalar dependencias del sistema:**

```bash
# Ubuntu/Debian
sudo apt-get install libmupdf-dev
```
<!--
```bash
# Ubuntu/Debian
sudo apt-get install libmupdf-dev
# macOS
brew install mupdf-tools

# Windows
# Descargar MuPDF desde https://mupdf.com/downloads/ 
```
-->

2. **Clonar y compilar:**

```bash
git clone <tu-repo>
cd backend
go mod download
go build -o iiif-server main.go
```

3. **Ejecutar:**

```bash
./iiif-server
```

## Configuración

Edita `config.yaml` para personalizar:

- Puerto del servidor
- Rutas de almacenamiento
- Límites de archivos
- Configuración IIIF
- Parámetros de conversión
- Frontend administrativo
- Conexión MySQL y modo de almacenamiento binario
- Proyectos consumidores y tenants opcionales

Para MySQL BLOB usa:

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
  temp_path: "/var/lib/project_iiif/temp"
```

Cuando `DB_CONNECTION`/`STORAGE_BACKEND` sea `mysql`, los PDFs e imágenes se guardan como BLOB en la base de datos. El disco local solo se usa para temporales durante la conversión.

### Proyectos y multitenant

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

`POST /api/upload` acepta `project` y `tenant` por form-data o por headers `X-IIIF-Project` y `X-IIIF-Tenant`. Si el proyecto no es multitenant, `tenant` se ignora. Si es multitenant, `tenant` es obligatorio.

## Dashboard

Activa el frontend en `config.yaml`:

```yaml
frontend:
  enabled: true
  path: "./frontend"
  require_auth: true
  username: "admin"
  password: "CAMBIAR_PASSWORD"
```

- `/` sigue siendo público.
- `/dashboard` requiere sesión cuando `require_auth=true`.
- El botón **Cerrar sesión** llama `POST /auth/logout`.
- La vista **Imágenes** muestra las páginas convertidas y sus URLs IIIF, por ejemplo:

```text
http://localhost:8080/iiif/3/{image_id}/full/max/0/default.jpg
```

- La vista **Configuración** permite editar campos permitidos del `config.yaml`. Los passwords se muestran como `********`; si se dejan así, se conserva el valor real.
- Después de guardar configuración, reinicia el servicio para aplicar cambios sensibles como puerto, storage o credenciales.

## API Endpoints

### Autenticación dashboard
- `POST /auth/login` - Crear sesión
- `POST /auth/logout` - Cerrar sesión
- `GET /auth/me` - Consultar sesión activa

### Gestión de documentos
- `POST /api/upload` - Subir PDF
- `GET /api/documents` - Listar documentos
- `GET /api/documents/:id` - Obtener documento
- `DELETE /api/documents/:id` - Eliminar documento

### Administración
- `GET /admin/api/config` - Configuración saneada
- `PUT /admin/api/config` - Guardar configuración permitida
- `GET /admin/api/projects` - Listar proyectos y tenants configurados
- `GET /admin/api/documents/:id/images` - Listar imágenes IIIF de un documento

### IIIF
- `GET /api/iiif/:id/manifest` - Manifiesto IIIF
- `GET /iiif/3/:identifier/info.json` - Info de imagen
- `GET /iiif/3/:identifier/:region/:size/:rotation/:quality.:format` - Imagen IIIF
- `GET /iiif/3/:identifier/default.jpg` - Imagen por defecto

### Configuración
- `GET /api/properties` - Obtener propiedades
- `PUT /api/properties` - Actualizar propiedades

## Estructura del proyecto

```
backend/
├── main.go                 # Punto de entrada
├── config.yaml            # Configuración
├── internal/
│   ├── config/            # Gestión de configuración
│   ├── models/            # Modelos de datos
│   ├── storage/           # Almacenamiento
│   ├── services/          # Lógica de negocio
│   └── handlers/          # Controladores HTTP
└── data/                  # Datos (creado automáticamente)
    ├── documents/         # Metadatos de documentos
    ├── images/           # Imágenes convertidas
    ├── thumbnails/       # Miniaturas
    └── manifests/        # Manifiestos IIIF
```

## Desarrollo

Para desarrollo, usa:

```bash
go run main.go
```

Para producción:

```bash
go build -ldflags="-s -w" -o iiif-server main.go
```

<!-- ## Docker (Opcional)

```dockerfile
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache musl-dev gcc mupdf-dev
WORKDIR /app
COPY . .
RUN go build -o iiif-server main.go

FROM alpine:latest
RUN apk add --no-cache mupdf
WORKDIR /app
COPY --from=builder /app/iiif-server .
COPY config.yaml .
EXPOSE 8080
CMD ["./iiif-server"]
``` -->
