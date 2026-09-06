import { describe, expect, it } from "vitest";
import { APIContractError, normalizeDocuments, normalizeOCRLanguageCatalog } from "./api-validation";

describe("contratos API en tiempo de ejecución", () => {
  it("normaliza colecciones OCR nulas por compatibilidad", () => {
    expect(normalizeOCRLanguageCatalog({ installation_enabled: false, installed: null, available: null })).toEqual({
      installation_enabled: false, installed: [], available: [],
    });
  });

  it("rechaza tipos de colección incompatibles", () => {
    expect(() => normalizeOCRLanguageCatalog({ installation_enabled: false, installed: {}, available: [] })).toThrow(APIContractError);
    expect(() => normalizeDocuments({})).toThrow(/se esperaba un arreglo/);
  });

  it("acepta documentos vacíos y poblados", () => {
    expect(normalizeDocuments([])).toEqual([]);
    expect(normalizeDocuments([{ id: "doc", name: "Documento", status: "completed", totalPages: 1, convertedPages: 1, migratedFromLocal: false }])).toHaveLength(1);
  });
});
