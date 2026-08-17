import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import type { AppConfig, Notify, UploadSettings } from "../types";
import { Alert, Button, Card, FormField, Input, Modal, PageHeader, Select, Spinner } from "../components/ui";

function validate(settings: UploadSettings): string {
  if (settings.width < 256 || settings.width > 8192) return "El ancho debe estar entre 256 y 8192 px.";
  if (settings.height < 256 || settings.height > 8192) return "El alto debe estar entre 256 y 8192 px.";
  if (settings.dpi < 72 || settings.dpi > 600) return "El DPI debe estar entre 72 y 600.";
  if (settings.quality < 1 || settings.quality > 100) return "La calidad debe estar entre 1 y 100.";
  return "";
}

export function UploadPage({ config, onUploaded, notify }: { config: AppConfig | null; onUploaded: () => void; notify: Notify }) {
  const [file, setFile] = useState<File | null>(null);
  const [modal, setModal] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const projects = config?.projects.items ?? [];
  const configuredDefault = config?.projects.default_project || projects[0]?.key || "default";
  const [project, setProject] = useState(configuredDefault);
  const [tenant, setTenant] = useState("");
  const selectedProject = projects.find((item) => item.key === project);
  const tenantRequired = Boolean(config?.projects.enabled && selectedProject?.multitenant);

  useEffect(() => {
    if (!config?.projects.enabled) return;
    const nextProject = projects.some((item) => item.key === project) ? project : configuredDefault;
    if (nextProject !== project) setProject(nextProject);
    const nextConfig = projects.find((item) => item.key === nextProject);
    if (!nextConfig?.multitenant) setTenant("");
  }, [config, configuredDefault, project, projects]);
  const defaults = useMemo<UploadSettings>(() => ({
    width: config?.conversion?.default_width || 1241,
    height: config?.conversion?.default_height || 1754,
    dpi: config?.conversion?.dpi || 150,
    format: config?.conversion?.default_format || "jpg",
    quality: config?.conversion?.default_quality || 85,
  }), [config]);
  const [settings, setSettings] = useState<UploadSettings>(defaults);

  const upload = async () => {
    const validationError = validate(settings);
    if (validationError) return setError(validationError);
    if (!file) return setError("Selecciona un archivo PDF.");
    if (config?.projects.enabled && config.projects.require_project && !project) return setError("Selecciona un proyecto.");
    if (tenantRequired && !tenant.trim()) return setError("Selecciona un tenant para el proyecto multitenant.");
    setBusy(true); setError("");
    try {
      const document = await api.upload(file, settings, { project, tenant: tenant.trim() });
      setMessage(`“${document.name}” se está convirtiendo.`);
      notify(`“${document.name}” se subió y comenzó a convertirse.`, "success");
      setModal(false); setFile(null); onUploaded();
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "No se pudo subir el PDF.";
      setError(message); notify(message, "danger");
    } finally { setBusy(false); }
  };

  return <>
    <PageHeader eyebrow="Documentos" title="Subir PDF" description="Convierte un PDF conservando su proporción y publícalo mediante IIIF." />
    {message && <Alert tone="success">{message}</Alert>}
    <Card className="narrow-card">
      {config?.projects.enabled && <div className="form-grid two-columns">
        <FormField label="Proyecto" help="Selecciona Proyecto por defecto si el documento no pertenece a un tenant.">
          {(id) => <Select id={id} value={project} onChange={(event) => { setProject(event.target.value); setTenant(""); setError(""); }}>
            {projects.map((item) => <option key={item.key} value={item.key}>{item.name || item.key}</option>)}
          </Select>}
        </FormField>
        {tenantRequired && <FormField label="Tenant" help={config.projects.allow_dynamic_tenants ? "Puedes usar un tenant configurado o escribir uno nuevo." : "Obligatorio para este proyecto."}>
          {(id) => config.projects.allow_dynamic_tenants
            ? <Input id={id} list="tenant-options" value={tenant} onChange={(event) => setTenant(event.target.value)} />
            : <Select id={id} value={tenant} onChange={(event) => setTenant(event.target.value)}><option value="">Selecciona un tenant</option>{selectedProject?.tenants.map((item) => <option key={item} value={item}>{item}</option>)}</Select>}
        </FormField>}
        {tenantRequired && config.projects.allow_dynamic_tenants && <datalist id="tenant-options">{selectedProject?.tenants.map((item) => <option key={item} value={item} />)}</datalist>}
      </div>}
      <FormField label="Archivo PDF" help="Solo PDF. El límite se toma de la configuración del servidor.">
        {(id) => <Input id={id} type="file" accept="application/pdf,.pdf" onChange={(event) => { setFile(event.target.files?.[0] ?? null); setError(""); }} />}
      </FormField>
      {error && !modal && <Alert tone="danger">{error}</Alert>}
      <Button disabled={!file} onClick={() => { setSettings(defaults); setModal(true); }}>Configurar y convertir</Button>
    </Card>
    {modal && <Modal title="Configuración de imágenes" description="Los valores son límites máximos; la página nunca se deforma." onClose={() => !busy && setModal(false)}>
      <div className="form-grid two-columns">
        <FormField label="Ancho máximo (px)">{(id) => <Input id={id} type="number" min="256" max="8192" value={settings.width} onChange={(e) => setSettings({ ...settings, width: Number(e.target.value) })} />}</FormField>
        <FormField label="Alto máximo (px)">{(id) => <Input id={id} type="number" min="256" max="8192" value={settings.height} onChange={(e) => setSettings({ ...settings, height: Number(e.target.value) })} />}</FormField>
        <FormField label="DPI">{(id) => <Input id={id} type="number" min="72" max="600" value={settings.dpi} onChange={(e) => setSettings({ ...settings, dpi: Number(e.target.value) })} />}</FormField>
        <FormField label="Formato">{(id) => <Select id={id} value={settings.format} onChange={(e) => setSettings({ ...settings, format: e.target.value as "jpg" | "png" })}><option value="jpg">JPG</option><option value="png">PNG</option></Select>}</FormField>
        <FormField label="Calidad JPG" help={settings.format === "png" ? "No se aplica a PNG." : undefined}>{(id) => <Input id={id} type="number" min="1" max="100" disabled={settings.format === "png"} value={settings.quality} onChange={(e) => setSettings({ ...settings, quality: Number(e.target.value) })} />}</FormField>
      </div>
      {error && <Alert tone="danger">{error}</Alert>}
      <div className="modal-actions"><Button variant="secondary" disabled={busy} onClick={() => setModal(false)}>Cancelar</Button><Button disabled={busy} onClick={upload}>{busy ? <Spinner label="Subiendo" /> : "Subir y convertir"}</Button></div>
    </Modal>}
  </>;
}
