import { useEffect, useId, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";

// Button aplica las variantes visuales y conserva los atributos nativos del botón.
export function Button({ variant = "primary", className = "", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "ghost" | "danger" }) {
  return <button className={`button button-${variant} ${className}`} {...props} />;
}

// Card agrupa contenido relacionado dentro de una superficie consistente.
export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <section className={`card ${className}`}>{children}</section>;
}

// Badge comunica estados breves mediante tono visual y texto.
export function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: "neutral" | "success" | "warning" | "danger" | "info" }) {
  return <span className={`badge badge-${tone}`}>{children}</span>;
}

// Spinner representa actividad en curso con una etiqueta accesible.
export function Spinner({ label = "Cargando" }: { label?: string }) {
  return <span className="spinner" role="status"><span aria-hidden="true" />{label}</span>;
}

// Alert presenta mensajes operativos con semántica accesible.
export function Alert({ children, tone = "info" }: { children: ReactNode; tone?: "info" | "danger" | "success" }) {
  return <div className={`alert alert-${tone}`} role={tone === "danger" ? "alert" : "status"}>{children}</div>;
}

// FormField enlaza etiqueta, ayuda y error con un control mediante un id estable.
export function FormField({ label, help, error, children }: { label: string; help?: string; error?: string; children: (id: string) => ReactNode }) {
  const id = useId();
  const descriptionId = help || error ? `${id}-description` : undefined;
  return <div className="form-field"><label htmlFor={id}>{label}</label>{children(id)}{(error || help) && <small id={descriptionId} className={error ? "field-error" : "field-help"}>{error || help}</small>}</div>;
}

// Input aplica el estilo compartido sin alterar el contrato nativo del elemento.
export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input className="input" {...props} />;
}

// Select aplica el estilo compartido a listas de selección nativas.
export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className="input" {...props} />;
}

// Checkbox asocia un control booleano con su etiqueta visible.
export function Checkbox({ label, ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  const id = useId();
  return <label className="checkbox" htmlFor={id}><input id={id} type="checkbox" {...props} /><span>{label}</span></label>;
}

// PageHeader unifica título, descripción y acciones principales de cada vista.
export function PageHeader({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description: string; actions?: ReactNode }) {
  return <header className="page-header"><div>{eyebrow && <span className="eyebrow">{eyebrow}</span>}<h1>{title}</h1><p>{description}</p></div>{actions && <div className="page-actions">{actions}</div>}</header>;
}

// EmptyState explica de forma explícita la ausencia de resultados.
export function EmptyState({ title, description }: { title: string; description: string }) {
  return <div className="empty-state"><strong>{title}</strong><p>{description}</p></div>;
}

// Modal presenta una tarea focal y permite cerrarla de forma accesible.
export function Modal({ title, description, children, onClose, className = "" }: { title: string; description?: string; children: ReactNode; onClose: () => void; className?: string }) {
  const dialog = useRef<HTMLDivElement>(null);
  const restoreFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    restoreFocus.current = document.activeElement as HTMLElement;
    dialog.current?.querySelector<HTMLElement>("button, input, select, textarea")?.focus();
    // escape encapsula esta interacción y mantiene coherente el estado de la vista.
    const escape = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("keydown", escape);
      restoreFocus.current?.focus();
    };
  }, [onClose]);
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div ref={dialog} className={`modal ${className}`} role="dialog" aria-modal="true" aria-labelledby="modal-title"><div className="modal-heading"><div><h2 id="modal-title">{title}</h2>{description && <p>{description}</p>}</div><Button variant="ghost" onClick={onClose} aria-label="Cerrar modal">×</Button></div>{children}</div></div>;
}
