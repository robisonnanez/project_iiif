import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "../api";
import { sampleConfig } from "../test/fixtures";
import { MigrationPage } from "./MigrationPage";

it("restaura explorador, confirmación de scope, estado, logs y progreso", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "migrationStatus").mockResolvedValue({ running: false, exit_code: -1, logs: ["Sin logs."] });
  vi.spyOn(api, "browseMigrationPath").mockResolvedValueOnce({ path: "/var/lib/project_iiif", dirs: [{ name: "pdfs", path: "/var/lib/project_iiif/pdfs" }] });
  render(<MigrationPage config={structuredClone(sampleConfig)} notify={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "Explorar directorios" }));
  expect(await screen.findByText("/var/lib/project_iiif/pdfs")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Iniciar migración" }));
  expect(screen.getByRole("dialog", { name: "Contexto de migración" })).toBeInTheDocument();
  expect(screen.getByLabelText("Ruta origen")).toHaveValue("/var/lib/project_iiif");
  await user.click(screen.getByRole("button", { name: "Cancelar" }));
  await user.click(screen.getByRole("button", { name: "Ver progreso" }));
  expect(screen.getByRole("dialog", { name: "Progreso de migración" })).toBeInTheDocument();
  expect(screen.getByText("Sin detalle por documento")).toBeInTheDocument();
  expect(screen.getByText("Sin logs.")).toBeInTheDocument();
});
