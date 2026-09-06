import type { AppConfig, DocumentRecord, OCRLanguage, OCRLanguageCatalog } from "../types";

// APIContractError identifica respuestas HTTP válidas cuyo cuerpo incumple el contrato esperado.
export class APIContractError extends Error {
  constructor(resource: string, detail: string) {
    super(`Respuesta inválida de ${resource}: ${detail}`);
    this.name = "APIContractError";
  }
}

// asRecord valida que un valor JSON sea un objeto no nulo y no un arreglo.
function asRecord(value: unknown, resource: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new APIContractError(resource, "se esperaba un objeto");
  return value as Record<string, unknown>;
}

// languageFromUnknown valida los campos que el frontend necesita para representar un idioma.
function languageFromUnknown(value: unknown, resource: string): OCRLanguage {
  const item = asRecord(value, resource);
  if (typeof item.code !== "string" || typeof item.name !== "string") throw new APIContractError(resource, "un idioma no contiene code y name válidos");
  return {
    code: item.code,
    name: item.name,
    package: typeof item.package === "string" ? item.package : undefined,
    installed: Boolean(item.installed),
    enabled: Boolean(item.enabled),
    detection_supported: Boolean(item.detection_supported),
  };
}

// languageArray conserva compatibilidad con colecciones nulas antiguas y rechaza otros tipos inválidos.
function languageArray(value: unknown, resource: string): OCRLanguage[] {
  if (value == null) return [];
  if (!Array.isArray(value)) throw new APIContractError(resource, "se esperaba un arreglo de idiomas");
  return value.map((item) => languageFromUnknown(item, resource));
}

// normalizeOCRLanguageCatalog valida el catálogo y garantiza colecciones seguras para React.
export function normalizeOCRLanguageCatalog(value: unknown): OCRLanguageCatalog {
  const catalog = asRecord(value, "catálogo OCR");
  if (typeof catalog.installation_enabled !== "boolean") throw new APIContractError("catálogo OCR", "installation_enabled no es booleano");
  return {
    installation_enabled: catalog.installation_enabled,
    installed: languageArray(catalog.installed, "catálogo OCR instalado"),
    available: languageArray(catalog.available, "catálogo OCR disponible"),
  };
}

// normalizeDocuments valida la colección y los campos mínimos consumidos por el dashboard.
export function normalizeDocuments(value: unknown): DocumentRecord[] {
  if (value == null) return [];
  if (!Array.isArray(value)) throw new APIContractError("documentos", "se esperaba un arreglo");
  return value.map((entry) => {
    const item = asRecord(entry, "documentos");
    if (typeof item.id !== "string" || typeof item.name !== "string" || typeof item.status !== "string") {
      throw new APIContractError("documentos", "un elemento no contiene id, name y status válidos");
    }
    return item as unknown as DocumentRecord;
  });
}

// validateConfig comprueba los bloques estructurales antes de que la normalización acceda a ellos.
export function validateConfig(value: unknown): AppConfig {
  const config = asRecord(value, "configuración");
  for (const section of ["server", "storage", "database", "frontend", "binary_storage", "s3", "iiif", "conversion", "ocr", "projects", "security"]) {
    asRecord(config[section], `configuración.${section}`);
  }
  asRecord(asRecord(config.database, "configuración.database").mongodb, "configuración.database.mongodb");
  return config as unknown as AppConfig;
}

// normalizeLanguageInstallation valida la envoltura devuelta después de instalar idiomas.
export function normalizeLanguageInstallation(value: unknown): { installed: string[]; catalog: OCRLanguageCatalog } {
  const result = asRecord(value, "instalación de idiomas OCR");
  if (!Array.isArray(result.installed) || !result.installed.every((code) => typeof code === "string")) {
    throw new APIContractError("instalación de idiomas OCR", "installed no es un arreglo de códigos");
  }
  return { installed: result.installed as string[], catalog: normalizeOCRLanguageCatalog(result.catalog) };
}
