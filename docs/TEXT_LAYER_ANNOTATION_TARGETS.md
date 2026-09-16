# Text Layer OCR y destinos de anotación

Project IIIF publica texto y geometría estables por página. No almacena usuarios, notas privadas, colores, permisos ni selectores de la aplicación consumidora.

## Identidad e invariantes

- `document_id` identifica un PDF inmutable. Un PDF distinto debe cargarse como un documento nuevo.
- Cada reprocesamiento crea una `generation` nueva; las anteriores se conservan.
- Las coordenadas tienen origen superior izquierdo, crecen hacia la derecha y abajo y usan píxeles del Canvas IIIF sin rotar.
- `canvas.width` y `canvas.height` coinciden con el manifest. El consumidor no vuelve a escalar desde las dimensiones de la imagen OCR.
- `geometry_status=page_only` significa que no existen cajas reales y no deben inventarse rectángulos.

## Autenticación

Cuando `security.integration_auth.enabled` está activo, las rutas OCR públicas y Text Layer requieren una cookie administrativa válida o un Bearer HS256 con scope `text-layer:read`.

Una sesión administrativa puede emitir un token temporal:

```http
POST /api/v1/admin/integration-tokens
Content-Type: application/json

{
  "consumer_id": "visor-anotaciones",
  "project": "metavisor",
  "tenant": "biblioteca-a",
  "document_id": "uuid-opcional",
  "ttl_seconds": 300
}
```

El TTL máximo es 3600 segundos. Los ámbitos se comparan con el proyecto y tenant resueltos desde el documento; nunca se confía en query strings como autoridad.

## Endpoints

```http
GET /api/v1/documents/{document_id}/text-layer/status
GET /api/v1/documents/{document_id}/text-layer/pages/{page}?generation={uuid}
GET /api/v1/documents/{document_id}/ocr/generations
```

Omitir `generation` selecciona la generación activa. La página contiene palabras con `order`, índices de bloque/párrafo/línea/palabra y `bbox`. `layer_sha256` también se entrega como ETag fuerte; `If-None-Match` puede producir 304.

Las respuestas admiten Brotli y gzip, incluyen `Vary: Accept-Encoding, Authorization` y usan caché privada. Una página superior a 10 MiB se sirve completa y genera una alerta de tamaño.

## Destino durable para el consumidor

La aplicación que guarda una anotación debe conservar como mínimo:

```json
{
  "document_id": "uuid-del-pdf",
  "generation": "uuid-del-ocr",
  "page_number": 25,
  "canvas_id": "https://iiif.example/canvas/25",
  "canvas_width": 1241,
  "canvas_height": 1754,
  "layer_sha256": "hexadecimal"
}
```

Una selección multilínea conserva una lista ordenada de rectángulos. Para almacenamiento independiente del tamaño de presentación, el consumidor puede normalizar cada coordenada dividiéndola por ancho o alto del canvas. El texto seleccionado puede complementarse con `exact`, `prefix` y `suffix` al estilo `TextQuoteSelector`.

## Errores

Los errores nuevos usan `error.code`, `error.message` y `error.details`. Los códigos principales son `invalid_request`, `unauthorized`, `forbidden`, `document_or_generation_not_found`, `document_not_ready`, `word_geometry_unavailable`, `ocr_unavailable` y `rate_limited`.

## Almacenamiento y retención

Los artefactos nuevos siguen el backend binario configurado: disco local o S3/RustFS. En S3, las claves viven bajo `ocr/`; las lecturas conservan fallback al layout local histórico. Un fallo de escritura remota no cae silenciosamente a disco local. No existe purga automática ni `pin/unpin` en esta versión.
