import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { ScopeFields } from "../components/ScopeFields";
import { Badge, Button, Card, EmptyState, FormField, PageHeader, Select, Spinner } from "../components/ui";
import type { AppConfig, DocumentImage, DocumentRecord, Notify, ProjectConfig } from "../types";

const pageSize = 10;

export function ImagesPage({ documents, config, notify }: { documents: DocumentRecord[]; config: AppConfig | null; notify: Notify }) {
  const projects = useMemo<ProjectConfig[]>(() => config?.projects.items?.length ? config.projects.items : [{ key: "default", name: "Proyecto por defecto", multitenant: false, tenants: [] }], [config]);
  const [project, setProject] = useState("");
  const [tenant, setTenant] = useState("");
  const [documentId, setDocumentId] = useState("");
  const [images, setImages] = useState<DocumentImage[]>([]);
  const [page, setPage] = useState(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const filteredDocuments = useMemo(() => documents.filter((document) => {
    if (document.status !== "completed") return false;
    if (project && (document.projectKey || "default") !== project) return false;
    if (tenant && (document.tenantKey || "") !== tenant) return false;
    return true;
  }), [documents, project, tenant]);
  const filteredDocumentIds = filteredDocuments.map((document) => document.id).join("|");

  useEffect(() => {
    if (!filteredDocuments.some((document) => document.id === documentId)) {
      setDocumentId(filteredDocuments[0]?.id ?? "");
      setImages([]); setPage(1);
    }
  }, [documentId, filteredDocumentIds, filteredDocuments]);

  const changeProject = (value: string) => {
    setProject(value);
    const item = projects.find((candidate) => candidate.key === value);
    setTenant(item?.multitenant ? item.tenants[0] ?? "" : "");
    setImages([]); setPage(1);
  };
  const changeTenant = (value: string) => { setTenant(value); setImages([]); setPage(1); };

  const loadImages = async () => {
    if (!documentId) return;
    setBusy(true); setError("");
    try {
      const result = await api.documentImages(documentId);
      setImages(result.images ?? []); setPage(1);
      notify(`${result.images?.length ?? 0} imágenes IIIF cargadas.`, "success");
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "No se pudieron cargar las imágenes.";
      setError(message); notify(message, "danger");
    } finally { setBusy(false); }
  };

  const totalPages = Math.max(1, Math.ceil(images.length / pageSize));
  const visible = images.slice((page - 1) * pageSize, page * pageSize);

  return <>
    <PageHeader eyebrow="Image API" title="Imágenes IIIF" description="Filtra por proyecto y tenant, selecciona un documento y consulta cada página convertida." actions={<Button disabled={!documentId || busy} onClick={loadImages}>{busy ? <Spinner label="Cargando" /> : "Cargar imágenes"}</Button>} />
    <Card>
      <ScopeFields projects={projects} project={project} tenant={tenant} allowAll onProject={changeProject} onTenant={changeTenant} />
      <FormField label="Documento">
        {(id) => <Select id={id} value={documentId} disabled={!filteredDocuments.length} onChange={(event) => { setDocumentId(event.target.value); setImages([]); }}>
          {!filteredDocuments.length && <option value="">Sin documentos completados</option>}
          {filteredDocuments.map((document) => <option key={document.id} value={document.id}>{document.name}</option>)}
        </Select>}
      </FormField>
      {error && <div className="inline-error" role="alert">{error}</div>}
    </Card>
    <Card className="images-card">
      {!images.length ? <EmptyState title="Sin imágenes cargadas" description={documentId ? "Pulsa “Cargar imágenes” para consultar las páginas convertidas." : "Selecciona un documento completado."} /> : <>
        <div className="card-heading"><div><h2>Páginas convertidas</h2><p>{images.length} imágenes disponibles.</p></div><Badge tone="info">Página {page} de {totalPages}</Badge></div>
        <div className="table-wrap"><table className="images-table"><thead><tr><th>Miniatura</th><th>Página</th><th>ID imagen</th><th>Proyecto / Tenant</th><th>Migrada</th><th>Dimensiones</th><th>URL IIIF</th><th>Acción</th></tr></thead><tbody>
          {visible.map((image) => <tr key={image.image_id}>
            <td><a href={image.iiif_url} target="_blank" rel="noreferrer"><img className="image-thumb" src={image.iiif_url} alt={`Página ${image.page_number}`} loading="lazy" /></a></td>
            <td>{image.page_number}</td><td><code className="code-cell">{image.image_id}</code></td><td>{image.project_key || "default"}{image.tenant_key ? ` / ${image.tenant_key}` : ""}</td>
            <td><Badge tone={image.migrated_from_local ? "success" : "neutral"}>{image.migrated_from_local ? "Sí" : "No"}</Badge></td><td>{image.width} × {image.height} px</td>
            <td><a className="iiif-url" href={image.iiif_url} target="_blank" rel="noreferrer">{image.iiif_url}</a></td><td><a className="button button-secondary compact-button" href={image.iiif_url} target="_blank" rel="noreferrer">Abrir</a></td>
          </tr>)}
        </tbody></table></div>
        <Pagination page={page} total={totalPages} onPage={setPage} />
      </>}
    </Card>
  </>;
}

function Pagination({ page, total, onPage }: { page: number; total: number; onPage: (page: number) => void }) {
  const pages = Array.from({ length: total }, (_, index) => index + 1);
  return <nav className="pagination" aria-label="Paginación de imágenes">
    <Button variant="secondary" disabled={page === 1} onClick={() => onPage(1)}>Inicio</Button>
    <Button variant="secondary" disabled={page === 1} onClick={() => onPage(page - 1)}>Anterior</Button>
    {pages.map((item) => <Button key={item} variant={item === page ? "primary" : "secondary"} aria-current={item === page ? "page" : undefined} onClick={() => onPage(item)}>{item}</Button>)}
    <Button variant="secondary" disabled={page === total} onClick={() => onPage(page + 1)}>Siguiente</Button>
    <Button variant="secondary" disabled={page === total} onClick={() => onPage(total)}>Última</Button>
  </nav>;
}
