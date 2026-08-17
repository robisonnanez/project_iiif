import type { ProjectConfig } from "../types";
import { FormField, Input, Select } from "./ui";

export function ScopeFields({ projects, project, tenant, allowAll = false, allowDynamic = false, onProject, onTenant }: {
  projects: ProjectConfig[];
  project: string;
  tenant: string;
  allowAll?: boolean;
  allowDynamic?: boolean;
  onProject: (value: string) => void;
  onTenant: (value: string) => void;
}) {
  const selected = projects.find((item) => item.key === project);
  return <div className="form-grid two-columns scope-fields">
    <FormField label="Proyecto">
      {(id) => <Select id={id} value={project} onChange={(event) => onProject(event.target.value)}>
        {allowAll && <option value="">Todos los proyectos</option>}
        {projects.map((item) => <option key={item.key} value={item.key}>{item.name || item.key}</option>)}
      </Select>}
    </FormField>
    {selected?.multitenant && <FormField label="Tenant" help="Obligatorio para este proyecto.">
      {(id) => allowDynamic
        ? <Input id={id} list="scope-tenant-options" value={tenant} onChange={(event) => onTenant(event.target.value)} />
        : <Select id={id} value={tenant} onChange={(event) => onTenant(event.target.value)}><option value="">Selecciona un tenant</option>{selected.tenants.map((item) => <option key={item} value={item}>{item}</option>)}</Select>}
    </FormField>}
    {selected?.multitenant && allowDynamic && <datalist id="scope-tenant-options">{selected.tenants.map((item) => <option key={item} value={item} />)}</datalist>}
  </div>;
}
