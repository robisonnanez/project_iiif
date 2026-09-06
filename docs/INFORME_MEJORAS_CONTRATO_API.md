# Informe de mejora: contrato del catálogo de idiomas OCR

## Incidente

La vista `/dashboard/configuracion` podía quedar en blanco cuando `GET /api/v1/admin/ocr/languages` devolvía `available: null`. El backend dejaba slices Go sin inicializar cuando no había elementos y el frontend asumía que siempre recibiría arreglos.

La condición se manifestó en producción al estar instalados todos los paquetes de idiomas disponibles. El servidor de pruebas no la reproducía naturalmente porque aún tenía idiomas pendientes.

## Corrección aplicada

- El backend inicializa `installed` y `available` como slices vacíos y garantiza que JSON los represente como `[]`.
- El cliente HTTP normaliza respuestas históricas con colecciones nulas.
- Las vistas de Configuración y OCR aplican una defensa adicional antes de recorrer el catálogo.
- La interfaz muestra un estado vacío explícito cuando no existen idiomas pendientes.
- Las pruebas verifican el JSON serializado y los casos `null`, vacío y poblado.

No se cambiaron rutas, autenticación, configuración, Tesseract, base de datos ni mecanismos de instalación.

## Mejoras prioritarias para próximas versiones

1. Definir una política transversal: toda colección de una API pública debe serializarse como arreglo vacío y probarse sobre el JSON real.
2. Validar respuestas HTTP en tiempo de ejecución; los tipos TypeScript no validan datos recibidos.
3. Incorporar un Error Boundary por ruta para sustituir pantallas blancas por una vista recuperable.
4. Añadir a CI pruebas backend, frontend, build y contrato OpenAPI para valores nulos, vacíos y poblados.
5. Agregar pruebas end-to-end de las vistas administrativas con respuestas controladas.

## Mejoras operativas

- Exponer versión, commit y fecha de compilación en un endpoint de diagnóstico.
- Construir artefactos inmutables con checksums y asociar backend, frontend y OpenAPI a la misma versión.
- Añadir monitoreo sintético de las vistas administrativas y captura estructurada de errores del navegador.
- Mantener las ramas locales sincronizadas con sus referencias remotas antes de promover una versión.
- Documentar y ensayar el rollback del binario y del frontend antes de cada despliegue.

## Criterios recomendados de liberación

Una versión no debe promoverse si el health check, login, catálogo OCR, render de Configuración, pruebas automatizadas o correspondencia entre assets y binario no han sido verificados. Cualquier fallo en esos controles debe restaurar el binario y el directorio `dist` de la versión anterior.
