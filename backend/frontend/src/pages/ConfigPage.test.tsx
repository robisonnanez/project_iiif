import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "../api";
import { ConfigPage } from "./ConfigPage";
import { sampleConfig } from "../test/fixtures";

it("muestra idiomas del sistema e instala una selección sin habilitarla", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "ocrLanguages").mockResolvedValueOnce({ installation_enabled: true, installed: [{ code: "spa", name: "Español", installed: true, enabled: true, detection_supported: true }], available: [{ code: "deu", name: "Alemán", package: "tesseract-ocr-deu", installed: false, enabled: false, detection_supported: true }] });
  const install = vi.spyOn(api, "installOCRLanguages").mockResolvedValueOnce({ installed: ["deu"], catalog: { installation_enabled: true, installed: [{ code: "deu", name: "Alemán", package: "tesseract-ocr-deu", installed: true, enabled: false, detection_supported: true }], available: [] } });
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  await user.click(await screen.findByLabelText("Alemán (deu)"));
  await user.click(screen.getByRole("button", { name: "Instalar seleccionados" }));
  expect(install).toHaveBeenCalledWith(["deu"]);
  expect(await screen.findByText(/Idiomas instalados y verificados: deu/)).toBeInTheDocument();
});

it("muestra solo Mongo URI para MongoDB", async () => {
  const user = userEvent.setup();
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  await user.selectOptions(screen.getByLabelText("Motor"), "mongodb");
  expect(screen.getByLabelText("Mongo URI")).toBeInTheDocument();
  expect(screen.queryByLabelText("Host")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Usuario")).not.toBeInTheDocument();
});

it("muestra S3 únicamente cuando el modo es s3", async () => {
  const user = userEvent.setup();
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  expect(screen.queryByTestId("s3-fields")).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText("Modo"), "s3");
  expect(screen.getByTestId("s3-fields")).toBeInTheDocument();
  expect(screen.getByLabelText("Endpoint")).toBeInTheDocument();
});

it("presenta los defaults de conversión", () => {
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  expect(screen.getByLabelText("Ancho máximo")).toHaveValue(1241);
  expect(screen.getByLabelText("Alto máximo")).toHaveValue(1754);
  expect(screen.getByLabelText("DPI")).toHaveValue(150);
});

it("permite configurar el menú vertical", async () => {
  const user = userEvent.setup();
  const save = vi.spyOn(api, "saveConfig").mockResolvedValueOnce({ message: "ok" });
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  await user.selectOptions(screen.getByLabelText("Orientación del menú"), "vertical");
  await user.click(screen.getByRole("button", { name: "Guardar cambios" }));
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ frontend: expect.objectContaining({ menu_orientation: "vertical" }) }));
});

it("muestra y permite editar OCR y los orígenes CORS", async () => {
  const user = userEvent.setup();
  const save = vi.spyOn(api, "saveConfig").mockResolvedValueOnce({ message: "ok" });
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  expect(screen.getByRole("heading", { name: "OCR e indexación" })).toBeInTheDocument();
  expect(screen.getByLabelText("Procesos concurrentes")).toHaveValue(2);
  await user.click(screen.getByLabelText("Activar OCR"));
  await user.clear(screen.getByLabelText("Direcciones URL permitidas por CORS"));
  await user.type(screen.getByLabelText("Direcciones URL permitidas por CORS"), "https://app.example.com{enter}http://localhost:5173");
  await user.click(screen.getByRole("button", { name: "Guardar cambios" }));
  expect(save).toHaveBeenCalledWith(expect.objectContaining({
    ocr: expect.objectContaining({ enabled: true }),
    security: expect.objectContaining({ cors_origins: ["https://app.example.com", "http://localhost:5173"] }),
  }));
});

it("cambia entre los campos específicos de MySQL y PostgreSQL", async () => {
  const user = userEvent.setup();
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  expect(screen.queryByLabelText("SSL mode")).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText("Motor"), "postgres");
  expect(screen.getByLabelText("Host")).toHaveValue(sampleConfig.database.postgres.host);
  expect(screen.getByLabelText("SSL mode")).toBeInTheDocument();
});

it("muestra el error que devuelve la API al guardar", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "saveConfig").mockRejectedValueOnce(new Error("Configuración rechazada"));
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);
  await user.click(screen.getByRole("button", { name: "Guardar cambios" }));
  expect(await screen.findByText("Configuración rechazada")).toBeInTheDocument();
});

it("solicita la contraseña y reinicia el servicio después de guardar", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "saveConfig").mockResolvedValueOnce({ message: "ok" });
  const restart = vi.spyOn(api, "restartService").mockResolvedValueOnce({ ok: true, message: "ok", active: true });
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);

  await user.click(screen.getByRole("button", { name: "Guardar cambios" }));
  expect(await screen.findByRole("heading", { name: "Reiniciar servicio" })).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Reiniciar servicio" }));
  expect(screen.getByText("Ingresa la contraseña del servidor para reiniciar el servicio.")).toBeInTheDocument();

  await user.type(screen.getByLabelText("Contraseña del servidor"), "sudo-test");
  await user.click(screen.getByRole("button", { name: "Reiniciar servicio" }));

  expect(restart).toHaveBeenCalledWith("sudo-test");
  expect(await screen.findByText(/Reinicio en curso/)).toBeInTheDocument();
  expect(screen.queryByRole("heading", { name: "Reiniciar servicio" })).not.toBeInTheDocument();
});

it("permite ejecutar las migraciones del motor activo", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "dbMigrationStatus").mockResolvedValueOnce({ running: false, result: { engine: "mysql", pending_before: 0, applied: 0, skipped: 0, duration_ms: 0, message: "sin ejecutar" } });
  const run = vi.spyOn(api, "runDBMigrations").mockResolvedValueOnce({ engine: "mysql", pending_before: 2, applied: 2, skipped: 4, duration_ms: 25, message: "migraciones aplicadas" });
  render(<ConfigPage initial={structuredClone(sampleConfig)} onSaved={() => undefined} />);

  await user.click(screen.getByRole("button", { name: "Ejecutar migraciones ahora" }));

  expect(run).toHaveBeenCalledOnce();
  expect(await screen.findByText("2 migración(es) aplicadas en mysql.")).toBeInTheDocument();
  expect(screen.getByText(/Aplicadas: 2 · Omitidas: 4/)).toBeInTheDocument();
});
