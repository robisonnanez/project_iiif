import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { AppConfig, DocumentRecord, NoticeTone, Notify } from "./types";
import { Alert, Badge, Button, Card, EmptyState, Modal, PageHeader, Spinner } from "./components/ui";
import { ManifestDialog } from "./components/ManifestDialog";
import { ToastStack, type ToastNotice } from "./components/ToastStack";
import { ConfigPage } from "./pages/ConfigPage";
import { ImagesPage } from "./pages/ImagesPage";
import { MigrationPage } from "./pages/MigrationPage";
import { OCRPage } from "./pages/OCRPage";
import { ProjectsPage } from "./pages/ProjectsPage";
import { UploadPage } from "./pages/UploadPage";

type View = "dashboard" | "documents" | "iiif" | "upload" | "ocr" | "migration" | "projects" | "config";

const nav: Array<{ view: View; label: string; path: string }> = [
  { view: "dashboard", label: "Dashboard", path: "/dashboard/inicio" },
  { view: "documents", label: "Documentos", path: "/dashboard/documentos" },
  { view: "iiif", label: "Imágenes IIIF", path: "/dashboard/iiif" },
  { view: "ocr", label: "OCR", path: "/dashboard/ocr" },
  { view: "upload", label: "Subir PDF", path: "/dashboard/subir-pdf" },
  { view: "migration", label: "Migración", path: "/dashboard/migracion" },
  { view: "projects", label: "Proyectos", path: "/dashboard/proyectos" },
  { view: "config", label: "Configuración", path: "/dashboard/configuracion" },
];

export const browserNavigation = {
  toHome: () => window.location.replace("/"),
};

function viewFromPath(): View {
  if (location.pathname === "/dashboard/imagenes") return "iiif";
  return nav.find((item) => location.pathname === item.path)?.view ?? "dashboard";
}

export default function App() {
  const [view, setView] = useState<View>(viewFromPath);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [documents, setDocuments] = useState<DocumentRecord[]>([]);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [notices, setNotices] = useState<ToastNotice[]>([]);
  const noticeSequence = useRef(0);
  const previousStatuses = useRef(new Map<string, string>());
  const documentsInitialized = useRef(false);

  const dismissNotice = useCallback((id: number) => setNotices((current) => current.filter((notice) => notice.id !== id)), []);
  const notify = useCallback((message: string, tone: NoticeTone = "success") => {
    noticeSequence.current += 1;
    setNotices((current) => [...current, { id: noticeSequence.current, message, tone }].slice(-5));
  }, []);
  const applyDocuments = useCallback((next: DocumentRecord[]) => {
    if (documentsInitialized.current) {
      for (const document of next) {
        const previous = previousStatuses.current.get(document.id);
        if (previous === "processing" && document.status === "completed") notify(`“${document.name}” terminó de convertirse.`, "success");
        if (previous === "processing" && document.status === "error") notify(`La conversión de “${document.name}” terminó con error.`, "danger");
      }
    }
    previousStatuses.current = new Map(next.map((document) => [document.id, document.status]));
    documentsInitialized.current = true;
    setDocuments(next);
  }, [notify]);
  const refreshDocuments = useCallback(async (reportError = true) => {
    try { applyDocuments(await api.documents()); }
    catch (cause) { if (reportError) setError(cause instanceof Error ? cause.message : "No se pudieron cargar los documentos."); }
  }, [applyDocuments]);

  const refresh = useCallback(async () => {
    setLoading(true); setError("");
    const [documentsResult, configResult] = await Promise.allSettled([api.documents(), api.config()]);
    if (documentsResult.status === "fulfilled") applyDocuments(documentsResult.value); else setError(documentsResult.reason.message);
    if (configResult.status === "fulfilled") setConfig(configResult.value); else setError((current) => current || configResult.reason.message);
    setLoading(false);
  }, [applyDocuments]);

  useEffect(() => { void refresh(); }, [refresh]);
  useEffect(() => {
    const timer = window.setInterval(() => { if (!document.hidden) void refreshDocuments(false); }, 2500);
    return () => window.clearInterval(timer);
  }, [refreshDocuments]);
  useEffect(() => {
    const pop = () => setView(viewFromPath());
    addEventListener("popstate", pop);
    return () => removeEventListener("popstate", pop);
  }, []);

  const navigate = (item: (typeof nav)[number]) => {
    history.pushState({}, "", item.path); setView(item.view); setMobileOpen(false);
  };
  const logout = async () => {
    try { await api.logout(); }
    finally { browserNavigation.toHome(); }
  };
  const completed = documents.filter((document) => document.status === "completed").length;

  const menuOrientation = config?.frontend.menu_orientation === "vertical" ? "vertical" : "horizontal";
  return <div className={`app-shell layout-${menuOrientation}`}>
    <a className="skip-link" href="#main-content">Saltar al contenido</a>
    <header className="topbar">
      <button className="brand" onClick={() => navigate(nav[0])} aria-label="Ir al dashboard"><span className="brand-mark">IIIF</span><span><strong>project_iiif</strong><small>PDF · Image · Presentation</small></span></button>
      <Button className="menu-toggle" variant="ghost" aria-expanded={mobileOpen} aria-controls="primary-navigation" onClick={() => setMobileOpen((open) => !open)}>Menú</Button>
      <nav id="primary-navigation" className={mobileOpen ? "primary-nav open" : "primary-nav"} aria-label="Navegación principal">
        {nav.map((item) => <button key={item.view} className={view === item.view ? "nav-link active" : "nav-link"} aria-current={view === item.view ? "page" : undefined} onClick={() => navigate(item)}>{item.label}</button>)}
      </nav>
      <div className="topbar-actions"><a className="button button-ghost" href="/swagger/index.html">API</a><Badge tone={error ? "danger" : "success"}>{error ? "Atención" : "Sistema activo"}</Badge><Button variant="ghost" onClick={() => void logout()}>Salir</Button></div>
    </header>
    <main id="main-content" className="main-content">
      {error && <Alert tone="danger">{error}</Alert>}
      {loading && documents.length === 0 ? <div className="loading-page"><Spinner label="Cargando dashboard" /></div> : <>
        {view === "dashboard" && <Dashboard documents={documents} completed={completed} config={config} onRefresh={refresh} />}
        {view === "documents" && <DocumentsPage documents={documents} onRefresh={refresh} notify={notify} />}
        {view === "upload" && <UploadPage config={config} onUploaded={() => void refreshDocuments(false)} notify={notify} />}
        {view === "iiif" && <ImagesPage documents={documents} config={config} notify={notify} />}
        {view === "ocr" && <OCRPage documents={documents} config={config} notify={notify} />}
        {view === "migration" && <MigrationPage config={config} notify={notify} />}
        {view === "projects" && (config ? <ProjectsPage config={config} onSaved={setConfig} notify={notify} /> : <Alert tone="danger">No se pudo cargar la configuración de proyectos.</Alert>)}
        {view === "config" && (config ? <ConfigPage initial={config} onSaved={setConfig} /> : <Alert tone="danger">No se pudo cargar la configuración.</Alert>)}
      </>}
    </main>
    <ToastStack notices={notices} dismiss={dismissNotice} />
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

function DocumentsPage({ documents, onRefresh, notify }: { documents: DocumentRecord[]; onRefresh: () => void | Promise<void>; notify: Notify }) {
  const [selected, setSelected] = useState<DocumentRecord | null>(null);
  const [deleting, setDeleting] = useState<DocumentRecord | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const remove = async () => {
    if (!deleting) return;
    setDeleteBusy(true); setDeleteError("");
    try {
      await api.deleteDocument(deleting.id);
      const name = deleting.name;
      setDeleting(null);
      await onRefresh();
      notify(`“${name}” y todos sus archivos fueron eliminados.`, "success");
    } catch (cause) {
      setDeleteError(cause instanceof Error ? cause.message : "No se pudo eliminar el documento.");
    } finally { setDeleteBusy(false); }
  };
  return <>
    <PageHeader eyebrow="Biblioteca" title="Documentos" description="Consulta conversiones y genera manifests completos o parciales." actions={<Button variant="secondary" onClick={onRefresh}>Actualizar</Button>} />
    <Card><DocumentTable documents={documents} onManifest={setSelected} onDelete={(document) => { setDeleteError(""); setDeleting(document); }} /></Card>
    {selected && <ManifestDialog document={selected} onClose={() => setSelected(null)} />}
    {deleting && <Modal title="Eliminar documento" description="Esta acción elimina metadata, PDF, imágenes, miniaturas, manifests y artefactos OCR asociados." onClose={() => !deleteBusy && setDeleting(null)}>
      <p>¿Deseas eliminar definitivamente <strong>{deleting.name}</strong>?</p>
      <p className="table-secondary">ID: {deleting.id}{deleting.projectKey ? ` · Proyecto: ${deleting.projectKey}` : ""}{deleting.tenantKey ? ` · Tenant: ${deleting.tenantKey}` : ""}</p>
      {deleteError && <Alert tone="danger">{deleteError}</Alert>}
      <div className="modal-actions"><Button variant="secondary" disabled={deleteBusy} onClick={() => setDeleting(null)}>Cancelar</Button><Button variant="danger" disabled={deleteBusy} onClick={remove}>{deleteBusy ? <Spinner label="Eliminando" /> : "Eliminar definitivamente"}</Button></div>
    </Modal>}
  </>;
}

function DocumentTable({ documents, onManifest, onDelete, compact = false }: { documents: DocumentRecord[]; onManifest?: (document: DocumentRecord) => void; onDelete?: (document: DocumentRecord) => void; compact?: boolean }) {
  if (!documents.length) return <EmptyState title="Sin documentos" description="Sube un PDF para comenzar." />;
  return <div className="table-wrap"><table><thead><tr><th>Documento</th><th>Estado</th><th>Páginas</th>{!compact && <th>Origen</th>}{(onManifest || onDelete) && <th><span className="sr-only">Acciones</span></th>}</tr></thead><tbody>{documents.map((document) => <tr key={document.id}><td><strong>{document.name}</strong><small className="table-secondary">{document.id}</small></td><td><Badge tone={document.status === "completed" ? "success" : document.status === "error" ? "danger" : "warning"}>{document.status}</Badge></td><td>{document.convertedPages} / {document.totalPages}</td>{!compact && <td>{document.migratedFromLocal ? "Migrado" : "Subido"}</td>}{(onManifest || onDelete) && <td><div className="button-row">{onManifest && <Button variant="secondary" disabled={document.status !== "completed"} onClick={() => onManifest(document)}>Generar manifest</Button>}{onDelete && <Button variant="danger" onClick={() => onDelete(document)}>Eliminar</Button>}</div></td>}</tr>)}</tbody></table></div>;
}
