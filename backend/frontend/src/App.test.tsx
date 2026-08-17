import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "./api";
import App from "./App";
import { sampleConfig } from "./test/fixtures";
import type { DocumentRecord } from "./types";

it("actualiza automáticamente el conteo de páginas y muestra una alerta al completar", async () => {
  history.replaceState({}, "", "/dashboard/documentos");
  const processing: DocumentRecord = { id: "book", name: "Libro.pdf", migratedFromLocal: false, status: "processing", totalPages: 2, convertedPages: 0 };
  const completed: DocumentRecord = { ...processing, status: "completed", convertedPages: 2 };
  vi.spyOn(api, "documents").mockResolvedValueOnce([processing]).mockResolvedValue([completed]);
  vi.spyOn(api, "config").mockResolvedValue(structuredClone(sampleConfig));
  render(<App />);
  expect(await screen.findByText("0 / 2")).toBeInTheDocument();
  expect(await screen.findByText("2 / 2", {}, { timeout: 3500 })).toBeInTheDocument();
  expect(screen.getByText("“Libro.pdf” terminó de convertirse.")).toBeInTheDocument();
}, 5000);

it("confirma y elimina un documento con todos sus archivos", async () => {
  history.replaceState({}, "", "/dashboard/documentos");
  const user = userEvent.setup();
  const document: DocumentRecord = { id: "book-id", name: "Libro.pdf", projectKey: "default", migratedFromLocal: false, status: "completed", totalPages: 2, convertedPages: 2 };
  vi.spyOn(api, "documents").mockResolvedValueOnce([document]).mockResolvedValue([]);
  vi.spyOn(api, "config").mockResolvedValue(structuredClone(sampleConfig));
  const remove = vi.spyOn(api, "deleteDocument").mockResolvedValueOnce({ message: "Documento eliminado" });
  render(<App />);

  expect(await screen.findByText("Libro.pdf")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Eliminar" }));
  expect(screen.getByRole("heading", { name: "Eliminar documento" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Eliminar definitivamente" }));

  expect(remove).toHaveBeenCalledWith("book-id");
  expect(await screen.findByText("Sin documentos")).toBeInTheDocument();
  expect(screen.getByText(/todos sus archivos fueron eliminados/)).toBeInTheDocument();
});
