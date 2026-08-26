import { useEffect, useState } from "react";
import { api } from "../api";
import { Alert, Button, Card, Checkbox, FormField, Input, PageHeader, Select, Spinner } from "../components/ui";
import type { AppConfig, Notify, ProjectConfig } from "../types";

type ProjectSettings = AppConfig["projects"];

export function ProjectsPage({ config, onSaved, notify }: { config: AppConfig; onSaved: (config: AppConfig) => void; notify: Notify }) {
  const [settings, setSettings] = useState<ProjectSettings>(() => structuredClone(config.projects));
  const [busy, setBusy] = useState(false);
  const [syncing, setSyncing] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => setSettings(structuredClone(config.projects)), [config.projects]);

  const updateProject = (index: number, change: Partial<ProjectConfig>) => setSettings((current) => ({ ...current, items: current.items.map((item, candidate) => candidate === index ? { ...item, ...change } : item) }));
  const addProject = () => {
    const used = new Set(settings.items.map((item) => item.key));
    let suffix = settings.items.length + 1;
    while (used.has(`proyecto-${suffix}`)) suffix += 1;
    setSettings((current) => ({ ...current, enabled: true, items: [...current.items, { key: `proyecto-${suffix}`, name: `Proyecto ${suffix}`, multitenant: false, tenants: [], tenants_endpoint: "", tenants_auth_type: "none", tenants_auth_header: "", tenants_auth_token: "" }] }));
  };
  const removeProject = (index: number) => setSettings((current) => {
    if (current.items.length === 1) return current;
    const removed = current.items[index];
    const items = current.items.filter((_, candidate) => candidate !== index);
    return { ...current, items, default_project: removed.key === current.default_project ? items[0].key : current.default_project };
  });
  const validate = () => {
    const keys = settings.items.map((item) => item.key.trim());
    if (keys.some((key) => !/^[A-Za-z0-9._-]{1,128}$/.test(key) || key === "." || key === "..")) return "Cada proyecto necesita una clave válida sin espacios.";
    if (new Set(keys.map((key) => key.toLowerCase())).size !== keys.length) return "Las claves de proyecto no pueden repetirse.";
    if (!keys.includes(settings.default_project)) return "Selecciona un proyecto predeterminado existente.";
    return "";
  };
  const persist = async (successMessage = true) => {
    const validation = validate();
    if (validation) throw new Error(validation);
    const projects = { ...settings, items: settings.items.map((item) => ({ ...item, key: item.key.trim(), name: item.name.trim() || item.key.trim(), tenants: [...new Set(item.tenants.map((tenant) => tenant.trim()).filter(Boolean))], tenants_endpoint: item.tenants_endpoint?.trim() || "", tenants_auth_type: item.tenants_auth_type || "none", tenants_auth_header: item.tenants_auth_header?.trim() || "", tenants_auth_token: item.tenants_auth_token || "" })) };
    const payload = { ...config, projects };
    await api.saveConfig(payload);
    setSettings(projects); onSaved(payload);
    if (successMessage) {
      setMessage("Proyectos guardados. La selección ya está disponible en cargas y migraciones.");
      notify("Configuración de proyectos actualizada.", "success");
    }
    return payload;
  };
  const save = async () => {
    setBusy(true); setError(""); setMessage("");
    try { await persist(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudieron guardar los proyectos."); }
    finally { setBusy(false); }
  };
  const sync = async (index: number) => {
    const project = settings.items[index];
    if (!project.tenants_endpoint?.trim()) { setError("Configura el endpoint de tenants antes de sincronizar."); return; }
    setSyncing(project.key); setError(""); setMessage("");
    try {
      const saved = await persist(false);
      const synced = await api.syncProjectTenants(project.key);
      const projects = { ...saved.projects, items: saved.projects.items.map((item) => item.key === synced.key ? synced : item) };
      setSettings(projects); onSaved({ ...saved, projects });
      const tenantLabel = synced.tenants.length === 1 ? "tenant sincronizado" : "tenants sincronizados";
      setMessage(`${synced.tenants.length} ${tenantLabel} para ${synced.name || synced.key}.`);
      notify(`Tenants de “${synced.name || synced.key}” sincronizados.`, "success");
    } catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudieron sincronizar los tenants."); }
    finally { setSyncing(""); }
  };

  return <>
    <PageHeader eyebrow="Organización" title="Proyectos y tenants" description="Administra los contextos disponibles para cargar documentos y ejecutar migraciones." actions={<><Button variant="secondary" onClick={addProject}>Agregar proyecto</Button><Button disabled={busy || Boolean(syncing)} onClick={save}>{busy ? <Spinner label="Guardando" /> : "Guardar proyectos"}</Button></>} />
    {message && <Alert tone="success">{message}</Alert>}{error && <Alert tone="danger">{error}</Alert>}
    <Card className="project-settings-card">
      <div className="form-grid two-columns">
        <Checkbox label="Activar proyectos" checked={settings.enabled} onChange={(event) => setSettings({ ...settings, enabled: event.target.checked })} />
        <Checkbox label="Exigir proyecto" checked={settings.require_project} disabled={!settings.enabled} onChange={(event) => setSettings({ ...settings, require_project: event.target.checked })} />
        <Checkbox label="Permitir tenants escritos manualmente" checked={settings.allow_dynamic_tenants} disabled={!settings.enabled} onChange={(event) => setSettings({ ...settings, allow_dynamic_tenants: event.target.checked })} />
        <FormField label="Proyecto predeterminado">{(id) => <Select id={id} value={settings.default_project} onChange={(event) => setSettings({ ...settings, default_project: event.target.value })}>{settings.items.map((item) => <option key={item.key} value={item.key}>{item.name || item.key}</option>)}</Select>}</FormField>
      </div>
    </Card>
    <div className="project-editor-grid">
      {settings.items.map((project, index) => <Card key={index} className="project-editor">
        <div className="card-heading"><div><span className="eyebrow">Proyecto {index + 1}</span><h2>{project.name || project.key}</h2></div><Button variant="danger" disabled={settings.items.length === 1 || Boolean(syncing)} onClick={() => removeProject(index)}>Eliminar</Button></div>
        <div className="form-grid two-columns">
          <FormField label="Clave" help="Se utiliza en rutas, filtros y almacenamiento.">{(id) => <Input id={id} value={project.key} onChange={(event) => updateProject(index, { key: event.target.value })} placeholder="metavisor" />}</FormField>
          <FormField label="Nombre visible">{(id) => <Input id={id} value={project.name} onChange={(event) => updateProject(index, { name: event.target.value })} placeholder="Metavisor" />}</FormField>
        </div>
        <Checkbox label="Proyecto multitenant" checked={project.multitenant} onChange={(event) => updateProject(index, { multitenant: event.target.checked, tenants: event.target.checked ? project.tenants : [] })} />
        {project.multitenant && <>
          <FormField label="Endpoint de tenants" help='Acepta JSON como ["tenant-a"] o {"tenants":[...]}.'>{(id) => <Input id={id} type="url" value={project.tenants_endpoint || ""} onChange={(event) => updateProject(index, { tenants_endpoint: event.target.value })} placeholder="https://api.ejemplo.com/tenants" />}</FormField>
          <div className="form-grid two-columns tenant-auth-grid">
            <FormField label="Autenticación del endpoint" help="Selecciona cómo autoriza la API la consulta de tenants.">{(id) => <Select id={id} value={project.tenants_auth_type || "none"} onChange={(event) => updateProject(index, { tenants_auth_type: event.target.value as ProjectConfig["tenants_auth_type"], tenants_auth_header: event.target.value === "bearer" ? "Authorization" : event.target.value === "api_key" ? (project.tenants_auth_header || "X-API-Key") : "", tenants_auth_token: event.target.value === "none" ? "" : project.tenants_auth_token })}><option value="none">Sin autenticación</option><option value="bearer">Bearer token</option><option value="api_key">API key en cabecera</option></Select>}</FormField>
            {project.tenants_auth_type === "api_key" && <FormField label="Nombre de la cabecera" help="Por ejemplo: X-API-Key o Authorization.">{(id) => <Input id={id} value={project.tenants_auth_header || "X-API-Key"} onChange={(event) => updateProject(index, { tenants_auth_header: event.target.value })} placeholder="X-API-Key" />}</FormField>}
            {(project.tenants_auth_type === "bearer" || project.tenants_auth_type === "api_key") && <FormField label={project.tenants_auth_type === "bearer" ? "Bearer token" : "API key"} help={project.tenants_token_configured ? "Ya existe un token guardado. Déjalo igual para conservarlo o escribe uno nuevo." : "Se guarda como secreto y nunca se devuelve completo desde la API."}>{(id) => <Input id={id} type="password" autoComplete="new-password" value={project.tenants_auth_token || ""} onChange={(event) => updateProject(index, { tenants_auth_token: event.target.value })} placeholder={project.tenants_token_configured ? "Token configurado" : "Pega aquí el token"} />}</FormField>}
          </div>
          <div className="tenant-sync-row"><Button variant="secondary" disabled={Boolean(syncing) || !project.tenants_endpoint?.trim()} onClick={() => sync(index)}>{syncing === project.key ? <Spinner label="Consultando endpoint" /> : "Sincronizar tenants"}</Button><span>{project.tenants.length} {project.tenants.length === 1 ? "tenant configurado" : "tenants configurados"}</span></div>
          <FormField label="Tenants" help="Uno por línea. Puedes editarlos manualmente aunque uses sincronización.">{(id) => <textarea id={id} className="input tenants-textarea" rows={6} value={project.tenants.join("\n")} onChange={(event) => updateProject(index, { tenants: event.target.value.split(/\r?\n|,/).map((tenant) => tenant.trim()).filter(Boolean) })} placeholder={"sunat\ndemo\nuniguajira"} />}</FormField>
        </>}
      </Card>)}
    </div>
  </>;
}
