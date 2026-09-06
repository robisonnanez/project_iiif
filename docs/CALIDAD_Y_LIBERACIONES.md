# Política de calidad y liberaciones

## Contratos JSON

- Las colecciones públicas se inicializan y serializan siempre como `[]`, nunca como `null`.
- Las pruebas de contrato deben inspeccionar el JSON real en estados nulo heredado, vacío y poblado.
- El frontend valida en ejecución la forma de respuestas críticas antes de utilizarlas; un incumplimiento genera un error controlado.

## Documentación del código

Las funciones nombradas de producción Go, TypeScript y TSX deben tener un comentario inmediatamente anterior que explique su responsabilidad. No se exige comentar callbacks triviales, pruebas ni Swagger generado. `go run ./cmd/check-comments` verifica esta política.

## Integración continua

El workflow ejecuta pruebas y análisis estático de Go, pruebas y build de React, verificación de comentarios y regeneración de OpenAPI. Una diferencia no confirmada en `backend/docs` bloquea la liberación.

## Artefactos y correspondencia

`deploy/build-release.sh` genera backend, frontend, OpenAPI, metadatos y `SHA256SUMS` desde una misma revisión. Si se proporciona `SIGNING_KEY`, firma los checksums con GPG. El frontend contiene `build-meta.json` y el backend expone `/api/v1/version`; el smoke test rechaza revisiones distintas.

## Monitoreo y rollback

`deploy/project-iiif-smoke-test` valida salud, versión, contrato OCR y correspondencia de artefactos. Debe ejecutarse después de desplegar y desde monitoreo periódico. Antes de reemplazar archivos se respaldan binario y `frontend/dist`; cualquier fallo en salud, login, catálogo o Configuración activa rollback.

## Flujo Git

Antes de promover una versión se comprueba que el árbol esté limpio y que las referencias remotas estén actualizadas. Los cambios se integran mediante revisión desde `development`; no se modifica `master` directamente y nunca se construye desde una rama local atrasada.
