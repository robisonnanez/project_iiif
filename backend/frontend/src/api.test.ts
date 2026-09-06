import { afterEach, expect, it, vi } from "vitest";
import { api } from "./api";

afterEach(() => vi.unstubAllGlobals());

it("normaliza a una lista vacía cuando una instalación nueva devuelve documentos null", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("null", {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })));

  await expect(api.documents()).resolves.toEqual([]);
});

it("normaliza colecciones nulas del catálogo OCR sin ocultar errores HTTP", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
    installation_enabled: false,
    installed: null,
    available: null,
  }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })));

  await expect(api.ocrLanguages()).resolves.toEqual({
    installation_enabled: false,
    installed: [],
    available: [],
  });
});

it("rechaza una configuración sin las secciones requeridas", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ server: {} }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })));

  await expect(api.config()).rejects.toThrow(/Respuesta inválida de configuración\.storage/);
});
