import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "../api";
import { UploadPage } from "./UploadPage";
import { sampleConfig } from "../test/fixtures";

it("abre el modal con defaults y permite cancelar", async () => {
  const user = userEvent.setup();
  render(<UploadPage config={structuredClone(sampleConfig)} onUploaded={() => undefined} />);
  const file = new File(["pdf"], "book.pdf", { type: "application/pdf" });
  await user.upload(screen.getByLabelText("Archivo PDF"), file);
  await user.click(screen.getByRole("button", { name: "Configurar y convertir" }));
  expect(screen.getByRole("dialog", { name: "Configuración de imágenes" })).toBeInTheDocument();
  expect(screen.getByLabelText("Ancho máximo (px)")).toHaveValue(1241);
  await user.click(screen.getByRole("button", { name: "Cancelar" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("valida parámetros y presenta errores de la API", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "upload").mockRejectedValueOnce(new Error("PDF corrupto"));
  render(<UploadPage config={structuredClone(sampleConfig)} onUploaded={() => undefined} />);
  await user.upload(screen.getByLabelText("Archivo PDF"), new File(["pdf"], "book.pdf", { type: "application/pdf" }));
  await user.click(screen.getByRole("button", { name: "Configurar y convertir" }));
  await user.clear(screen.getByLabelText("DPI"));
  await user.type(screen.getByLabelText("DPI"), "20");
  await user.click(screen.getByRole("button", { name: "Subir y convertir" }));
  expect(screen.getByText(/DPI debe estar entre 72 y 600/)).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("DPI"), { target: { value: "150" } });
  await user.click(screen.getByRole("button", { name: "Subir y convertir" }));
  expect(await screen.findByText("PDF corrupto")).toBeInTheDocument();
});

it("envía el proyecto y tenant elegidos al subir", async () => {
  const user = userEvent.setup();
  const config = structuredClone(sampleConfig);
  config.projects = {
    enabled: true,
    default_project: "default",
    require_project: false,
    allow_dynamic_tenants: false,
    items: [
      { key: "default", name: "Proyecto por defecto", multitenant: false, tenants: [] },
      { key: "metavisor", name: "Metavisor", multitenant: true, tenants: ["sunat", "uniguajira"] },
    ],
  };
  const upload = vi.spyOn(api, "upload").mockResolvedValueOnce({
    id: "document-id", name: "book.pdf", migratedFromLocal: false, status: "processing", totalPages: 0, convertedPages: 0,
  });
  render(<UploadPage config={config} onUploaded={() => undefined} />);
  await user.selectOptions(screen.getByLabelText("Proyecto"), "metavisor");
  await user.selectOptions(screen.getByLabelText("Tenant"), "sunat");
  await user.upload(screen.getByLabelText("Archivo PDF"), new File(["pdf"], "book.pdf", { type: "application/pdf" }));
  await user.click(screen.getByRole("button", { name: "Configurar y convertir" }));
  await user.click(screen.getByRole("button", { name: "Subir y convertir" }));
  expect(upload).toHaveBeenCalledWith(expect.any(File), expect.any(Object), { project: "metavisor", tenant: "sunat" });
});
