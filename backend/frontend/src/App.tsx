import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "./api";
import type { AppConfig, DocumentRecord } from "./types";
import { Alert, Badge, Button, Card, EmptyState, FormField, Input, PageHeader, Select, Spinner } from "./components/ui";
import { ManifestDialog } from "./components/ManifestDialog";
import { ConfigPage } from "./pages/ConfigPage";
import { UploadPage } from "./pages/UploadPage";

type View = "dashboard" | "documents" | "iiif" | "upload" | "migration" | "config";

const nav: Array<{ view: View; label: string; path: string }> = [
  { view: "dashboard", label: "Dashboard", path: "/dashboard/inicio" },
  { view: "documents", label: "Documentos", path: "/dashboard/documentos" },
  { view: "iiif", label: "IIIF", path: "/dashboard/iiif" },
  { view: "upload", label: "Subir PDF", path: "/dashboard/subir-pdf" },
  { view: "migration", label: "Migración", path: "/dashboard/migracion" },
  { view: "config", label: "Configuración", path: "/dashboard/configuracion" },
];

function viewFromPath(): View {
  return nav.find((item) => location.pathname === item.path)?.view ?? "dashboard";
}

export default function App() {
  const [view, setView] = useState<View>(viewFromPath);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [documents, setDocuments] = useState<DocumentRecord[]>([]);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true); setError("");
    const [documentsResult, configResult] = await Promise.allSettled([api.documents(), api.config()]);
    if (documentsResult.status === "fulfilled") setDocuments(documentsResult.value); else setError(documentsResult.reason.message);
    if (configResult.status === "fulfilled") setConfig(configResult.value); else setError((current) => current || configResult.reason.message);
    setLoading(false);
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);
  useEffect(() => {
    const pop = () => setView(viewFromPath());
    addEventListener("popstate", pop);
    return () => removeEventListener("popstate", pop);
  }, []);

  const navigate = (item: (typeof nav)[number]) => {
    history.pushState({}, "", item.path); setView(item.view); setMobileOpen(false);
  };
  const completed = documents.filter((document) => document.status === "completed").length;

  return <div className="app-shell">
    <a className="skip-link" href="#main-content">Saltar al contenido</a>
    <header className="topbar">
      <button className="brand" onClick={() => navigate(nav[0])} aria-label="Ir al dashboard"><span className="brand-mark">IIIF</span><span><strong>project_iiif</strong><small>PDF · Image · Presentation</small></span></button>
      <Button className="menu-toggle" variant="ghost" aria-expanded={mobileOpen} aria-controls="primary-navigation" onClick={() => setMobileOpen((open) => !open)}>Menú</Button>
      <nav id="primary-navigation" className={mobileOpen ? "primary-nav open" : "primary-nav"} aria-label="Navegación principal">
        {nav.map((item) => <button key={item.view} className={view === item.view ? "nav-link active" : "nav-link"} aria-current={view === item.view ? "page" : undefined} onClick={() => navigate(item)}>{item.label}</button>)}
      </nav>
      <div className="topbar-actions"><a className="button button-ghost" href="/swagger/index.html">API</a><Badge tone={error ? "danger" : "success"}>{error ? "Atención" : "Sistema activo"}</Badge><Button variant="ghost" onClick={async () => { await api.logout(); location.assign("/dashboard"); }}>Salir</Button></div>
    </header>
    <main id="main-content" className="main-content">
      {error && <Alert tone="danger">{error}</Alert>}
      {loading && documents.length === 0 ? <div className="loading-page"><Spinner label="Cargando dashboard" /></div> : <>
        {view === "dashboard" && <Dashboard documents={documents} completed={completed} config={config} onRefresh={refresh} />}
        {view === "documents" && <DocumentsPage documents={documents} onRefresh={refresh} />}
        {view === "iiif" && <IIIFPage documents={documents} />}
        {view === "upload" && <UploadPage config={config} onUploaded={refresh} />}
        {view === "migration" && <MigrationPage />}
        {view === "config" && (config ? <ConfigPage initial={config} onSaved={setConfig} /> : <Alert tone="danger">No se pudo cargar la configuración.</Alert>)}
      </>}
    </main>
  </div>;
}

function Dashboard({ documents, completed, config, onRefresh }: { documents: DocumentRecord[]; completed: number; config: AppConfig | null; onRefresh: () => void }) {
  return <>
    <PageHeader eyebrow="Resumen" title="Dashboard" description="Estado operativo del servidor PDF e IIIF." actions={<Button variant="secondary" onClick={onRefresh}>Actualizar</Button>} />
    <div className="metric-grid">
      <Card className="metric-card"><span>Documentos</span><strong>{documents.length}</strong><small>registrados</small></Card>
      <Card className="metric-card accent"><span>Completados</span><strong>{completed}</strong><small>listos para IIIF</small></Card>
      <Card className="metric-card"><span>Metadata</span><strong className="metric-word">{config?.storage.backend ?? "—"}</strong><small>motor activo</small></Card>
      <Card className="metric-card dark"><span>Binarios</span><strong className="metric-word">{config?.binary_storage.mode ?? "—"}</strong><small>modo activo</small></Card>
    </div>
    <Card><div className="card-heading"><div><h2>Últimos documentos</h2><p>Actividad reciente del sistema.</p></div></div><DocumentTable documents={documents.slice(0, 6)} compact /></Card>
  </>;
}

function DocumentsPage({ documents, onRefresh }: { documents: DocumentRecord[]; onRefresh: () => void }) {
  const [selected, setSelected] = useState<DocumentRecord | null>(null);
  return <>
    <PageHeader eyebrow="Biblioteca" title="Documentos" description="Consulta conversiones y genera manifests completos o parciales." actions={<Button variant="secondary" onClick={onRefresh}>Actualizar</Button>} />
    <Card><DocumentTable documents={documents} onManifest={setSelected} /></Card>
    {selected && <ManifestDialog document={selected} onClose={() => setSelected(null)} />}
  </>;
}

function DocumentTable({ documents, onManifest, compact = false }: { documents: DocumentRecord[]; onManifest?: (document: DocumentRecord) => void; compact?: boolean }) {
  if (!documents.length) return <EmptyState title="Sin documentos" description="Sube un PDF para comenzar." />;
  return <div className="table-wrap"><table><thead><tr><th>Documento</th><th>Estado</th><th>Páginas</th>{!compact && <th>Origen</th>}{onManifest && <th><span className="sr-only">Acciones</span></th>}</tr></thead><tbody>{documents.map((document) => <tr key={document.id}><td><strong>{document.name}</strong><small className="table-secondary">{document.id}</small></td><td><Badge tone={document.status === "completed" ? "success" : document.status === "error" ? "danger" : "warning"}>{document.status}</Badge></td><td>{document.convertedPages} / {document.totalPages}</td>{!compact && <td>{document.migratedFromLocal ? "Migrado" : "Subido"}</td>}{onManifest && <td><Button variant="secondary" disabled={document.status !== "completed"} onClick={() => onManifest(document)}>Generar manifest</Button></td>}</tr>)}</tbody></table></div>;
}

function IIIFPage({ documents }: { documents: DocumentRecord[] }) {
  const ready = documents.filter((document) => document.status === "completed");
  const [selectedId, setSelectedId] = useState(ready[0]?.id ?? "");
  const selected = useMemo(() => ready.find((document) => document.id === selectedId), [ready, selectedId]);
  const [dialog, setDialog] = useState(false);
  return <>
    <PageHeader eyebrow="Interoperabilidad" title="IIIF" description="Publica Presentation API 2.1 y conserva una ruta compatible con v3." />
    <Card className="narrow-card">
      {ready.length ? <><FormField label="Documento">{(id) => <Select id={id} value={selectedId} onChange={(e) => setSelectedId(e.target.value)}>{ready.map((doc) => <option key={doc.id} value={doc.id}>{doc.name}</option>)}</Select>}</FormField><div className="button-row"><Button onClick={() => setDialog(true)}>Generar manifest</Button><a className="button button-secondary" href={selected ? `/api/iiif/v3/${encodeURIComponent(selected.id)}/manifest` : "#"} target="_blank" rel="noreferrer">Manifest v3</a></div></> : <EmptyState title="No hay documentos listos" description="Completa una conversión antes de generar manifests." />}
    </Card>
    {dialog && selected && <ManifestDialog document={selected} onClose={() => setDialog(false)} />}
  </>;
}

function MigrationPage() {
  const [source, setSource] = useState<"local" | "database" | "ssh">("local");
  const [path, setPath] = useState("./data");
  const [host, setHost] = useState(""); const [user, setUser] = useState(""); const [privateKey, setPrivateKey] = useState("");
  const [busy, setBusy] = useState(false); const [message, setMessage] = useState(""); const [error, setError] = useState("");
  const start = async () => {
    setBusy(true); setError("");
    try {
      const result = await api.startMigration({ source: { type: source, local: { path }, ssh: { host, port: 22, user, path, private_key: privateKey } }, scope: { project_key: "default", tenant_key: "" } });
      setMessage(result.message || "Migración iniciada.");
    } catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudo iniciar."); } finally { setBusy(false); }
  };
  return <><PageHeader eyebrow="Transferencias" title="Migración" description="Migra almacenamiento local, base de datos o un servidor SSH hacia el destino configurado." />{message && <Alert tone="success">{message}</Alert>}{error && <Alert tone="danger">{error}</Alert>}<Card className="narrow-card"><FormField label="Origen">{(id) => <Select id={id} value={source} onChange={(e) => setSource(e.target.value as typeof source)}><option value="local">Almacenamiento local</option><option value="database">Base de datos activa</option><option value="ssh">Servidor SSH</option></Select>}</FormField>{source !== "database" && <FormField label={source === "ssh" ? "Ruta remota" : "Ruta local"}>{(id) => <Input id={id} value={path} onChange={(e) => setPath(e.target.value)} />}</FormField>}{source === "ssh" && <><FormField label="Host">{(id) => <Input id={id} value={host} onChange={(e) => setHost(e.target.value)} />}</FormField><FormField label="Usuario">{(id) => <Input id={id} value={user} onChange={(e) => setUser(e.target.value)} />}</FormField><FormField label="Llave privada">{(id) => <textarea id={id} className="input" rows={6} value={privateKey} onChange={(e) => setPrivateKey(e.target.value)} autoComplete="off" />}</FormField></>}<Button disabled={busy} onClick={start}>{busy ? <Spinner label="Iniciando" /> : "Iniciar migración"}</Button></Card></>;
}
