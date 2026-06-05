const MASKED_SECRET = "********";

const state = {
  allDocuments: [],
  filteredDocuments: [],
  config: null,
  projects: { enabled: false, default_project: "default", items: [] },
  images: { items: [], page: 1, pageSize: 10 },
  migration: { timer: null, running: false },
  polling: new Map(),
  mongoURIEdited: false,
};

const viewMeta = {
  summary: ["Inicio", "Estado general del servidor IIIF."],
  upload: ["Subir PDF", "Carga un documento PDF y lanza la conversion."],
  documents: ["Documentos", "Consulta documentos, estados y manifiestos."],
  images: ["Imagenes", "Galeria de paginas servidas por IIIF."],
  config: ["Configuracion", "Edita valores permitidos del config.yaml sin exponer secretos."],
  migration: ["Migracion", "Migra datos locales hacia la base de datos activa de forma controlada."],
};

const viewRoutes = {
  summary: "/dashboard/inicio",
  upload: "/dashboard/subir-pdf",
  documents: "/dashboard/documentos",
  images: "/dashboard/imagenes",
  config: "/dashboard/configuracion",
  migration: "/dashboard/migracion",
};

const routeViews = {
  "/dashboard": "summary",
  "/dashboard/": "summary",
  "/dashboard/inicio": "summary",
  "/dashboard/subir-pdf": "upload",
  "/dashboard/documentos": "documents",
  "/dashboard/imagenes": "images",
  "/dashboard/configuracion": "config",
  "/dashboard/migracion": "migration",
};

const DB_PORT_DEFAULTS = {
  mysql: "3306",
  postgres: "5432",
  mongodb: "27017",
};

const API_V1_BASE = "/api/v1";
const DOCUMENTS_API_BASE = `${API_V1_BASE}/documents`;
const ADMIN_API_BASE = `${API_V1_BASE}/admin`;

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => Array.from(document.querySelectorAll(selector));

document.addEventListener("DOMContentLoaded", () => {
  $$(".nav-item").forEach((button) => {
    button.addEventListener("click", () => showView(button.dataset.view, true));
  });

  $("#refresh-button").addEventListener("click", refreshCurrentView);
  $("#upload-form").addEventListener("submit", uploadPDF);
  $("#logout-button").addEventListener("click", logout);
  $("#load-images-button").addEventListener("click", loadSelectedImages);
  $("#filter-documents-button").addEventListener("click", loadDocuments);
  $("#upload-project").addEventListener("change", () => updateTenantSelect("upload"));
  $("#documents-project").addEventListener("change", () => {
    updateTenantSelect("documents");
    loadDocuments();
  });
  $("#documents-tenant").addEventListener("change", loadDocuments);
  $("#images-project").addEventListener("change", () => {
    updateTenantSelect("images");
    resetImages();
    renderImageDocumentOptions();
  });
  $("#images-tenant").addEventListener("change", () => {
    resetImages();
    renderImageDocumentOptions();
  });
  $("#images-document").addEventListener("change", resetImages);
  $("#start-migration-button").addEventListener("click", startMigration);
  $("#refresh-migration-button").addEventListener("click", loadMigrationStatus);
  $("#open-migration-progress-button").addEventListener("click", async () => {
    openMigrationProgressModal();
    await loadMigrationStatus();
  });
  $("#migration-source-type").addEventListener("change", updateMigrationSourceMode);
  $("#browse-migration-local-button").addEventListener("click", browseMigrationLocalDirs);
  $("#migration-cancel-button").addEventListener("click", closeMigrationModal);
  $("#migration-confirm-button").addEventListener("click", submitMigrationFromModal);
  $("#migration-project").addEventListener("change", () => updateTenantSelect("migration"));
  $("#migration-progress-close").addEventListener("click", closeMigrationProgressModal);
  $("#restart-service-later").addEventListener("click", closeRestartServiceModal);
  $("#restart-service-now").addEventListener("click", restartServiceNow);
  $("#api-docs-button")?.addEventListener("click", openAPIDocs);
  $("#config-view")?.addEventListener("click", handleConfigActions);
  window.addEventListener("popstate", () => showView(viewFromLocation(), false));

  loadProjects();
  loadConfig();
  showView(viewFromLocation(), false);
  updateMigrationSourceMode();
});

function showView(name, pushRoute = false) {
  name = viewMeta[name] ? name : "summary";
  $$(".nav-item").forEach((button) => button.classList.toggle("active", button.dataset.view === name));
  $$(".view").forEach((view) => view.classList.toggle("active", view.id === `${name}-view`));

  if (pushRoute) {
    const nextPath = viewRoutes[name] || "/dashboard";
    if (window.location.pathname !== nextPath) {
      window.history.pushState({ view: name }, "", nextPath);
    }
  }

  const meta = viewMeta[name] || viewMeta.summary;
  $("#view-title").textContent = meta[0];
  $("#view-subtitle").textContent = meta[1];

  if (name === "summary") loadAllDocuments();
  if (name === "documents") loadDocuments(false);
  if (name === "images") {
    loadAllDocuments().then(() => renderImageDocumentOptions());
  }
  if (name === "config") loadConfig();
  if (name === "migration") loadMigrationStatus();
}

function refreshCurrentView() {
  const active = $(".nav-item.active")?.dataset.view || "summary";
  if (active === "config") {
    loadConfig();
  } else if (active === "migration") {
    loadMigrationStatus();
  } else if (active === "images") {
    loadSelectedImages();
  } else if (active === "summary") {
    loadConfig();
    loadAllDocuments();
  } else {
    loadDocuments(active === "documents");
  }
}

function openAPIDocs() {
  window.location.href = "/swagger/index.html";
}

function handleConfigActions(event) {
  const runButton = event.target.closest("#run-db-migrations");
  if (runButton) {
    event.preventDefault();
    runDBMigrations();
  }
}

async function startMigration() {
  // Abre modal para confirmar proyecto/tenant antes de iniciar la migracion.
  const payload = collectMigrationPayload(false);
  if (!payload) {
    return;
  }
  openMigrationModal(payload);
}

async function submitMigrationFromModal() {
  const payload = collectMigrationPayload(true);
  if (!payload) {
    return;
  }
  closeMigrationModal();
  try {
    const response = await fetch(`${ADMIN_API_BASE}/migrations/local-to-db/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    showToast(data.message || "Migracion iniciada.");
    openMigrationProgressModal();
    await loadMigrationStatus();
  } catch (error) {
    showToast(`No se pudo iniciar migracion: ${error.message}`, "error");
  }
}

async function loadMigrationStatus() {
  const badge = $("#migration-status-badge");
  const summary = $("#migration-summary");
  const logs = $("#migration-logs");
  if (!badge || !summary || !logs) return;

  try {
    const response = await fetch(`${ADMIN_API_BASE}/migrations/local-to-db/status`);
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);

    const running = Boolean(data.running);
    badge.className = `status ${running ? "" : data.exit_code === 0 ? "completed" : data.exit_code > 0 ? "error" : ""}`;
    badge.textContent = running ? "en ejecucion" : data.exit_code === 0 ? "completada" : data.exit_code > 0 ? "con errores" : "sin ejecutar";

    const started = data.started_at ? new Date(data.started_at).toLocaleString() : "-";
    const finished = data.finished_at ? new Date(data.finished_at).toLocaleString() : "-";
    summary.textContent = [
      `running: ${running}`,
      `exit_code: ${data.exit_code}`,
      `started_at: ${started}`,
      `finished_at: ${finished}`,
      `message: ${data.message || "-"}`
    ].join("\n");

    const logLines = Array.isArray(data.logs) && data.logs.length ? data.logs : ["Sin logs."];
    logs.textContent = logLines.join("\n");
    renderMigrationProgress(data);

    toggleMigrationPolling(running);
    $("#start-migration-button").disabled = running;
  } catch (error) {
    summary.textContent = `No se pudo consultar estado: ${error.message}`;
    toggleMigrationPolling(false);
  }
}

function openMigrationProgressModal() {
  const modal = $("#migration-progress-modal");
  if (modal) modal.hidden = false;
}

function closeMigrationProgressModal() {
  const modal = $("#migration-progress-modal");
  if (modal) modal.hidden = true;
}

function renderMigrationProgress(data) {
  const fill = $("#migration-progress-fill");
  const label = $("#migration-progress-label");
  const table = $("#migration-progress-table");
  if (!fill || !label || !table) return;
  const percent = Number(data.progress_percent || 0);
  fill.style.width = `${Math.max(0, Math.min(100, percent))}%`;
  label.textContent = `${percent}% completado${data.current_document ? ` - ${data.current_document}` : ""}`;

  const items = Array.isArray(data.items) ? [...data.items] : [];
  items.sort((a, b) => migrationStatusRank(a.status) - migrationStatusRank(b.status));
  if (!items.length) {
    table.innerHTML = `<tr><td colspan="4">Sin detalle por documento aun.</td></tr>`;
    return;
  }
  table.innerHTML = items.map((item) => `
    <tr>
      <td>${escapeHTML(item.pdf_name || item.document_id || "-")}</td>
      <td>${Number(item.images_done || 0)} / ${Number(item.images_total || 0)}</td>
      <td>${migrationStatusBadge(item.status)}</td>
      <td>${escapeHTML(item.message || "-")}</td>
    </tr>
  `).join("");
}

function migrationStatusBadge(status) {
  const value = String(status || "pending").toLowerCase();
  if (value === "ok") return '<span class="status completed">OK</span>';
  if (value === "error") return '<span class="status error">error</span>';
  return '<span class="status">ejecutando</span>';
}

function migrationStatusRank(status) {
  const value = String(status || "").toLowerCase();
  if (value === "running") return 0;
  if (value === "error") return 1;
  if (value === "ok") return 2;
  return 3;
}

function toggleMigrationPolling(shouldRun) {
  if (shouldRun && !state.migration.timer) {
    state.migration.timer = window.setInterval(() => {
      loadMigrationStatus();
    }, 3000);
  }
  if (!shouldRun && state.migration.timer) {
    window.clearInterval(state.migration.timer);
    state.migration.timer = null;
  }
}

function updateMigrationSourceMode() {
  const sourceType = $("#migration-source-type")?.value || "local";
  const localBox = $("#migration-local-source");
  const sshBox = $("#migration-ssh-source");
  if (localBox) localBox.hidden = sourceType !== "local";
  if (sshBox) sshBox.hidden = sourceType !== "ssh";
}

function collectMigrationPayload(includeScope) {
  const sourceType = $("#migration-source-type")?.value || "local";
  const scope = includeScope ? collectMigrationScope() : null;
  if (sourceType === "local") {
    const path = ($("#migration-local-path")?.value || "").trim();
    if (!path) {
      showToast("La ruta local es obligatoria.", "error");
      return null;
    }
    const payload = {
      source: {
        type: "local",
        local: { path },
      },
    };
    if (scope) payload.scope = scope;
    return payload;
  }

  const host = ($("#migration-ssh-host")?.value || "").trim();
  const user = ($("#migration-ssh-user")?.value || "").trim();
  const path = ($("#migration-ssh-path")?.value || "").trim();
  const privateKey = ($("#migration-ssh-key")?.value || "").trim();
  const port = Number.parseInt($("#migration-ssh-port")?.value || "22", 10) || 22;
  if (!host || !user || !path || !privateKey) {
    showToast("Para SSH: host, usuario, ruta y llave privada son obligatorios.", "error");
    return null;
  }
  const payload = {
    source: {
      type: "ssh",
      ssh: {
        host,
        port,
        user,
        path,
        private_key: privateKey,
      },
    },
  };
  if (scope) payload.scope = scope;
  return payload;
}

function collectMigrationScope() {
  const projectKey = ($("#migration-project")?.value || "").trim();
  const tenantField = $("#migration-tenant-field");
  const tenantKey = tenantField && !tenantField.hidden ? ($("#migration-tenant")?.value || "").trim() : "";
  if (!projectKey) {
    showToast("Selecciona un proyecto para la migracion.", "error");
    return null;
  }
  const project = projectItems().find((item) => item.key === projectKey);
  if (project?.multitenant && !tenantKey) {
    showToast("Selecciona un tenant para el proyecto multitenant.", "error");
    return null;
  }
  return { project_key: projectKey, tenant_key: tenantKey };
}

function openMigrationModal(payload) {
  const modal = $("#migration-scope-modal");
  if (!modal) return;
  const projectSelect = $("#migration-project");
  const defaultProject = state.projects.default_project || "default";
  projectSelect.innerHTML = projectItems().map((project) => `
    <option value="${escapeHTML(project.key)}">${escapeHTML(project.name || project.key)}</option>
  `).join("");
  projectSelect.value = defaultProject;
  updateTenantSelect("migration");
  const sourcePath = payload?.source?.type === "ssh"
    ? `${payload.source.ssh.user}@${payload.source.ssh.host}:${payload.source.ssh.path}`
    : payload?.source?.local?.path || "";
  $("#migration-source-preview").value = sourcePath;
  modal.hidden = false;
}

function closeMigrationModal() {
  const modal = $("#migration-scope-modal");
  if (modal) modal.hidden = true;
}

async function browseMigrationLocalDirs() {
  const input = $("#migration-local-path");
  const table = $("#migration-local-browser");
  if (!input || !table) return;
  const path = (input.value || "").trim();
  const query = path ? `?path=${encodeURIComponent(path)}` : "";
  try {
    const response = await fetch(`${ADMIN_API_BASE}/migrations/sources/local/browse${query}`);
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    const rows = Array.isArray(data.dirs) ? data.dirs : [];
    if (!rows.length) {
      table.innerHTML = `<tr><td colspan="3">Sin subdirectorios permitidos.</td></tr>`;
      return;
    }
    table.innerHTML = rows.map((row) => `
      <tr>
        <td>${escapeHTML(row.name || "")}</td>
        <td><code>${escapeHTML(row.path || "")}</code></td>
        <td><button class="secondary" type="button" data-pick-path="${escapeHTML(row.path || "")}">Seleccionar</button></td>
      </tr>
    `).join("");
    $$("[data-pick-path]").forEach((button) => {
      button.addEventListener("click", () => {
        const selected = button.getAttribute("data-pick-path") || "";
        input.value = selected;
      });
    });
  } catch (error) {
    table.innerHTML = `<tr><td colspan="3">No se pudo explorar: ${escapeHTML(error.message)}</td></tr>`;
  }
}

async function logout() {
  // Cierra la sesion del dashboard y vuelve al Welcome publico.
  try {
    await fetch("/auth/logout", { method: "POST" });
  } finally {
    window.location.href = "/";
  }
}

function viewFromLocation() {
  return routeViews[window.location.pathname] || "summary";
}

async function loadAllDocuments() {
  try {
    const response = await fetch(DOCUMENTS_API_BASE);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.allDocuments = await response.json();
    renderSummary();
    renderImageDocumentOptions();
  } catch (error) {
    $("#recent-documents").textContent = `No se pudieron cargar documentos: ${error.message}`;
  }
}

async function loadDocuments(applyFilters = true) {
  try {
    const params = applyFilters ? selectedScopeParams("documents") : "";
    const response = await fetch(`${DOCUMENTS_API_BASE}${params ? `?${params}` : ""}`);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.filteredDocuments = await response.json();
    renderDocuments();
  } catch (error) {
    $("#documents-table").innerHTML = `<tr><td colspan="8">No se pudieron cargar documentos: ${escapeHTML(error.message)}</td></tr>`;
  }
}

async function loadConfig() {
  try {
    const response = await fetch(`${ADMIN_API_BASE}/config`);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.config = await response.json();
    updateMigrationCopy();
    renderSummary();
    renderConfigForm();
  } catch (error) {
    $("#config-form").innerHTML = `<div class="config-card"><pre>No se pudo cargar configuracion: ${escapeHTML(error.message)}</pre></div>`;
  }
}

function updateMigrationCopy() {
  // Ajusta textos de migracion segun el motor de base de datos activo.
  const backend = String(state.config?.storage?.backend || state.config?.database?.DB_CONNECTION || "base de datos").toLowerCase();
  const dbLabel = backend === "postgres" ? "Postgres" : backend === "mysql" ? "MySQL" : backend === "mongodb" || backend === "mongo" ? "MongoDB" : "base de datos";
  const title = $("#migration-title");
  const subtitle = $("#migration-subtitle");
  if (title) title.textContent = `Migracion local a ${dbLabel} BLOB`;
  if (subtitle) subtitle.textContent = `Ejecuta la migracion one-shot de metadatos y binarios desde almacenamiento local hacia ${dbLabel}.`;
}

async function loadProjects() {
  try {
    const response = await fetch(`${ADMIN_API_BASE}/projects`);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.projects = await response.json();
  } catch (error) {
    state.projects = state.config?.projects || { enabled: false, default_project: "default", items: [] };
  }
  renderProjectControls();
  await loadAllDocuments();
  await loadDocuments(false);
}

async function uploadPDF(event) {
  event.preventDefault();
  const fileInput = $("#pdf-file");
  const result = $("#upload-result");
  const file = fileInput.files[0];

  if (!file) return;

  const formData = new FormData();
  formData.append("pdf", file);
  appendScopeToFormData(formData, "upload");

  result.hidden = false;
  result.textContent = "Subiendo PDF...";

  try {
    const response = await fetch(`${DOCUMENTS_API_BASE}/upload`, {
      method: "POST",
      body: formData,
    });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    result.textContent = JSON.stringify(data, null, 2);
    fileInput.value = "";
    if (data.id) {
      showToast("PDF recibido. Conversion en proceso...");
      watchDocument(data.id);
    }
    await loadAllDocuments();
    await loadDocuments(false);
  } catch (error) {
    result.textContent = `Error: ${error.message}`;
  }
}

function watchDocument(documentID) {
  if (state.polling.has(documentID)) return;

  let attempts = 0;
  const timer = window.setInterval(async () => {
    attempts += 1;
    try {
      const response = await fetch(`${DOCUMENTS_API_BASE}/${encodeURIComponent(documentID)}`);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const doc = await response.json();

      if (doc.status === "completed") {
        window.clearInterval(timer);
        state.polling.delete(documentID);
        showToast("PDF convertido a imagenes correctamente.");
        await loadAllDocuments();
        await loadDocuments(false);
      } else if (doc.status === "error") {
        window.clearInterval(timer);
        state.polling.delete(documentID);
        showToast("La conversion del PDF fallo.", "error");
        await loadAllDocuments();
        await loadDocuments(false);
      } else if (attempts >= 120) {
        window.clearInterval(timer);
        state.polling.delete(documentID);
        showToast("La conversion sigue en proceso. Actualiza documentos mas tarde.", "error");
      }
    } catch (error) {
      if (attempts >= 5) {
        window.clearInterval(timer);
        state.polling.delete(documentID);
        showToast(`No se pudo consultar el estado: ${error.message}`, "error");
      }
    }
  }, 3000);

  state.polling.set(documentID, timer);
}

function renderSummary() {
  const docs = Array.isArray(state.allDocuments) ? state.allDocuments : [];
  $("#documents-count").textContent = docs.length;
  $("#completed-count").textContent = docs.filter((doc) => doc.status === "completed").length;
  $("#storage-backend").textContent = state.config?.storage?.backend || "-";

  if (!docs.length) {
    $("#recent-documents").className = "empty";
    $("#recent-documents").textContent = "Sin documentos cargados.";
    return;
  }

  $("#recent-documents").className = "table-wrap";
  $("#recent-documents").innerHTML = `
    <table>
      <thead>
        <tr><th>Nombre</th><th>Estado</th><th>Paginas</th></tr>
      </thead>
      <tbody>
        ${docs.slice(0, 5).map((doc) => `
          <tr>
            <td>${escapeHTML(doc.name || doc.id)}</td>
            <td>${statusBadge(doc.status)}</td>
            <td>${Number(doc.convertedPages || 0)} / ${Number(doc.totalPages || 0)}</td>
          </tr>
        `).join("")}
      </tbody>
    </table>
  `;
}

function renderDocuments() {
  const docs = Array.isArray(state.filteredDocuments) ? state.filteredDocuments : [];
  if (!docs.length) {
    $("#documents-table").innerHTML = `<tr><td colspan="8">Sin documentos cargados.</td></tr>`;
    return;
  }

  $("#documents-table").innerHTML = docs.map((doc) => `
    <tr>
      <td><code>${escapeHTML(doc.id || "")}</code></td>
      <td>${escapeHTML(doc.name || "")}</td>
      <td>${escapeHTML(doc.projectKey || "default")}</td>
      <td>${escapeHTML(doc.tenantKey || "-")}</td>
      <td>${migratedBadge(Boolean(doc.migratedFromLocal))}</td>
      <td>${statusBadge(doc.status)}</td>
      <td>${Number(doc.convertedPages || 0)} / ${Number(doc.totalPages || 0)}</td>
      <td>${doc.manifestUrl ? `<a href="${escapeHTML(doc.manifestUrl)}" target="_blank" rel="noreferrer">Ver</a>` : "-"}</td>
    </tr>
  `).join("");
}

function renderImageDocumentOptions() {
  const select = $("#images-document");
  if (!select) return;

  const scope = selectedScope("images");
  const completedDocs = (state.allDocuments || []).filter((doc) => {
    if (doc.status !== "completed") return false;
    if (scope.project && (doc.projectKey || "default") !== scope.project) return false;
    if (scope.tenant && (doc.tenantKey || "") !== scope.tenant) return false;
    return true;
  });
  if (!completedDocs.length) {
    select.innerHTML = `<option value="">Sin documentos completados</option>`;
    resetImages("No hay imagenes disponibles.");
    return;
  }

  const current = select.value;
  select.innerHTML = completedDocs.map((doc) => `
    <option value="${escapeHTML(doc.id)}">${escapeHTML(doc.name || doc.id)}</option>
  `).join("");
  if (completedDocs.some((doc) => doc.id === current)) {
    select.value = current;
  }
}

function resetImages(message = "Selecciona un documento completado.") {
  state.images.items = [];
  state.images.page = 1;
  const gallery = $("#images-gallery");
  if (!gallery) return;
  gallery.className = "image-gallery empty";
  gallery.textContent = message;
}

async function loadSelectedImages() {
  // Carga metadatos de paginas y usa IIIF como fuente real de cada miniatura.
  const documentID = $("#images-document")?.value;
  const gallery = $("#images-gallery");
  if (!documentID) {
    gallery.className = "image-gallery empty";
    gallery.textContent = "Selecciona un documento completado.";
    return;
  }

  gallery.className = "image-gallery empty";
  gallery.textContent = "Cargando imagenes...";

  try {
    const response = await fetch(`${ADMIN_API_BASE}/documents/${encodeURIComponent(documentID)}/images`);
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    state.images.items = data.images || [];
    state.images.page = 1;
    renderImages();
  } catch (error) {
    gallery.className = "image-gallery empty";
    gallery.textContent = `No se pudieron cargar imagenes: ${error.message}`;
  }
}

function renderImages() {
  const gallery = $("#images-gallery");
  const images = state.images.items || [];
  if (!images.length) {
    gallery.className = "image-gallery empty";
    gallery.textContent = "Este documento no tiene paginas convertidas.";
    return;
  }

  const pageSize = state.images.pageSize;
  const totalPages = Math.max(1, Math.ceil(images.length / pageSize));
  state.images.page = Math.min(Math.max(1, state.images.page), totalPages);
  const start = (state.images.page - 1) * pageSize;
  const visibleImages = images.slice(start, start + pageSize);

  gallery.className = "table-wrap images-table-wrap";
  gallery.innerHTML = `
    <table class="images-table">
      <thead>
        <tr>
          <th>Miniatura</th>
          <th>Pagina</th>
          <th>ID imagen</th>
          <th>Proyecto / Tenant</th>
          <th>Migrada</th>
          <th>Dimensiones</th>
          <th>URL IIIF</th>
          <th>Accion</th>
        </tr>
      </thead>
      <tbody>
        ${visibleImages.map((image) => `
          <tr>
            <td>
              <a class="thumb-link" href="${escapeHTML(image.iiif_url)}" target="_blank" rel="noreferrer">
                <img class="image-thumb" src="${escapeHTML(image.iiif_url)}" alt="Pagina ${Number(image.page_number || 0)}">
              </a>
            </td>
            <td>${Number(image.page_number || 0)}</td>
            <td><code>${escapeHTML(image.image_id || "")}</code></td>
            <td>${escapeHTML(image.project_key || "default")} ${image.tenant_key ? `/ ${escapeHTML(image.tenant_key)}` : ""}</td>
            <td>${migratedBadge(Boolean(image.migrated_from_local))}</td>
            <td>${Number(image.width || 0)} x ${Number(image.height || 0)} px</td>
            <td><a class="iiif-url" href="${escapeHTML(image.iiif_url)}" target="_blank" rel="noreferrer">${escapeHTML(image.iiif_url)}</a></td>
            <td><a class="table-action" href="${escapeHTML(image.iiif_url)}" target="_blank" rel="noreferrer">Abrir</a></td>
          </tr>
        `).join("")}
      </tbody>
    </table>
    ${renderImagesPagination(totalPages)}
  `;
  bindImagesPagination(totalPages);
}

function renderImagesPagination(totalPages) {
  const current = state.images.page;
  const buttons = [];
  buttons.push(paginationButton("Inicio", 1, current === 1));
  buttons.push(paginationButton("Previous", current - 1, current === 1));
  for (let page = 1; page <= totalPages; page += 1) {
    buttons.push(paginationButton(String(page), page, page === current, page === current));
  }
  buttons.push(paginationButton("Next", current + 1, current === totalPages));
  buttons.push(paginationButton("Ultima", totalPages, current === totalPages));
  return `<nav class="pagination" aria-label="Paginacion de imagenes">${buttons.join("")}</nav>`;
}

function paginationButton(label, page, disabled, active = false) {
  return `
    <button type="button" class="${active ? "active" : ""}" data-page="${Number(page)}" ${disabled ? "disabled" : ""}>
      ${escapeHTML(label)}
    </button>
  `;
}

function bindImagesPagination(totalPages) {
  $$(".pagination [data-page]").forEach((button) => {
    button.addEventListener("click", () => {
      const page = Number.parseInt(button.dataset.page, 10);
      if (!Number.isFinite(page)) return;
      state.images.page = Math.min(Math.max(1, page), totalPages);
      renderImages();
    });
  });
}

function hydrateMongoFieldsFromConfig() {
  const database = state.config?.database || {};
  const mongo = database.mongodb || {};
  const setValue = (name, nextValue) => {
    const field = formField(name);
    if (!field) return;
    field.value = nextValue ?? "";
  };

  setValue("database.DB_HOST", mongo.host || database.DB_HOST || "127.0.0.1");
  setValue("database.DB_PORT", mongo.port || database.DB_PORT || DB_PORT_DEFAULTS.mongodb);
  setValue("database.DB_DATABASE", mongo.database || database.DB_DATABASE || "");
  setValue("database.DB_USERNAME", mongo.user || database.DB_USERNAME || "");
  setValue("database.DB_PASSWORD", mongo.password || database.DB_PASSWORD || "");
}

function renderConfigForm() {
  const config = state.config;
  if (!config) return;
  state.mongoURIEdited = false;

  $("#config-form").innerHTML = `
    <div class="form-grid">
      ${fieldset("Servidor", [
        input("server.port", "Puerto", config.server?.port || ""),
        input("server.mode", "Modo", config.server?.mode || "")
      ])}
      ${fieldset("Storage", [
        select("storage.backend", "Backend", config.storage?.backend || "local", ["local", "mysql", "postgres", "mongodb"]),
        input("storage.data_path", "Data path", config.storage?.data_path || ""),
        input("storage.pdfs_path", "PDFs path", config.storage?.pdfs_path || ""),
        input("storage.images_path", "Images path", config.storage?.images_path || ""),
        input("storage.documents_path", "Documents path", config.storage?.documents_path || ""),
        input("storage.thumbnails_path", "Thumbnails path", config.storage?.thumbnails_path || ""),
        input("storage.manifests_path", "Manifests path", config.storage?.manifests_path || "")
      ])}
      ${renderDatabaseFieldset(config)}
      ${fieldset("Frontend", [
        checkbox("frontend.enabled", "Frontend activo", Boolean(config.frontend?.enabled)),
        checkbox("frontend.require_auth", "Requiere login", Boolean(config.frontend?.require_auth)),
        input("frontend.path", "Path", config.frontend?.path || ""),
        input("frontend.username", "Usuario", config.frontend?.username || ""),
        input("frontend.password", "Password", config.frontend?.password || "", "password")
      ])}
      ${fieldset("Binarios", [
        select("binary_storage.mode", "Modo binario", config.binary_storage?.mode || "local", ["local", "database"]),
        input("binary_storage.temp_path", "Temp path", config.binary_storage?.temp_path || "")
      ])}
      ${fieldset("IIIF", [
        input("iiif.base_url", "Base URL", config.iiif?.base_url || ""),
        input("iiif.api_version", "API version", config.iiif?.api_version || "3"),
        input("iiif.max_width", "Max width", config.iiif?.max_width || 2048, "number"),
        input("iiif.max_height", "Max height", config.iiif?.max_height || 2048, "number"),
        input("iiif.cache_ttl", "Cache TTL", config.iiif?.cache_ttl || 3600, "number"),
        checkbox("iiif.cache", "Cache activo", Boolean(config.iiif?.cache))
      ])}
      ${fieldset("Security", [
        checkbox("security.enable_auth", "Enable auth", Boolean(config.security?.enable_auth)),
        input("security.log_level", "Log level", config.security?.log_level || "info"),
        input("security.max_concurrent_uploads", "Max concurrent uploads", config.security?.max_concurrent_uploads || 5, "number"),
        textarea("security.cors_origins", "CORS origins (una URL por linea)", (config.security?.cors_origins || []).join("\n"))
      ])}
      ${fieldset("Proyectos", [
        checkbox("projects.enabled", "Proyectos activos", Boolean(config.projects?.enabled)),
        input("projects.default_project", "Proyecto por defecto", config.projects?.default_project || "default"),
        checkbox("projects.require_project", "Proyecto obligatorio", Boolean(config.projects?.require_project)),
        checkbox("projects.allow_dynamic_tenants", "Permitir tenants dinamicos", Boolean(config.projects?.allow_dynamic_tenants)),
        textarea("projects.items", "Proyectos JSON", JSON.stringify(config.projects?.items || [], null, 2))
      ])}
    </div>
    <div class="config-card">
      <legend>Migraciones DB</legend>
      <div class="toolbar-row">
        <button class="primary" type="button" id="run-db-migrations">Ejecutar migraciones</button>
        <span class="muted">Motor activo: ${escapeHTML(config.storage?.backend || "local")}</span>
      </div>
      <pre id="db-migration-status" class="result">Sin ejecuciones de migracion DB.</pre>
      <label class="field">
        <span>Ejemplo de nueva migracion (${escapeHTML((config.storage?.backend || "local").toLowerCase())})</span>
        <textarea rows="8" readonly>${escapeHTML(exampleMigrationSQL(config.storage?.backend || "local"))}</textarea>
      </label>
    </div>
    <div class="form-actions">
      <button class="primary" type="submit">Guardar configuracion</button>
      <span class="muted">Los secretos en ${MASKED_SECRET} se conservan sin cambios.</span>
    </div>
  `;

  $("#config-form").addEventListener("submit", saveConfig);
  bindDBConnectionAutofill();
  void loadDBMigrationStatus();
}

function bindDBConnectionAutofill() {
  // Sincroniza puerto por defecto, backend y visibilidad de campos al cambiar motor.
  const connection = $("#config-form select[name='database.DB_CONNECTION']");
  const port = $("#config-form input[name='database.DB_PORT']");
  const storageBackend = $("#config-form select[name='storage.backend']");
  if (!connection || !port) return;

  const syncDatabaseUI = () => {
    const engine = normalizedDBEngine(connection.value || "local");
    const sqlBlocks = $$("#sql-db-fields");
    const mongoBlocks = $$("#mongo-db-fields");
    sqlBlocks.forEach((block) => {
      block.hidden = engine === "mongodb";
    });
    mongoBlocks.forEach((block) => {
      block.hidden = engine !== "mongodb";
    });
    if (engine === "mongodb" && !state.mongoURIEdited) {
      hydrateMongoFieldsFromConfig();
    }
    updateMongoURIPreview();
  };

  let lastEngine = String(connection.value || "local").toLowerCase();
  connection.addEventListener("change", () => {
    const nextEngine = normalizedDBEngine(connection.value || "local");
    const nextDefaultPort = DB_PORT_DEFAULTS[nextEngine] || "";
    const previousDefaultPort = DB_PORT_DEFAULTS[normalizedDBEngine(lastEngine)] || "";

    // Cambia puerto automaticamente cuando coincide con el default anterior o esta vacio.
    if (nextDefaultPort && (!port.value || port.value === previousDefaultPort)) {
      port.value = nextDefaultPort;
    }

    // Mantiene storage.backend alineado con el motor de DB cuando aplica.
    if (storageBackend && (nextEngine === "mysql" || nextEngine === "postgres" || nextEngine === "mongodb" || nextEngine === "local")) {
      storageBackend.value = nextEngine;
    }
    lastEngine = nextEngine;
    syncDatabaseUI();
  });

  [
    "database.DB_HOST",
    "database.DB_PORT",
    "database.DB_DATABASE",
    "database.DB_USERNAME",
    "database.DB_PASSWORD",
  ].forEach((name) => {
    const field = formField(name);
    if (!field) return;
    field.addEventListener("input", updateMongoURIPreview);
    field.addEventListener("change", updateMongoURIPreview);
  });

  const mongoURIField = $("#mongo-uri-preview");
  if (mongoURIField) {
    mongoURIField.addEventListener("input", () => {
      state.mongoURIEdited = true;
    });
  }

  syncDatabaseUI();
}

async function saveConfig(event) {
  // Envia solo el formulario permitido y deja que el backend valide antes de escribir config.yaml.
  event.preventDefault();
  const payload = collectConfigPayload();
  if (!payload) return;
  try {
    const response = await fetch(`${ADMIN_API_BASE}/config`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    state.config = data.config;
    renderConfigForm();
    showToast(data.message || "Configuracion guardada.");
    if (data.requires_restart) {
      openRestartServiceModal();
    }
  } catch (error) {
    showToast(`No se pudo guardar: ${error.message}`, "error");
  }
}

function openRestartServiceModal() {
  const modal = $("#restart-service-modal");
  const status = $("#restart-service-status");
  const password = $("#restart-service-password");
  if (status) status.textContent = "";
  if (password) password.value = "";
  if (modal) modal.hidden = false;
}

function closeRestartServiceModal() {
  const modal = $("#restart-service-modal");
  const status = $("#restart-service-status");
  const password = $("#restart-service-password");
  if (password) password.value = "";
  if (status) status.textContent = "";
  if (modal) modal.hidden = true;
  showToast("Configuracion guardada. Reinicia el servicio luego para aplicar cambios.", "success");
}

async function restartServiceNow() {
  const status = $("#restart-service-status");
  const passwordInput = $("#restart-service-password");
  const password = (passwordInput?.value || "").trim();
  if (!password) {
    if (status) status.textContent = "Ingresa la contraseña para continuar.";
    return;
  }

  const restartButton = $("#restart-service-now");
  const laterButton = $("#restart-service-later");
  if (restartButton) restartButton.disabled = true;
  if (laterButton) laterButton.disabled = true;
  if (status) status.textContent = "Programando reinicio...";

  try {
    const response = await fetch(`${ADMIN_API_BASE}/service/restart`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    });
    let data = {};
    try {
      data = await response.json();
    } catch (jsonError) {
      data = {};
    }
    if (!response.ok || !data.ok) {
      throw new Error(data.error || data.details || `HTTP ${response.status}`);
    }
    if (status) status.textContent = "Reinicio programado. Esperando que el servicio vuelva...";
    showToast(data.message || "Reinicio programado.");
    if (passwordInput) passwordInput.value = "";
    window.setTimeout(() => {
      const modal = $("#restart-service-modal");
      if (modal) modal.hidden = true;
    }, 1200);
    void monitorServiceRecovery();
  } catch (error) {
    const message = String(error?.message || "");
    const networkError = /failed to fetch|networkerror|load failed/i.test(message);
    const readable = networkError
      ? "No hubo respuesta del servidor. Verifica si el servicio se esta reiniciando."
      : `No se pudo reiniciar: ${message}`;
    if (status) status.textContent = readable;
    showToast(networkError ? readable : `No se pudo reiniciar servicio: ${message}`, "error");
  } finally {
    if (restartButton) restartButton.disabled = false;
    if (laterButton) laterButton.disabled = false;
  }
}

async function monitorServiceRecovery() {
  // Espera la caida y vuelta del endpoint /health para confirmar reinicio exitoso.
  const timeoutMs = 60000;
  const intervalMs = 3000;
  const deadline = Date.now() + timeoutMs;
  let sawDown = false;

  while (Date.now() < deadline) {
    try {
      const response = await fetch("/health", { cache: "no-store" });
      if (response.ok) {
        if (sawDown) {
          showToast("Servicio activo nuevamente.");
          return;
        }
      } else {
        sawDown = true;
      }
    } catch (error) {
      sawDown = true;
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((resolve) => window.setTimeout(resolve, intervalMs));
  }

  showToast("El servicio tarda en volver. Revisa: journalctl -u project-iiif -n 100", "error");
}

function collectConfigPayload() {
  const form = $("#config-form");
  const value = (name) => form.elements[name]?.value || "";
  const checked = (name) => Boolean(form.elements[name]?.checked);
  const intValue = (name) => Number.parseInt(value(name), 10) || 0;

  const dbConnection = normalizedDBEngine(value("database.DB_CONNECTION"));
  let dbHost = value("database.DB_HOST");
  let dbPort = value("database.DB_PORT");
  let dbName = value("database.DB_DATABASE");
  let dbUser = value("database.DB_USERNAME");
  let dbPassword = value("database.DB_PASSWORD");
  let mongoAuthSource = value("database.mongodb.auth_source");
  let mongoTimeout = intValue("database.mongodb.server_selection_timeout_ms");
  let mongoDirectConnection = checked("database.mongodb.direct_connection");

  const mysqlCurrent = state.config?.database?.mysql || {};
  const postgresCurrent = state.config?.database?.postgres || {};
  const mongoCurrent = state.config?.database?.mongodb || {};
  const mysqlPayload = {
    host: mysqlCurrent.host || dbHost,
    port: mysqlCurrent.port || DB_PORT_DEFAULTS.mysql,
    user: mysqlCurrent.user || dbUser,
    password: mysqlCurrent.password || dbPassword,
    database: mysqlCurrent.database || dbName,
    charset: mysqlCurrent.charset || "utf8mb4",
    parse_time: mysqlCurrent.parse_time !== undefined ? Boolean(mysqlCurrent.parse_time) : true,
  };
  const postgresPayload = {
    host: postgresCurrent.host || dbHost,
    port: postgresCurrent.port || DB_PORT_DEFAULTS.postgres,
    user: postgresCurrent.user || dbUser,
    password: postgresCurrent.password || dbPassword,
    database: postgresCurrent.database || dbName,
    sslmode: postgresCurrent.sslmode || "disable",
    schema: postgresCurrent.schema || "public",
  };
  const mongoPayload = {
    host: mongoCurrent.host || dbHost,
    port: mongoCurrent.port || DB_PORT_DEFAULTS.mongodb,
    user: mongoCurrent.user || dbUser,
    password: mongoCurrent.password || dbPassword,
    database: mongoCurrent.database || dbName,
    auth_source: mongoCurrent.auth_source || "admin",
    direct_connection: mongoCurrent.direct_connection !== undefined ? Boolean(mongoCurrent.direct_connection) : true,
    server_selection_timeout_ms: Number.parseInt(mongoCurrent.server_selection_timeout_ms, 10) || mongoCurrent.server_selection_timeout_ms || 2000,
  };

  if (dbConnection === "mysql") {
    mysqlPayload.host = dbHost;
    mysqlPayload.port = dbPort || DB_PORT_DEFAULTS.mysql;
    mysqlPayload.user = dbUser;
    mysqlPayload.password = dbPassword;
    mysqlPayload.database = dbName;
  } else if (dbConnection === "postgres") {
    postgresPayload.host = dbHost;
    postgresPayload.port = dbPort || DB_PORT_DEFAULTS.postgres;
    postgresPayload.user = dbUser;
    postgresPayload.password = dbPassword;
    postgresPayload.database = dbName;
  } else if (dbConnection === "mongodb" || dbConnection === "mongo") {
    const mongoURI = ($("#mongo-uri-preview")?.value || "").trim();
    if (mongoURI) {
      if (/^mongodb\+srv:\/\//i.test(mongoURI)) {
        showToast("Este proyecto aun no soporta Mongo URI tipo mongodb+srv://. Usa mongodb:// con host y puerto.", "error");
        return null;
      }
      const parsedMongoURI = parseMongoURI(mongoURI);
      if (parsedMongoURI) {
        dbHost = parsedMongoURI.host || dbHost;
        dbPort = parsedMongoURI.port || dbPort || DB_PORT_DEFAULTS.mongodb;
        dbName = parsedMongoURI.database || dbName;
        dbUser = parsedMongoURI.username || dbUser;
        dbPassword = parsedMongoURI.password || dbPassword;
        mongoAuthSource = parsedMongoURI.authSource || mongoAuthSource || "admin";
        mongoDirectConnection = parsedMongoURI.directConnection ?? mongoDirectConnection;
        mongoTimeout = parsedMongoURI.serverSelectionTimeoutMS || mongoTimeout || 2000;
      } else {
        showToast("El Mongo URI no es valido. Usa el formato mongodb://...", "error");
        return null;
      }
    }
    mongoPayload.host = dbHost;
    mongoPayload.port = dbPort || DB_PORT_DEFAULTS.mongodb;
    mongoPayload.user = dbUser;
    mongoPayload.password = dbPassword;
    mongoPayload.database = dbName;
    mongoPayload.auth_source = mongoAuthSource || "admin";
    mongoPayload.direct_connection = mongoDirectConnection;
    mongoPayload.server_selection_timeout_ms = mongoTimeout || 2000;
  }

  return {
    server: { port: value("server.port"), mode: value("server.mode") },
    storage: {
      backend: value("storage.backend"),
      data_path: value("storage.data_path"),
      pdfs_path: value("storage.pdfs_path"),
      images_path: value("storage.images_path"),
      documents_path: value("storage.documents_path"),
      thumbnails_path: value("storage.thumbnails_path"),
      manifests_path: value("storage.manifests_path"),
    },
    database: {
      DB_CONNECTION: dbConnection,
      DB_HOST: dbHost,
      DB_PORT: dbPort,
      DB_DATABASE: dbName,
      DB_USERNAME: dbUser,
      DB_PASSWORD: dbPassword,
      mysql: mysqlPayload,
      postgres: postgresPayload,
      mongodb: mongoPayload,
    },
    frontend: {
      enabled: checked("frontend.enabled"),
      path: value("frontend.path"),
      require_auth: checked("frontend.require_auth"),
      username: value("frontend.username"),
      password: value("frontend.password"),
    },
    binary_storage: {
      mode: value("binary_storage.mode"),
      temp_path: value("binary_storage.temp_path"),
    },
    iiif: {
      base_url: value("iiif.base_url"),
      api_version: value("iiif.api_version"),
      max_width: intValue("iiif.max_width"),
      max_height: intValue("iiif.max_height"),
      cache: checked("iiif.cache"),
      cache_ttl: intValue("iiif.cache_ttl"),
    },
    projects: {
      enabled: checked("projects.enabled"),
      default_project: value("projects.default_project"),
      require_project: checked("projects.require_project"),
      allow_dynamic_tenants: checked("projects.allow_dynamic_tenants"),
      items: parseProjectsItems(value("projects.items")),
    },
    security: {
      enable_auth: checked("security.enable_auth"),
      log_level: value("security.log_level"),
      cors_origins: parseCorsOrigins(value("security.cors_origins")),
      max_concurrent_uploads: intValue("security.max_concurrent_uploads"),
    },
  };
}

async function loadDBMigrationStatus() {
  const box = $("#db-migration-status");
  if (!box) return;
  try {
    const response = await fetch(`${ADMIN_API_BASE}/db/migrations/status`);
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
    box.textContent = formatDBMigrationStatus(data.result || {}, Boolean(data.running));
  } catch (error) {
    box.textContent = `No se pudo cargar estado: ${error.message}`;
  }
}

async function runDBMigrations() {
  const box = $("#db-migration-status");
  if (box) box.textContent = "Ejecutando migraciones pendientes...";
  try {
    const response = await fetch(`${ADMIN_API_BASE}/db/migrations/run`, { method: "POST" });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || data?.result?.message || `HTTP ${response.status}`);
    }
    if (box) box.textContent = formatDBMigrationStatus(data, false);
    showToast(data.message || "Migraciones ejecutadas.");
  } catch (error) {
    if (box) box.textContent = `Error ejecutando migraciones: ${error.message}`;
    showToast(`No se pudo ejecutar migraciones: ${error.message}`, "error");
  }
}

function formatDBMigrationStatus(data, running = false) {
  return [
    `running: ${running}`,
    `engine: ${data.engine || "-"}`,
    `pending_before: ${Number(data.pending_before || 0)}`,
    `applied: ${Number(data.applied || 0)}`,
    `skipped: ${Number(data.skipped || 0)}`,
    `duration_ms: ${Number(data.duration_ms || 0)}`,
    `message: ${data.message || "-"}`,
    Array.isArray(data.applied_files) && data.applied_files.length ? `applied_files: ${data.applied_files.join(", ")}` : "",
    Array.isArray(data.errors) && data.errors.length ? `errors: ${data.errors.join(" | ")}` : "",
  ].filter(Boolean).join("\n");
}

function exampleMigrationSQL(engine) {
  if (String(engine).toLowerCase().startsWith("mongo")) {
    return `// 006_add_example_index.js\n// Ejecutar desde el runner interno de Mongo\nawait db.collection("documents").createIndex(\n  { status: 1, created_at: -1 },\n  { name: "documents_status_created_at" }\n);`;
  }
  if (String(engine).toLowerCase().startsWith("post")) {
    return "BEGIN;\n-- 005_add_example_table.sql\nCREATE TABLE IF NOT EXISTS example_table (\n  id UUID PRIMARY KEY,\n  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()\n);\nCOMMIT;";
  }
  return "START TRANSACTION;\n-- 005_add_example_table.sql\nCREATE TABLE IF NOT EXISTS example_table (\n  id VARCHAR(36) PRIMARY KEY,\n  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP\n);\nCOMMIT;";
}

function renderProjectControls() {
  ["upload", "documents", "images", "migration"].forEach((target) => {
    const select = $(`#${target}-project`);
    if (!select) return;
    const current = select.value;
    const projects = projectItems();
    const allOption = target === "upload" ? "" : `<option value="">Todos los proyectos</option>`;
    select.innerHTML = allOption + projects.map((project) => `
      <option value="${escapeHTML(project.key)}">${escapeHTML(project.name || project.key)}</option>
    `).join("");
    const defaultProject = state.projects.default_project || "default";
    if ((target === "documents" || target === "images") && current === "") {
      select.value = "";
    } else {
      select.value = projects.some((project) => project.key === current) ? current : defaultProject;
    }
    updateTenantSelect(target);
  });
}

function updateTenantSelect(target) {
  const projectKey = $(`#${target}-project`)?.value || "";
  const tenantField = $(`#${target}-tenant-field`);
  const tenantSelect = $(`#${target}-tenant`);
  const project = projectItems().find((item) => item.key === projectKey);
  if (!tenantField || !tenantSelect) return;
  if (!project?.multitenant) {
    tenantField.hidden = true;
    tenantSelect.innerHTML = "";
    return;
  }
  tenantField.hidden = false;
  tenantSelect.innerHTML = (project.tenants || []).map((tenant) => `
    <option value="${escapeHTML(tenant)}">${escapeHTML(tenant)}</option>
  `).join("");
}

function projectItems() {
  const items = state.projects?.items || [];
  if (items.length) return items;
  return [{ key: "default", name: "Proyecto por defecto", multitenant: false, tenants: [] }];
}

function selectedScope(target) {
  const project = $(`#${target}-project`)?.value || "";
  const tenantField = $(`#${target}-tenant-field`);
  const tenant = tenantField && !tenantField.hidden ? ($(`#${target}-tenant`)?.value || "") : "";
  return { project, tenant };
}

function selectedScopeParams(target) {
  const scope = selectedScope(target);
  const params = new URLSearchParams();
  if (scope.project) params.set("project", scope.project);
  if (scope.tenant) params.set("tenant", scope.tenant);
  return params.toString();
}

function appendScopeToFormData(formData, target) {
  const scope = selectedScope(target);
  if (scope.project) formData.append("project", scope.project);
  if (scope.tenant) formData.append("tenant", scope.tenant);
}

function parseProjectsItems(value) {
  try {
    const parsed = JSON.parse(value || "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch (error) {
    showToast("El JSON de proyectos no es valido.", "error");
    return state.config?.projects?.items || [];
  }
}

function parseCorsOrigins(value) {
  return String(value || "")
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

function normalizedDBEngine(value) {
  const engine = String(value || "local").toLowerCase();
  if (engine === "mongo") return "mongodb";
  if (engine === "postgresql") return "postgres";
  return engine;
}

function renderDatabaseFieldset(config) {
  const database = config.database || {};
  const mongo = database.mongodb || {};
  const engine = normalizedDBEngine(database.DB_CONNECTION || "local");
  const mongoURI = buildMongoURI({
    host: database.DB_HOST || mongo.host || "127.0.0.1",
    port: database.DB_PORT || mongo.port || DB_PORT_DEFAULTS.mongodb,
    authSource: mongo.auth_source || "admin",
    directConnection: mongo.direct_connection !== undefined ? Boolean(mongo.direct_connection) : true,
    timeoutMs: mongo.server_selection_timeout_ms || 2000,
  });

  return `
    <fieldset class="config-card">
      <legend>Base de datos</legend>
      ${select("database.DB_CONNECTION", "DB connection", engine, ["local", "mysql", "postgres", "mongodb"])}
      <div id="sql-db-fields" ${engine === "mongodb" ? "hidden" : ""}>
        ${input("database.DB_HOST", "DB host", database.DB_HOST || "")}
        ${input("database.DB_PORT", "DB port", database.DB_PORT || "")}
        ${input("database.DB_DATABASE", "DB database", database.DB_DATABASE || "")}
        ${input("database.DB_USERNAME", "DB username", database.DB_USERNAME || "")}
        ${input("database.DB_PASSWORD", "DB password", database.DB_PASSWORD || "", "password")}
      </div>
      <div id="mongo-db-fields" ${engine === "mongodb" ? "" : "hidden"}>
        <label class="field">
          <span>Mongo URI</span>
          <textarea id="mongo-uri-preview" rows="4" placeholder="mongodb://usuario:password@127.0.0.1:27017/project_iiif?directConnection=true&serverSelectionTimeoutMS=2000&authSource=admin">${escapeHTML(mongoURI)}</textarea>
        </label>
        <p class="field-help">Pega la misma URI que usas en MongoDB Compass. Esta version soporta formato <code>mongodb://</code> con host y puerto; <code>mongodb+srv://</code> aun no esta soportado.</p>
      </div>
    </fieldset>
  `;
}

function buildMongoURI({ host, port, authSource, directConnection, timeoutMs }) {
  const username = formField("database.DB_USERNAME")?.value || state.config?.database?.DB_USERNAME || "";
  const password = formField("database.DB_PASSWORD")?.value || "";
  const database = formField("database.DB_DATABASE")?.value || state.config?.database?.DB_DATABASE || "";
  const query = new URLSearchParams();
  query.set("directConnection", String(Boolean(directConnection)));
  query.set("serverSelectionTimeoutMS", String(timeoutMs || 2000));
  query.set("authSource", authSource || "admin");
  const credentials = username
    ? `${encodeURIComponent(username)}${password && password !== MASKED_SECRET ? `:${encodeURIComponent(password)}` : ""}@`
    : "";
  const databasePath = database ? `/${encodeURIComponent(database)}` : "/";
  return `mongodb://${credentials}${host || "127.0.0.1"}:${port || DB_PORT_DEFAULTS.mongodb}${databasePath}?${query.toString()}`;
}

function formField(name) {
  return $("#config-form")?.elements?.[name] || null;
}

function parseMongoURI(value) {
  try {
    const uri = new URL(value);
    if (uri.protocol !== "mongodb:") {
      return null;
    }
    return {
      host: uri.hostname || "",
      port: uri.port || DB_PORT_DEFAULTS.mongodb,
      database: decodeURIComponent((uri.pathname || "").replace(/^\/+/, "")) || "",
      username: decodeURIComponent(uri.username || ""),
      password: decodeURIComponent(uri.password || ""),
      authSource: uri.searchParams.get("authSource") || "admin",
      directConnection: uri.searchParams.get("directConnection") === null
        ? true
        : uri.searchParams.get("directConnection") === "true",
      serverSelectionTimeoutMS: Number.parseInt(uri.searchParams.get("serverSelectionTimeoutMS") || "2000", 10) || 2000,
    };
  } catch (error) {
    return null;
  }
}

function updateMongoURIPreview() {
  const preview = $("#mongo-uri-preview");
  const connection = formField("database.DB_CONNECTION");
  if (!preview || normalizedDBEngine(connection?.value || "local") !== "mongodb") return;
  if (state.mongoURIEdited) return;
  const mongo = state.config?.database?.mongodb || {};

  preview.value = buildMongoURI({
    host: formField("database.DB_HOST")?.value || "127.0.0.1",
    port: formField("database.DB_PORT")?.value || DB_PORT_DEFAULTS.mongodb,
    authSource: mongo.auth_source || "admin",
    directConnection: mongo.direct_connection !== undefined ? Boolean(mongo.direct_connection) : true,
    timeoutMs: mongo.server_selection_timeout_ms || 2000,
  });
}

function fieldset(title, fields) {
  return `
    <fieldset class="config-card">
      <legend>${escapeHTML(title)}</legend>
      ${fields.join("")}
    </fieldset>
  `;
}

function input(name, label, value, type = "text") {
  return `
    <label class="field">
      <span>${escapeHTML(label)}</span>
      <input name="${escapeHTML(name)}" type="${escapeHTML(type)}" value="${escapeHTML(value)}">
    </label>
  `;
}

function textarea(name, label, value) {
  return `
    <label class="field">
      <span>${escapeHTML(label)}</span>
      <textarea name="${escapeHTML(name)}" rows="10">${escapeHTML(value)}</textarea>
    </label>
  `;
}

function select(name, label, value, options) {
  return `
    <label class="field">
      <span>${escapeHTML(label)}</span>
      <select name="${escapeHTML(name)}">
        ${options.map((option) => `<option value="${escapeHTML(option)}" ${option === value ? "selected" : ""}>${escapeHTML(option)}</option>`).join("")}
      </select>
    </label>
  `;
}

function checkbox(name, label, value) {
  return `
    <label class="check-field">
      <input name="${escapeHTML(name)}" type="checkbox" ${value ? "checked" : ""}>
      <span>${escapeHTML(label)}</span>
    </label>
  `;
}

function statusBadge(status) {
  const safeStatus = escapeHTML(status || "unknown");
  const className = status === "completed" ? "completed" : status === "error" ? "error" : "";
  return `<span class="status ${className}">${safeStatus}</span>`;
}

function migratedBadge(value) {
  return value ? '<span class="status completed">Si</span>' : '<span class="status">No</span>';
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function showToast(message, type = "success") {
  const stack = $("#toast-stack");
  if (!stack) return;

  const toast = document.createElement("div");
  toast.className = `toast ${type === "error" ? "error" : ""}`;
  toast.textContent = message;
  stack.appendChild(toast);

  window.setTimeout(() => {
    toast.remove();
  }, 6000);
}
