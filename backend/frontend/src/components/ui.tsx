import { useEffect, useId, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";

export function Button({ variant = "primary", className = "", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "ghost" | "danger" }) {
  return <button className={`button button-${variant} ${className}`} {...props} />;
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <section className={`card ${className}`}>{children}</section>;
}

export function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: "neutral" | "success" | "warning" | "danger" | "info" }) {
  return <span className={`badge badge-${tone}`}>{children}</span>;
}

export function Spinner({ label = "Cargando" }: { label?: string }) {
  return <span className="spinner" role="status"><span aria-hidden="true" />{label}</span>;
}

export function Alert({ children, tone = "info" }: { children: ReactNode; tone?: "info" | "danger" | "success" }) {
  return <div className={`alert alert-${tone}`} role={tone === "danger" ? "alert" : "status"}>{children}</div>;
}

export function FormField({ label, help, error, children }: { label: string; help?: string; error?: string; children: (id: string) => ReactNode }) {
  const id = useId();
  const descriptionId = help || error ? `${id}-description` : undefined;
  return <div className="form-field"><label htmlFor={id}>{label}</label>{children(id)}{(error || help) && <small id={descriptionId} className={error ? "field-error" : "field-help"}>{error || help}</small>}</div>;
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input className="input" {...props} />;
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className="input" {...props} />;
}

export function Checkbox({ label, ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  const id = useId();
  return <label className="checkbox" htmlFor={id}><input id={id} type="checkbox" {...props} /><span>{label}</span></label>;
}

export function PageHeader({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description: string; actions?: ReactNode }) {
  return <header className="page-header"><div>{eyebrow && <span className="eyebrow">{eyebrow}</span>}<h1>{title}</h1><p>{description}</p></div>{actions && <div className="page-actions">{actions}</div>}</header>;
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return <div className="empty-state"><strong>{title}</strong><p>{description}</p></div>;
}

export function Modal({ title, description, children, onClose }: { title: string; description?: string; children: ReactNode; onClose: () => void }) {
  const dialog = useRef<HTMLDivElement>(null);
  const restoreFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    restoreFocus.current = document.activeElement as HTMLElement;
    dialog.current?.querySelector<HTMLElement>("button, input, select, textarea")?.focus();
    const escape = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("keydown", escape);
      restoreFocus.current?.focus();
    };
  }, [onClose]);
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div ref={dialog} className="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div className="modal-heading"><div><h2 id="modal-title">{title}</h2>{description && <p>{description}</p>}</div><Button variant="ghost" onClick={onClose} aria-label="Cerrar modal">×</Button></div>{children}</div></div>;
}
