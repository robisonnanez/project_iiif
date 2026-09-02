# OCR híbrido por página

El OCR se ejecuta como trabajos asíncronos y conserva el número de página y los Canvas IIIF v2/v3 de cada resultado. El modo `hybrid` conserva la capa de texto del PDF cuando es suficiente y ejecuta Tesseract una vez sobre la imagen renderizada para obtener geometría por palabra; en páginas escaneadas usa también ese resultado como texto completo. `exhaustive` combina texto nativo y OCR.

## Activación

El despliegue inicial deja OCR desactivado. En el servicio del puerto 8080 edite `/opt/project_iiif/backend/config.yaml`:

```yaml
ocr:
  enabled: true
  auto_after_conversion: false
  default_mode: hybrid
  workers: 2
  page_timeout_seconds: 120
  retries_per_page: 2
  render_dpi: 300
  min_text_chars: 40
  candidate_languages: [spa, eng, fra, por]
  fallback_languages: [spa]
  language_detection:
    enabled: true
    sample_pages: 5
    min_sample_chars: 200
    minimum_confidence: 0.70
    max_languages: 2
  artifacts:
    gzip: true
  language_installation:
    enabled: false
    helper_path: /usr/local/sbin/project-iiif-install-tesseract-language
    timeout_seconds: 300
```

Después reinicie únicamente ese servicio:

```bash
sudo systemctl restart project-iiif
```

En Docker, cambie la misma sección en el archivo persistente del volumen de configuración o en `deploy/docker/config.yaml` antes de crear el volumen. Después reconstruya y recree únicamente el servicio de aplicación del perfil elegido.

Verifique las dependencias con:

```bash
tesseract --version
tesseract --list-langs
```

Los idiomas instalados deben incluir `spa`, `eng`, `fra`, `por` y `osd`.

## Idiomas del sistema e instalación segura

La configuración administrativa consulta `tesseract --list-langs` para los idiomas realmente instalados y `apt-cache pkgnames tesseract-ocr-` para los paquetes disponibles. La diferencia se presenta como **Idiomas por instalar**. La lista no está codificada en el frontend; los nombres conocidos se traducen y los códigos desconocidos se muestran como `Tesseract (código)`.

Endpoints administrativos, ambos protegidos por la cookie de sesión:

- `GET /api/v1/admin/ocr/languages`
- `POST /api/v1/admin/ocr/languages/install` con `{"languages":["deu","ita"]}`

La instalación está deshabilitada por defecto. Para habilitarla en Ubuntu/Debian, un administrador debe instalar los archivos versionados con propiedad y permisos restringidos:

```bash
sudo install -o root -g root -m 0755 deploy/project-iiif-install-tesseract-language /usr/local/sbin/project-iiif-install-tesseract-language
sudo install -o root -g root -m 0440 deploy/project-iiif-tesseract-language.sudoers /etc/sudoers.d/project-iiif-tesseract-language
sudo visudo -cf /etc/sudoers.d/project-iiif-tesseract-language
```

Después se puede cambiar únicamente en el archivo `config.yaml`:

```yaml
ocr:
  language_installation:
    enabled: true
    helper_path: /usr/local/sbin/project-iiif-install-tesseract-language
    timeout_seconds: 300
```

El backend nunca ejecuta `apt-get` directamente, no usa `sh -c` y no acepta nombres de paquetes del navegador. Valida como máximo diez códigos contra el catálogo APT recién consultado y llama con argumentos separados a `sudo -n <helper> <código>`. El helper, propiedad de `root`, vuelve a validar un único código, bloquea instalaciones simultáneas, limita el paquete a `tesseract-ocr-<código>`, aplica un timeout propio, registra auditoría y verifica el resultado con `tesseract --list-langs`. La API administrativa no puede habilitar ni cambiar la ruta del helper: estos campos se conservan desde `config.yaml`.

Instalar no habilita automáticamente. Los idiomas ISO 639-3 compatibles con Lingua pueden activarse luego en **Idiomas instalados** para detección automática; otros permanecen disponibles para selección manual. No se concede `sudo apt` general al usuario del servicio.

## Operación

La vista `/dashboard/ocr` permite seleccionar documento, proyecto/tenant, modo e idiomas, iniciar/cancelar trabajos y buscar resultados. El progreso se actualiza automáticamente.

Endpoints administrativos:

- `POST /api/v1/admin/documents/{id}/ocr/jobs`
- `GET /api/v1/admin/ocr/jobs/{job_id}`
- `POST /api/v1/admin/ocr/jobs/{job_id}/cancel`
- `GET /api/v1/admin/documents/{id}/ocr`
- `GET /api/v1/admin/documents/{id}/ocr/pages/{page}`
- `GET /api/v1/admin/ocr/search?q=texto&project=default`
- `GET /api/v1/admin/ocr/autocomplete?q=func&project=default&limit=10`
- `DELETE /api/v1/admin/documents/{id}/ocr`

Endpoints públicos de lectura para integraciones externas:

- `GET /api/v1/documents/{id}/ocr`
- `GET /api/v1/documents/{id}/ocr/pages/{page}`
- `GET /api/v1/documents/{id}/ocr/pages/{page}/words?q=SÁNCHEZ&limit=100`
- `GET /api/v1/documents/{id}/ocr/search?q=texto`
- `GET /api/v1/documents/{id}/ocr/autocomplete?q=func&limit=10`
- `GET /api/v1/ocr/search?q=texto&project=default&tenant=tenant`
- `GET /api/v1/ocr/autocomplete?q=func&project=default&tenant=tenant&limit=10`
- `GET /api/v1/iiif/{id}/manifest`
- `GET /api/v1/iiif/{id}/manifest/v3`

Las llamadas desde un frontend ubicado en otro dominio deben agregar ese origen exacto en **Configuración → Seguridad y CORS**. CORS no afecta las integraciones servidor a servidor.

Ejemplo de creación:

```json
{
  "mode": "hybrid",
  "language_mode": "auto",
  "languages": [],
  "force": false
}
```

Los trabajos y artefactos son durables bajo `{storage.data_path}/ocr`. Cada página se guarda comprimida por documento y generación; una generación nueva solo queda activa cuando termina el procesamiento completo. Los trabajos incompletos vuelven a la cola cuando arranca el servicio.

## Texto y coordenadas por palabra

Tesseract se ejecuta directamente, sin shell ni Tesseract.js, con una sola salida TSV por página:

```text
tesseract <imagen-temporal.png> stdout -l <idiomas> --psm 3 tsv
```

El parser acepta únicamente registros TSV de palabra (`level=5`) con texto, confianza no negativa y dimensiones positivas. La puntuación y Unicode se conservan sin aplicar la normalización utilizada por el buscador.

```json
{
  "page_number": 97,
  "width": 1239,
  "height": 1754,
  "ocr_image_width": 2480,
  "ocr_image_height": 3509,
  "geometry_status": "word",
  "geometry_space": "canvas",
  "words": [
    {
      "text": "SÁNCHEZ,",
      "confidence": 95.20067596435548,
      "bbox": {"x0": 470, "x1": 508, "y0": 772, "y1": 779}
    }
  ]
}
```

MuPDF renderiza la imagen temporal según `ocr.render_dpi` (300 DPI por defecto). El Canvas IIIF usa las dimensiones de la imagen de página almacenada, que pueden ser distintas. Antes de persistir, el servicio transforma cada esquina con `canvas/ocrImage`; por ello `bbox` está listo para superponerse directamente sobre el Canvas. `width` y `height` describen el Canvas, mientras `ocr_image_width` y `ocr_image_height` permiten auditar la transformación.

Los estados existentes se conservan:

- `word`: hay cajas reales de palabras y `geometry_space` vale `canvas`.
- `page_only`: hay texto de página, pero no cajas; es el estado normal del OCR histórico no reprocesado o el fallback si Tesseract falla y existe texto nativo.

Las páginas antiguas que sí guardaban `left/top/width/height` se leen de forma compatible y se transforman al contrato `bbox` al responder. No se modifican ni eliminan artefactos históricos. Para obtener cajas en páginas históricas `page_only`, se debe lanzar manualmente una nueva generación con `force: true`.

Para localizar una palabra concreta sin transferir el texto completo ni todas las cajas se puede usar:

```http
GET /api/v1/documents/{id}/ocr/pages/{page}/words?q=SÁNCHEZ&limit=100
```

La respuesta contiene únicamente las apariciones exactas con `text`, `confidence` y `bbox`. La comparación ignora mayúsculas, acentos y puntuación exterior, preservando el texto OCR original en la respuesta. `limit` vale 100 por defecto y no supera 1000. Una generación histórica `page_only` responde `409` e indica que debe reprocesarse; la ruta nunca genera OCR durante una consulta.

## Autocompletado de palabras

El autocompletado complementa la búsqueda de páginas y no la reemplaza. Solo devuelve palabras que existen en la generación OCR activa dentro del documento, proyecto y tenant solicitados. La vista administrativa usa el mismo alcance elegido en el buscador, espera 300 ms después de la última pulsación y cancela solicitudes obsoletas.

```http
GET /api/v1/ocr/autocomplete?q=func&project=default&limit=10
```

```json
{
  "query": "func",
  "items": [
    { "text": "funciones", "frequency": 128 },
    { "text": "funcionalidad", "frequency": 84 },
    { "text": "funcionamiento", "frequency": 41 }
  ]
}
```

- `q` es obligatorio y debe contener al menos 2 caracteres Unicode después de normalizar espacios.
- `limit` vale 10 por defecto, debe ser positivo y nunca supera 50.
- `project`, `tenant` y `document_id` conservan el mismo significado y alcance que en `/api/v1/ocr/search`. La variante bajo `/api/v1/admin` requiere la cookie de sesión administrativa; las rutas públicas mantienen la política de lectura del buscador OCR existente.
- La comparación es por inicio de palabra, no por substring. Ignora mayúsculas y diacríticos mediante normalización Unicode, por lo que `informacion` puede sugerir `información`; la respuesta conserva la grafía original del OCR.
- Las formas equivalentes por mayúsculas o acentos se consolidan. El orden prioriza coincidencia exacta normalizada, frecuencia descendente, longitud y orden alfabético.
- Cada generación terminada crea un vocabulario comprimido y ordenado bajo `{storage.data_path}/ocr/vocabularies/<documento>/<generación>.json.gz`. Las instalaciones con OCR anterior lo reconstruyen una sola vez al primer uso y lo conservan en caché de memoria. Reprocesar crea otro vocabulario antes de activar la generación; eliminar el OCR o el documento elimina también sus vocabularios.

El vocabulario evita abrir, descomprimir y tokenizar todas las páginas en cada pulsación. No añade tablas, migraciones, Redis ni dependencias externas.

## Recuperación

- Una página puede reintentarse dos veces y tiene un tiempo límite independiente de 120 segundos.
- Cancelar un trabajo conserva las páginas ya procesadas, pero no activa esa generación.
- Para reprocesar un documento con OCR activo, marque la opción de nueva generación (`force: true`).
- Para detener consumo de CPU sin borrar resultados, cambie `ocr.enabled` a `false` y reinicie.
