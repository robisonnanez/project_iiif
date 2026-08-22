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
