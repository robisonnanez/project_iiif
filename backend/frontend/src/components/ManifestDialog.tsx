import { useState } from "react";
import type { DocumentRecord } from "../types";
import { normalizePageSelection } from "../lib/validation";
import { Alert, Button, FormField, Input, Modal } from "./ui";

export function ManifestDialog({ document, onClose, onOpen = (url) => window.open(url, "_blank", "noopener,noreferrer") }: { document: DocumentRecord; onClose: () => void; onOpen?: (url: string) => void }) {
  const [mode, setMode] = useState<"all" | "selected">("all");
  const [pages, setPages] = useState("");
  const [error, setError] = useState("");

  const submit = () => {
    try {
      const query = mode === "selected" ? `?pages=${encodeURIComponent(normalizePageSelection(pages, document.totalPages))}` : "";
      onOpen(`/api/v1/iiif/${encodeURIComponent(document.id)}/manifest${query}`);
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Selección inválida.");
    }
  };

  return <Modal title="Generar manifest IIIF" description={document.name} onClose={onClose}>
    <div className="segmented" role="radiogroup" aria-label="Páginas del manifest">
      <label><input type="radio" name="manifest-mode" checked={mode === "all"} onChange={() => setMode("all")} />Todas las páginas</label>
      <label><input type="radio" name="manifest-mode" checked={mode === "selected"} onChange={() => setMode("selected")} />Páginas seleccionadas</label>
    </div>
    {mode === "selected" && <FormField label="Páginas" help={`Formato: 1-5,8,10-12. Documento de ${document.totalPages} páginas.`} error={error}>
      {(id) => <Input id={id} value={pages} onChange={(event) => { setPages(event.target.value); setError(""); }} placeholder="1-5,8,10-12" aria-invalid={Boolean(error)} />}
    </FormField>}
    {error && mode === "all" && <Alert tone="danger">{error}</Alert>}
    <div className="modal-actions"><Button variant="secondary" onClick={onClose}>Cancelar</Button><Button onClick={submit}>Abrir manifest</Button></div>
  </Modal>;
}
