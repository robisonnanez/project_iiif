import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { ScopeFields } from "../components/ScopeFields";
import { Alert, Badge, Button, Card, EmptyState, FormField, Input, Modal, PageHeader, Select, Spinner } from "../components/ui";
import type { AppConfig, MigrationDirectory, MigrationPayload, MigrationStatus, Notify, ProjectConfig } from "../types";

type SourceType = "local" | "ssh" | "database";

export function MigrationPage({ config, notify }: { config: AppConfig | null; notify: Notify }) {
  const projects = useMemo<ProjectConfig[]>(() => config?.projects.items?.length ? config.projects.items : [{ key: "default", name: "Proyecto por defecto", multitenant: false, tenants: [] }], [config]);
  const [source, setSource] = useState<SourceType>("local");
  const [localPath, setLocalPath] = useState(config?.storage.data_path || "/var/lib/project_iiif");
  const [dirs, setDirs] = useState<MigrationDirectory[]>([]);
  const [host, setHost] = useState(""); const [port, setPort] = useState(22); const [user, setUser] = useState(""); const [sshPath, setSSHPath] = useState(""); const [privateKey, setPrivateKey] = useState("");
  const [project, setProject] = useState(config?.projects.default_project || "default"); const [tenant, setTenant] = useState("");
  const [status, setStatus] = useState<MigrationStatus | null>(null);
  const [error, setError] = useState(""); const [busy, setBusy] = useState(false); const [browsing, setBrowsing] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false); const [progressOpen, setProgressOpen] = useState(false);

  useEffect(() => {
    if (config?.storage.data_path) setLocalPath((current) => current === "/var/lib/project_iiif" ? config.storage.data_path : current);
    if (config?.projects.default_project && !projects.some((item) => item.key === project)) setProject(config.projects.default_project);
  }, [config, project, projects]);

  const loadStatus = useCallback(async () => {
    try { setStatus(await api.migrationStatus()); setError(""); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudo consultar la migración."); }
  }, []);
  useEffect(() => { void loadStatus(); }, [loadStatus]);
  useEffect(() => {
    if (!status?.running) return;
    const timer = window.setInterval(async () => {
      try {
        const next = await api.migrationStatus(); setStatus(next);
        if (!next.running) notify(next.exit_code === 0 ? "Migración completada correctamente." : "La migración terminó con errores.", next.exit_code === 0 ? "success" : "danger");
      } catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudo actualizar el progreso."); }
    }, 3000);
    return () => window.clearInterval(timer);
  }, [notify, status?.running]);

  const selectedProject = projects.find((item) => item.key === project);
  const backend = String(config?.storage.backend || config?.database.DB_CONNECTION || "base de datos");
  const dbLabel = backend === "postgres" ? "Postgres" : backend === "mysql" ? "MySQL" : backend.includes("mongo") ? "MongoDB" : backend;
  const usesS3 = config?.binary_storage.mode === "s3" || config?.s3.filesystem_disk === "s3";
  const title = usesS3 ? `Migración hacia S3 / RustFS con catálogo ${dbLabel}` : `Migración local a ${dbLabel} BLOB`;
  const description = usesS3 ? `Copia binarios locales, remotos o almacenados en ${dbLabel} hacia el bucket S3 configurado.` : `Migra metadatos y binarios hacia ${dbLabel}.`;

  const sourceError = () => {
    if (source === "local" && !localPath.trim()) return "La ruta local es obligatoria.";
    if (source === "ssh" && (!host.trim() || !user.trim() || !sshPath.trim() || !privateKey.trim())) return "Para SSH: host, usuario, ruta y llave privada son obligatorios.";
    return "";
  };
  const openConfirmation = () => {
    const message = sourceError();
    if (message) { setError(message); notify(message, "danger"); return; }
    setError(""); setConfirmOpen(true);
  };
  const sourcePreview = source === "database" ? "Base de datos activa (BLOB/GridFS) → S3/RustFS" : source === "ssh" ? `${user}@${host}:${sshPath}` : localPath;
  const changeProject = (value: string) => {
    setProject(value);
    const item = projects.find((candidate) => candidate.key === value);
    setTenant(item?.multitenant ? item.tenants[0] ?? "" : "");
  };
  const start = async () => {
    if (!project) { setError("Selecciona un proyecto para la migración."); return; }
    if (selectedProject?.multitenant && !tenant.trim()) { setError("Selecciona un tenant para el proyecto multitenant."); return; }
    const payload: MigrationPayload = { source: { type: source }, scope: { project_key: project, tenant_key: tenant.trim() } };
    if (source === "local") payload.source.local = { path: localPath.trim() };
    if (source === "ssh") payload.source.ssh = { host: host.trim(), port, user: user.trim(), path: sshPath.trim(), private_key: privateKey.trim() };
    setBusy(true); setError("");
    try {
      const result = await api.startMigration(payload); setStatus(result.status); setConfirmOpen(false); setProgressOpen(true); notify(result.message || "Migración iniciada.", "success");
    } catch (cause) { const message = cause instanceof Error ? cause.message : "No se pudo iniciar la migración."; setError(message); notify(message, "danger"); }
    finally { setBusy(false); }
  };
  const browse = async () => {
    setBrowsing(true); setError("");
    try { const result = await api.browseMigrationPath(localPath.trim()); setLocalPath(result.path); setDirs(result.dirs ?? []); }
    catch (cause) { const message = cause instanceof Error ? cause.message : "No se pudo explorar la ruta."; setError(message); notify(message, "danger"); }
    finally { setBrowsing(false); }
  };

  return <>
    <PageHeader eyebrow="Transferencias" title={title} description={description} actions={<><Button disabled={status?.running} onClick={openConfirmation}>Iniciar migración</Button><Button variant="secondary" onClick={() => void loadStatus()}>Actualizar estado</Button><Button variant="secondary" onClick={() => setProgressOpen(true)}>Ver progreso</Button></>} />
    {error && <Alert tone="danger">{error}</Alert>}
    <div className="migration-layout">
      <Card><div className="card-heading"><div><h2>Fuente de datos</h2><p>Selecciona el origen que se copiará al almacenamiento activo.</p></div></div>
        <FormField label="Tipo de origen">{(id) => <Select id={id} value={source} onChange={(event) => { setSource(event.target.value as SourceType); setError(""); }}><option value="local">Local</option><option value="ssh">Servidor remoto (SSH)</option><option value="database">Base de datos activa (BLOB/GridFS) a S3</option></Select>}</FormField>
        {source === "local" && <><div className="field-with-action"><FormField label="Ruta local">{(id) => <Input id={id} value={localPath} onChange={(event) => setLocalPath(event.target.value)} placeholder="/var/lib/project_iiif" />}</FormField><Button variant="secondary" disabled={browsing} onClick={browse}>{browsing ? <Spinner label="Explorando" /> : "Explorar directorios"}</Button></div>
          {dirs.length > 0 && <div className="table-wrap directory-table"><table><thead><tr><th>Directorio</th><th>Ruta</th><th>Acción</th></tr></thead><tbody>{dirs.map((dir) => <tr key={dir.path}><td>{dir.name}</td><td><code>{dir.path}</code></td><td><Button variant="secondary" onClick={() => setLocalPath(dir.path)}>Seleccionar</Button></td></tr>)}</tbody></table></div>}</>}
        {source === "ssh" && <><div className="form-grid two-columns"><FormField label="Host">{(id) => <Input id={id} value={host} onChange={(event) => setHost(event.target.value)} placeholder="172.21.227.83" />}</FormField><FormField label="Puerto">{(id) => <Input id={id} type="number" min="1" max="65535" value={port} onChange={(event) => setPort(Number(event.target.value) || 22)} />}</FormField><FormField label="Usuario">{(id) => <Input id={id} value={user} onChange={(event) => setUser(event.target.value)} placeholder="robison" />}</FormField><FormField label="Ruta base remota">{(id) => <Input id={id} value={sshPath} onChange={(event) => setSSHPath(event.target.value)} placeholder="/var/lib/project_iiif" />}</FormField></div><FormField label="Llave privada SSH">{(id) => <textarea id={id} className="input" rows={8} value={privateKey} onChange={(event) => setPrivateKey(event.target.value)} placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" autoComplete="off" />}</FormField></>}
        {source === "database" && <Alert>Se copiarán los BLOB/GridFS de la base activa hacia S3/RustFS.</Alert>}
      </Card>
      <Card><div className="card-heading"><div><h2>Estado</h2><p>Última ejecución registrada.</p></div><MigrationBadge status={status} /></div><MigrationSummary status={status} /></Card>
    </div>
    <Card className="migration-logs"><div className="card-heading"><div><h2>Logs</h2><p>Salida acumulada del proceso.</p></div></div><pre className="log-output">{status?.logs?.length ? status.logs.join("\n") : "Sin logs."}</pre></Card>
    {confirmOpen && <Modal title="Contexto de migración" description="Selecciona proyecto y tenant antes de iniciar la migración." onClose={() => !busy && setConfirmOpen(false)}><ScopeFields projects={projects} project={project} tenant={tenant} allowDynamic={Boolean(config?.projects.allow_dynamic_tenants)} onProject={changeProject} onTenant={setTenant} /><FormField label="Ruta origen">{(id) => <Input id={id} value={sourcePreview} readOnly />}</FormField>{error && <Alert tone="danger">{error}</Alert>}<div className="modal-actions"><Button variant="secondary" disabled={busy} onClick={() => setConfirmOpen(false)}>Cancelar</Button><Button disabled={busy} onClick={start}>{busy ? <Spinner label="Iniciando" /> : "Iniciar migración"}</Button></div></Modal>}
    {progressOpen && <Modal className="modal-wide" title="Progreso de migración" description={`${Math.max(0, Math.min(100, status?.progress_percent ?? 0))}% completado${status?.current_document ? ` — ${status.current_document}` : ""}`} onClose={() => setProgressOpen(false)}><MigrationProgress status={status} /><div className="modal-actions"><Button variant="secondary" onClick={() => setProgressOpen(false)}>Cerrar</Button></div></Modal>}
  </>;
}

function MigrationBadge({ status }: { status: MigrationStatus | null }) {
  if (!status || status.exit_code === -1 && !status.running) return <Badge>Sin ejecutar</Badge>;
  if (status.running) return <Badge tone="warning">En ejecución</Badge>;
  return <Badge tone={status.exit_code === 0 ? "success" : "danger"}>{status.exit_code === 0 ? "Completada" : "Con errores"}</Badge>;
}
function MigrationSummary({ status }: { status: MigrationStatus | null }) {
  if (!status) return <EmptyState title="Sin ejecuciones" description="Todavía no existe información de migración." />;
  return <dl className="summary-list"><div><dt>En ejecución</dt><dd>{status.running ? "Sí" : "No"}</dd></div><div><dt>Código de salida</dt><dd>{status.exit_code}</dd></div><div><dt>Inicio</dt><dd>{status.started_at ? new Date(status.started_at).toLocaleString() : "—"}</dd></div><div><dt>Fin</dt><dd>{status.finished_at ? new Date(status.finished_at).toLocaleString() : "—"}</dd></div><div><dt>Mensaje</dt><dd>{status.message || "—"}</dd></div></dl>;
}
function MigrationProgress({ status }: { status: MigrationStatus | null }) {
  const percent = Math.max(0, Math.min(100, status?.progress_percent ?? 0));
  const items = [...(status?.items ?? [])].sort((a, b) => rank(a.status) - rank(b.status));
  return <><div className="progress-track" aria-label={`Progreso ${percent}%`}><div className="progress-fill" style={{ width: `${percent}%` }} /></div>{!items.length ? <EmptyState title="Sin detalle por documento" description="El detalle aparecerá cuando comience el procesamiento." /> : <div className="table-wrap"><table><thead><tr><th>PDF</th><th>Imágenes</th><th>Estado</th><th>Mensaje</th></tr></thead><tbody>{items.map((item) => <tr key={`${item.document_id}-${item.pdf_name}`}><td>{item.pdf_name || item.document_id || "—"}</td><td>{item.images_done || 0} / {item.images_total || 0}</td><td><Badge tone={item.status === "ok" ? "success" : item.status === "error" ? "danger" : "warning"}>{item.status === "ok" ? "OK" : item.status || "ejecutando"}</Badge></td><td>{item.message || "—"}</td></tr>)}</tbody></table></div>}</>;
}
function rank(status: string) { return status === "running" ? 0 : status === "error" ? 1 : status === "ok" ? 2 : 3; }
