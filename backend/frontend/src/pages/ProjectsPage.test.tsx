import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { api } from "../api";
import { sampleConfig } from "../test/fixtures";
import { ProjectsPage } from "./ProjectsPage";

it("guarda proyectos y sincroniza tenants desde el endpoint configurado", async () => {
  const user = userEvent.setup();
  const config = structuredClone(sampleConfig);
  config.projects = { enabled: true, default_project: "metavisor", require_project: true, allow_dynamic_tenants: false, items: [{ key: "metavisor", name: "Metavisor", multitenant: true, tenants: [], tenants_endpoint: "https://example.test/tenants" }] };
  vi.spyOn(api, "saveConfig").mockResolvedValue({ message: "ok" });
  const sync = vi.spyOn(api, "syncProjectTenants").mockResolvedValueOnce({ key: "metavisor", name: "Metavisor", multitenant: true, tenants: ["demo", "sunat"], tenants_endpoint: "https://example.test/tenants" });
  const onSaved = vi.fn();
  render(<ProjectsPage config={config} onSaved={onSaved} notify={vi.fn()} />);

  await user.click(screen.getByRole("button", { name: "Sincronizar tenants" }));

  expect(sync).toHaveBeenCalledWith("metavisor");
  expect(await screen.findByText("2 tenant(s) sincronizados para Metavisor.")).toBeInTheDocument();
  expect(onSaved).toHaveBeenLastCalledWith(expect.objectContaining({ projects: expect.objectContaining({ items: [expect.objectContaining({ tenants: ["demo", "sunat"] })] }) }));
});
