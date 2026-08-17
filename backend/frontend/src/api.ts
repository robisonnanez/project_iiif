import type { AppConfig, DocumentImagesResponse, DocumentRecord, MigrationDirectory, MigrationPayload, MigrationStatus, OCRJob, OCRSearchResponse, UploadScope, UploadSettings } from "./types";

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { credentials: "same-origin", ...init });
  const contentType = response.headers.get("content-type") ?? "";
  const body = contentType.includes("application/json") ? await response.json() : await response.text();
  if (!response.ok) {
    const message = typeof body === "object" && body && "error" in body ? String(body.error) : `HTTP ${response.status}`;
    throw new Error(message);
  }
  return body as T;
}

export const api = {
  config: () => request<AppConfig>("/api/v1/admin/config"),
  saveConfig: (config: AppConfig) => request<{ message: string }>("/api/v1/admin/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  }),
  documents: () => request<DocumentRecord[]>("/api/v1/documents"),
  upload: (file: File, settings: UploadSettings, scope: UploadScope) => {
    const form = new FormData();
    form.append("pdf", file);
    form.append("max_width", String(settings.width));
    form.append("max_height", String(settings.height));
    form.append("dpi", String(settings.dpi));
    form.append("format", settings.format);
    form.append("quality", String(settings.quality));
    if (scope.project) form.append("project", scope.project);
    if (scope.tenant) form.append("tenant", scope.tenant);
    return request<DocumentRecord>("/api/v1/documents/upload", { method: "POST", body: form });
  },
  documentImages: (documentId: string) => request<DocumentImagesResponse>(`/api/v1/admin/documents/${encodeURIComponent(documentId)}/images`),
  browseMigrationPath: (path: string) => request<{ path: string; dirs: MigrationDirectory[] }>(`/api/v1/admin/migrations/sources/local/browse${path ? `?path=${encodeURIComponent(path)}` : ""}`),
  startMigration: (payload: MigrationPayload) => request<{ message: string; status: MigrationStatus }>("/api/v1/admin/migrations/local-to-db/start", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  }),
  migrationStatus: () => request<MigrationStatus>("/api/v1/admin/migrations/local-to-db/status"),
  startOCR: (documentId: string, payload: { mode: string; language_mode: string; languages: string[]; force: boolean }) => request<OCRJob>(`/api/v1/admin/documents/${encodeURIComponent(documentId)}/ocr/jobs`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) }),
  ocrJob: (jobId: string) => request<OCRJob>(`/api/v1/admin/ocr/jobs/${encodeURIComponent(jobId)}`),
  cancelOCR: (jobId: string) => request<OCRJob>(`/api/v1/admin/ocr/jobs/${encodeURIComponent(jobId)}/cancel`, { method: "POST" }),
  searchOCR: (query: string, documentId?: string, project?: string, tenant?: string) => {
    const params = new URLSearchParams({ q: query, limit: "100" });
    if (documentId) params.set("document_id", documentId); if (project) params.set("project", project); if (tenant) params.set("tenant", tenant);
    return request<OCRSearchResponse>(`/api/v1/admin/ocr/search?${params}`);
  },
  logout: () => request<void>("/auth/logout", { method: "POST" }),
};
