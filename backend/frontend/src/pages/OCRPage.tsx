import { useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import { api } from "../api";
import { Alert, Badge, Button, Card, EmptyState, PageHeader } from "../components/ui";
import type { AppConfig, DocumentRecord, Notify, OCRAutocompleteItem, OCRJob, OCRSearchResult } from "../types";

const terminal = new Set(["completed", "completed_with_errors", "failed", "cancelled"]);

export function OCRPage({ documents, config, notify }: { documents: DocumentRecord[]; config: AppConfig | null; notify: Notify }) {
  const ready = useMemo(() => documents.filter((item) => item.status === "completed"), [documents]);
  const [documentId, setDocumentId] = useState("");
  const [mode, setMode] = useState("hybrid");
  const [languageMode, setLanguageMode] = useState("auto");
  const [languages, setLanguages] = useState<string[]>(["spa"]);
  const [force, setForce] = useState(false);
  const [job, setJob] = useState<OCRJob | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [scope, setScope] = useState("document");
  const [results, setResults] = useState<OCRSearchResult[]>([]);
  const [searched, setSearched] = useState(false);
  const [suggestions, setSuggestions] = useState<OCRAutocompleteItem[]>([]);
  const [suggestionsOpen, setSuggestionsOpen] = useState(false);
  const [activeSuggestion, setActiveSuggestion] = useState(-1);
  const skipAutocomplete = useRef(false);

  useEffect(() => { if (!documentId && ready.length) setDocumentId(ready[0].id); }, [documentId, ready]);
  useEffect(() => {
    if (!job || terminal.has(job.status)) return;
    const timer = window.setInterval(async () => {
      try {
        const next = await api.ocrJob(job.id); setJob(next);
        if (terminal.has(next.status)) notify(next.status.startsWith("completed") ? "OCR finalizado e indexado." : `OCR terminó: ${next.status}`, next.status.startsWith("completed") ? "success" : "danger");
      } catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudo actualizar el trabajo OCR."); }
    }, 1500);
    return () => window.clearInterval(timer);
  }, [job, notify]);
  useEffect(() => {
    if (skipAutocomplete.current) {
      skipAutocomplete.current = false;
      return;
    }
    const prefix = query.trim();
    if (prefix.length < 2) {
      setSuggestions([]); setSuggestionsOpen(false); setActiveSuggestion(-1);
      return;
    }
    const selected = ready.find((item) => item.id === documentId);
    const controller = new AbortController();
    let current = true;
    const timer = window.setTimeout(async () => {
      try {
        const response = await api.autocompleteOCR(prefix, scope === "document" ? documentId : undefined, scope === "project" ? selected?.projectKey : undefined, scope === "project" ? selected?.tenantKey : undefined, controller.signal);
        if (!current) return;
        setSuggestions(response.items); setSuggestionsOpen(response.items.length > 0); setActiveSuggestion(-1);
      } catch (cause) {
        if (current && !(cause instanceof DOMException && cause.name === "AbortError")) {
          setSuggestions([]); setSuggestionsOpen(false); setActiveSuggestion(-1);
        }
      }
    }, 300);
    return () => { current = false; controller.abort(); window.clearTimeout(timer); };
  }, [documentId, query, ready, scope]);

  const toggleLanguage = (language: string) => setLanguages((current) => current.includes(language) ? current.filter((item) => item !== language) : [...current, language]);
  const start = async () => {
    if (!documentId) return; setBusy(true); setError("");
    try { setJob(await api.startOCR(documentId, { mode, language_mode: languageMode, languages: languageMode === "manual" ? languages : [], force })); notify("Trabajo OCR agregado a la cola."); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudo iniciar OCR."); }
    finally { setBusy(false); }
  };
  const search = async () => {
    if (query.trim().length < 2) { setError("Escribe al menos dos caracteres para buscar."); return; }
    setBusy(true); setError(""); setSearched(true);
    try {
      const selected = ready.find((item) => item.id === documentId);
      const response = await api.searchOCR(query, scope === "document" ? documentId : undefined, scope === "project" ? selected?.projectKey : undefined, scope === "project" ? selected?.tenantKey : undefined);
      setResults(response.results);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "No se pudo buscar en OCR."); setResults([]); }
    finally { setBusy(false); }
  };
  const chooseSuggestion = (item: OCRAutocompleteItem) => {
    skipAutocomplete.current = true; setQuery(item.text); setSuggestions([]); setSuggestionsOpen(false); setActiveSuggestion(-1);
  };
  const handleQueryKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown" && suggestions.length) {
      event.preventDefault(); setSuggestionsOpen(true); setActiveSuggestion((current) => Math.min(current + 1, suggestions.length - 1)); return;
    }
    if (event.key === "ArrowUp" && suggestions.length) {
      event.preventDefault(); setSuggestionsOpen(true); setActiveSuggestion((current) => current <= 0 ? suggestions.length - 1 : current - 1); return;
    }
    if (event.key === "Escape") {
      event.preventDefault(); setSuggestionsOpen(false); setActiveSuggestion(-1); return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      if (suggestionsOpen && activeSuggestion >= 0) chooseSuggestion(suggestions[activeSuggestion]);
      else void search();
    }
  };
  const progress = job?.total_pages ? Math.round(job.processed_pages * 100 / job.total_pages) : 0;

  return <>
    <PageHeader eyebrow="Texto indexado" title="OCR por página" description="Extrae texto de todas las páginas y localiza cada coincidencia en su Canvas IIIF." />
    {!config?.ocr?.enabled && <Alert tone="info">OCR está desactivado. Activa <code>ocr.enabled: true</code> en <code>config.yaml</code> y reinicia el servicio.</Alert>}
    {config?.ocr?.enabled && <Alert tone="info">Los resultados OCR se guardan comprimidos en <code>{config.storage.data_path.replace(/\/$/, "")}/ocr/pages/&lt;documento&gt;/&lt;generación&gt;/000001.json.gz</code>. Cada palabra nueva incluye <code>bbox.x0</code>, <code>x1</code>, <code>y0</code> y <code>y1</code> en coordenadas del Canvas IIIF. Los OCR históricos sin geometría conservan <code>geometry_status: page_only</code> hasta ser reprocesados.</Alert>}
    {error && <Alert tone="danger">{error}</Alert>}
    <div className="ocr-layout">
      <Card>
        <div className="card-heading"><div><h2>Nuevo procesamiento</h2><p>El modo híbrido usa la capa de texto y aplica Tesseract a las páginas escaneadas.</p></div></div>
        <div className="form-field"><label htmlFor="ocr-document">Documento</label><select id="ocr-document" className="input" value={documentId} onChange={(event) => setDocumentId(event.target.value)}>{ready.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.projectKey || "default"}{item.tenantKey ? ` / ${item.tenantKey}` : ""}</option>)}</select></div>
        <div className="form-grid two-columns">
          <div className="form-field"><label htmlFor="ocr-mode">Cobertura</label><select id="ocr-mode" className="input" value={mode} onChange={(event) => setMode(event.target.value)}><option value="hybrid">Híbrido por página</option><option value="exhaustive">Exhaustivo</option><option value="ocr_only">Solo OCR</option></select></div>
          <div className="form-field"><label htmlFor="ocr-language-mode">Idiomas</label><select id="ocr-language-mode" className="input" value={languageMode} onChange={(event) => setLanguageMode(event.target.value)}><option value="auto">Detectar automáticamente</option><option value="manual">Seleccionar manualmente</option></select></div>
        </div>
        {languageMode === "manual" && <div className="language-options">{["spa", "eng", "fra", "por"].map((language) => <label className="checkbox" key={language}><input type="checkbox" checked={languages.includes(language)} onChange={() => toggleLanguage(language)} />{language}</label>)}</div>}
        <label className="checkbox"><input type="checkbox" checked={force} onChange={(event) => setForce(event.target.checked)} />Crear una nueva generación aunque ya exista OCR</label>
        <Button disabled={!config?.ocr?.enabled || !documentId || busy || (!!job && !terminal.has(job.status))} onClick={start}>{busy ? "Procesando…" : "Iniciar OCR"}</Button>
        {job && <div className="job-progress"><div className="job-progress-heading"><strong>{job.status.replaceAll("_", " ")}</strong><span>{job.processed_pages} / {job.total_pages} páginas</span></div><div className="progress-track"><div className="progress-fill" style={{ width: `${progress}%` }} /></div>{job.error && <p className="inline-error">{job.error}</p>}{!terminal.has(job.status) && <Button variant="danger" onClick={async () => setJob(await api.cancelOCR(job.id))}>Cancelar</Button>}</div>}
      </Card>
      <Card>
        <div className="card-heading"><div><h2>Buscar texto</h2><p>Los resultados siempre indican documento, página y Canvas.</p></div></div>
        <div className="form-field autocomplete-field"><label htmlFor="ocr-query">Texto</label><input id="ocr-query" className="input" role="combobox" aria-autocomplete="list" aria-expanded={suggestionsOpen} aria-controls="ocr-suggestions" aria-activedescendant={activeSuggestion >= 0 ? `ocr-suggestion-${activeSuggestion}` : undefined} value={query} onChange={(event) => { setQuery(event.target.value); setSuggestionsOpen(true); }} onFocus={() => suggestions.length && setSuggestionsOpen(true)} onBlur={() => setSuggestionsOpen(false)} onKeyDown={handleQueryKeyDown} placeholder="Ej. patrimonio cultural" autoComplete="off" />
          {suggestionsOpen && <ul id="ocr-suggestions" className="autocomplete-list" role="listbox">{suggestions.map((item, index) => <li id={`ocr-suggestion-${index}`} className={index === activeSuggestion ? "active" : ""} role="option" aria-selected={index === activeSuggestion} key={item.text} onMouseDown={(event) => event.preventDefault()} onClick={() => chooseSuggestion(item)}><span>{item.text}</span><small>{item.frequency} aparición{item.frequency === 1 ? "" : "es"}</small></li>)}</ul>}
        </div>
        <div className="segmented"><label><input type="radio" checked={scope === "document"} onChange={() => setScope("document")} />Documento seleccionado</label><label><input type="radio" checked={scope === "project"} onChange={() => setScope("project")} />Proyecto / tenant</label></div>
        <Button disabled={busy || !documentId} onClick={search}>Buscar</Button>
      </Card>
    </div>
    <Card className="ocr-results"><div className="card-heading"><div><h2>Coincidencias</h2><p>{searched ? `${results.length} páginas encontradas` : "Ejecuta una búsqueda para ver las páginas."}</p></div></div>{searched && !results.length ? <EmptyState title="Sin coincidencias" description="Prueba otra palabra o verifica que el documento tenga OCR activo." /> : <div className="table-wrap"><table><thead><tr><th>Documento</th><th>Página</th><th>Origen</th><th>Coincidencia</th><th>IIIF</th></tr></thead><tbody>{results.map((result) => <tr key={`${result.document_id}-${result.page_number}`}><td><code>{result.document_id.slice(0, 12)}…</code></td><td><strong>{result.page_number}</strong></td><td><Badge tone={result.source === "ocr" ? "info" : "success"}>{result.source}</Badge></td><td><span className="ocr-snippet">{result.snippet}</span><small className="table-secondary">{result.matches} coincidencia(s)</small></td><td>{result.image_id ? <a className="button button-secondary compact-button" href={`/iiif/3/${encodeURIComponent(result.image_id)}/full/max/0/default.jpg`} target="_blank" rel="noreferrer">Abrir imagen IIIF</a> : <span className="table-secondary">Imagen no disponible</span>}</td></tr>)}</tbody></table></div>}</Card>
  </>;
}
