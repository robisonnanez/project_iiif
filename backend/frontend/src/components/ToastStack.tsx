import { useEffect } from "react";
import type { NoticeTone } from "../types";

export interface ToastNotice {
  id: number;
  message: string;
  tone: NoticeTone;
}

export function ToastStack({ notices, dismiss }: { notices: ToastNotice[]; dismiss: (id: number) => void }) {
  return <div className="toast-stack" aria-live="polite" aria-atomic="false">
    {notices.map((notice) => <Toast key={notice.id} notice={notice} dismiss={dismiss} />)}
  </div>;
}

function Toast({ notice, dismiss }: { notice: ToastNotice; dismiss: (id: number) => void }) {
  useEffect(() => {
    const timer = window.setTimeout(() => dismiss(notice.id), 6000);
    return () => window.clearTimeout(timer);
  }, [dismiss, notice.id]);
  return <div className={`toast toast-${notice.tone}`} role={notice.tone === "danger" ? "alert" : "status"}>
    <span>{notice.message}</span>
    <button type="button" onClick={() => dismiss(notice.id)} aria-label="Cerrar alerta">×</button>
  </div>;
}
