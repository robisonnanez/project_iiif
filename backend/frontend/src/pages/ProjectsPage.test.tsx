import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "../api";
import { sampleConfig } from "../test/fixtures";
import { ProjectsPage } from "./ProjectsPage";

it("guarda proyectos y sincroniza tenants desde el endpoint configurado", async () => {
  const user = userEvent.setup();
  const config = structuredClone(sampleConfig);
  config.projects = { enabled: true, default_project: "metavisor", require_project: true, allow_dynamic_tenants: false, items: [{ key: "metavisor", name: "Metavisor", multitenant: true, tenants: [], tenants_endpoint: "https://example.test/tenants", tenants_auth_type: "bearer", tenants_auth_header: "Authorization", tenants_auth_token: "********", tenants_token_configured: true }] };
  const saveConfig = vi.spyOn(api, "saveConfig").mockResolvedValue({ message: "ok" });
  const sync = vi.spyOn(api, "syncProjectTenants").mockResolvedValueOnce({ key: "metavisor", name: "Metavisor", multitenant: true, tenants: ["demo", "sunat"], tenants_endpoint: "https://example.test/tenants", tenants_auth_type: "bearer", tenants_auth_header: "Authorization", tenants_auth_token: "********", tenants_token_configured: true });
  const onSaved = vi.fn();
  render(<ProjectsPage config={config} onSaved={onSaved} notify={vi.fn()} />);

  const token = screen.getByLabelText("Bearer token");
  expect(token).toHaveAttribute("type", "password");
  expect(screen.getByText(/Ya existe un token guardado/)).toBeInTheDocument();
  await user.clear(token);
  await user.type(token, "nuevo-token");
  await user.click(screen.getByRole("button", { name: "Sincronizar tenants" }));

  expect(saveConfig).toHaveBeenCalledWith(expect.objectContaining({ projects: expect.objectContaining({ items: [expect.objectContaining({ tenants_auth_type: "bearer", tenants_auth_token: "nuevo-token" })] }) }));
  expect(sync).toHaveBeenCalledWith("metavisor");
  expect(await screen.findByText("2 tenant(s) sincronizados para Metavisor.")).toBeInTheDocument();
  expect(onSaved).toHaveBeenLastCalledWith(expect.objectContaining({ projects: expect.objectContaining({ items: [expect.objectContaining({ tenants: ["demo", "sunat"] })] }) }));
});
