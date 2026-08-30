import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "../api";
import { sampleConfig } from "../test/fixtures";
import type { DocumentRecord } from "../types";
import { OCRPage } from "./OCRPage";

it("abre la imagen IIIF real y explica dónde se guardan los bbox", async () => {
  const user = userEvent.setup();
  const document: DocumentRecord = { id: "doc-1", name: "Libro.pdf", migratedFromLocal: false, status: "completed", totalPages: 1, convertedPages: 1 };
  vi.spyOn(api, "searchOCR").mockResolvedValueOnce({ total: 1, limit: 100, offset: 0, results: [{ document_id: "doc-1", page_number: 1, canvas_v2: "http://old/canvas", canvas_v3: "http://old/canvas/1", image_id: "74a59d4b-2094-423f-8473-124d4be5b45e", source: "ocr", snippet: "texto encontrado", score: 1, matches: 1 }] });
  const config = structuredClone(sampleConfig);
  config.ocr.enabled = true;
  render(<OCRPage documents={[document]} config={config} notify={() => undefined} />);

  expect(screen.getByRole("status")).toHaveTextContent("Cada palabra reconocida incluye");
  await user.type(screen.getByLabelText("Texto"), "texto");
  await user.click(screen.getByRole("button", { name: "Buscar" }));

  const link = await screen.findByRole("link", { name: "Abrir imagen IIIF" });
  expect(link).toHaveAttribute("href", "/iiif/3/74a59d4b-2094-423f-8473-124d4be5b45e/full/max/0/default.jpg");
});

it("autocompleta con debounce y permite seleccionar con teclado", async () => {
  const user = userEvent.setup();
  const document: DocumentRecord = { id: "doc-1", name: "Libro.pdf", projectKey: "project-a", tenantKey: "tenant-a", migratedFromLocal: false, status: "completed", totalPages: 1, convertedPages: 1 };
  const autocomplete = vi.spyOn(api, "autocompleteOCR").mockResolvedValue({ query: "func", items: [{ text: "funciones", frequency: 12 }, { text: "funcionalidad", frequency: 5 }] });
  const config = structuredClone(sampleConfig);
  config.ocr.enabled = true;
  render(<OCRPage documents={[document]} config={config} notify={() => undefined} />);

  const input = screen.getByRole("combobox", { name: "Texto" });
  await user.type(input, "func");
  await waitFor(() => expect(autocomplete).toHaveBeenCalledTimes(1), { timeout: 1000 });
  expect(autocomplete).toHaveBeenCalledWith("func", "doc-1", undefined, undefined, expect.any(AbortSignal));
  expect(await screen.findByRole("option", { name: /funciones/ })).toBeVisible();

  await user.keyboard("{ArrowDown}{Enter}");
  expect(input).toHaveValue("funciones");
  expect(input).toHaveAttribute("aria-expanded", "false");
});
