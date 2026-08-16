import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { ManifestDialog } from "./ManifestDialog";

const document = { id: "doc-1", name: "Book.pdf", status: "completed", totalPages: 6, convertedPages: 6, migratedFromLocal: false };

it("genera una URL parcial normalizada", async () => {
  const user = userEvent.setup(); const open = vi.fn();
  render(<ManifestDialog document={document} onClose={() => undefined} onOpen={open} />);
  await user.click(screen.getByLabelText("Páginas seleccionadas"));
  await user.type(screen.getByLabelText("Páginas"), "1-3, 3,5");
  await user.click(screen.getByRole("button", { name: "Abrir manifest" }));
  expect(open).toHaveBeenCalledWith("/api/iiif/doc-1/manifest?pages=1%2C2%2C3%2C5");
});

it("muestra errores de páginas fuera del documento", async () => {
  const user = userEvent.setup();
  render(<ManifestDialog document={document} onClose={() => undefined} onOpen={() => undefined} />);
  await user.click(screen.getByLabelText("Páginas seleccionadas"));
  await user.type(screen.getByLabelText("Páginas"), "7");
  await user.click(screen.getByRole("button", { name: "Abrir manifest" }));
  expect(screen.getByText(/entre 1 y 6/)).toBeInTheDocument();
});
