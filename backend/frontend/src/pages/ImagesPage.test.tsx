import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "../api";
import { sampleConfig } from "../test/fixtures";
import type { DocumentRecord } from "../types";
import { ImagesPage } from "./ImagesPage";

it("filtra por proyecto y tenant y muestra la tabla de miniaturas IIIF", async () => {
  const user = userEvent.setup();
  const config = structuredClone(sampleConfig);
  config.projects = { enabled: true, default_project: "default", require_project: false, allow_dynamic_tenants: false, items: [
    { key: "default", name: "Proyecto por defecto", multitenant: false, tenants: [] },
    { key: "metavisor", name: "Metavisor", multitenant: true, tenants: ["sunat", "uniguajira"] },
  ] };
  const documents: DocumentRecord[] = [{ id: "book", name: "Libro.pdf", projectKey: "metavisor", tenantKey: "sunat", migratedFromLocal: false, status: "completed", totalPages: 1, convertedPages: 1 }];
  vi.spyOn(api, "documentImages").mockResolvedValueOnce({ document_id: "book", project_key: "metavisor", tenant_key: "sunat", images: [{ image_id: "image-1", document_id: "book", project_key: "metavisor", tenant_key: "sunat", page_number: 1, width: 800, height: 1200, format: "jpg", media_type: "image/jpeg", byte_size: 10, migrated_from_local: false, iiif_url: "http://example.test/iiif/3/image-1/full/max/0/default.jpg", info_url: "http://example.test/iiif/3/image-1/info.json" }] });
  const view = render(<ImagesPage documents={documents} config={config} notify={vi.fn()} />);
  await user.selectOptions(screen.getByLabelText("Proyecto"), "metavisor");
  expect(screen.getByLabelText("Tenant")).toHaveValue("sunat");
  expect(screen.getByLabelText("Documento")).toHaveValue("book");
  await user.click(screen.getByRole("button", { name: "Cargar imágenes" }));
  expect(await screen.findByAltText("Página 1")).toBeInTheDocument();
  expect(screen.getByText("800 × 1200 px")).toBeInTheDocument();
  view.rerender(<ImagesPage documents={[{ ...documents[0], convertedPages: 1 }]} config={config} notify={vi.fn()} />);
  expect(screen.getByAltText("Página 1")).toBeInTheDocument();
});
