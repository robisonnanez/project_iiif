# OCR híbrido por página

El OCR se ejecuta como trabajos asíncronos y conserva el número de página y los Canvas IIIF v2/v3 de cada resultado. El modo `hybrid` usa la capa de texto del PDF cuando es suficiente y ejecuta Tesseract en páginas escaneadas; `exhaustive` también reconoce las imágenes de páginas que ya tienen texto digital.

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

## Operación

La vista `/dashboard/ocr` permite seleccionar documento, proyecto/tenant, modo e idiomas, iniciar/cancelar trabajos y buscar resultados. El progreso se actualiza automáticamente.

Endpoints administrativos:

- `POST /api/v1/admin/documents/{id}/ocr/jobs`
- `GET /api/v1/admin/ocr/jobs/{job_id}`
- `POST /api/v1/admin/ocr/jobs/{job_id}/cancel`
- `GET /api/v1/admin/documents/{id}/ocr`
- `GET /api/v1/admin/documents/{id}/ocr/pages/{page}`
- `GET /api/v1/admin/ocr/search?q=texto&project=default`
- `DELETE /api/v1/admin/documents/{id}/ocr`

Endpoints públicos de lectura para integraciones externas:

- `GET /api/v1/documents/{id}/ocr`
- `GET /api/v1/documents/{id}/ocr/pages/{page}`
- `GET /api/v1/documents/{id}/ocr/search?q=texto`
- `GET /api/v1/ocr/search?q=texto&project=default&tenant=tenant`
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

## Recuperación

- Una página puede reintentarse dos veces y tiene un tiempo límite independiente de 120 segundos.
- Cancelar un trabajo conserva las páginas ya procesadas, pero no activa esa generación.
- Para reprocesar un documento con OCR activo, marque la opción de nueva generación (`force: true`).
- Para detener consumo de CPU sin borrar resultados, cambie `ocr.enabled` a `false` y reinicie.
