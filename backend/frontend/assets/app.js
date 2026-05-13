const MASKED_SECRET = "********";

const state = {
  allDocuments: [],
  filteredDocuments: [],
  config: null,
  projects: { enabled: false, default_project: "default", items: [] },
  images: { items: [], page: 1, pageSize: 10 },
  migration: { timer: null, running: false },
  polling: new Map(),
};

const viewMeta = {
  summary: ["Inicio", "Estado general del servidor IIIF."],
  upload: ["Subir PDF", "Carga un documento PDF y lanza la conversion."],
  documents: ["Documentos", "Consulta documentos, estados y manifiestos."],
  images: ["Imagenes", "Galeria de paginas servidas por IIIF."],
  config: ["Configuracion", "Edita valores permitidos del config.yaml sin exponer secretos."],
  migration: ["Migracion", "Migra datos locales hacia MySQL BLOB de forma controlada."],
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
    const response = await fetch("/admin/api/migrations/local-to-mysql/start", {
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
    const response = await fetch("/admin/api/migrations/local-to-mysql/status");
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
    const response = await fetch(`/admin/api/migrations/sources/local/browse${query}`);
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
    const response = await fetch("/api/documents");
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
    const response = await fetch(`/api/documents${params ? `?${params}` : ""}`);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.filteredDocuments = await response.json();
    renderDocuments();
  } catch (error) {
    $("#documents-table").innerHTML = `<tr><td colspan="8">No se pudieron cargar documentos: ${escapeHTML(error.message)}</td></tr>`;
  }
}

async function loadConfig() {
  try {
    const response = await fetch("/admin/api/config");
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.config = await response.json();
    renderSummary();
    renderConfigForm();
  } catch (error) {
    $("#config-form").innerHTML = `<div class="config-card"><pre>No se pudo cargar configuracion: ${escapeHTML(error.message)}</pre></div>`;
  }
}

async function loadProjects() {
  try {
    const response = await fetch("/admin/api/projects");
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
    const response = await fetch("/api/upload", {
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
      const response = await fetch(`/api/documents/${encodeURIComponent(documentID)}`);
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
    const response = await fetch(`/admin/api/documents/${encodeURIComponent(documentID)}/images`);
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

function renderConfigForm() {
  const config = state.config;
  if (!config) return;

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
      ${fieldset("Base de datos", [
        select("database.DB_CONNECTION", "DB connection", config.database?.DB_CONNECTION || "local", ["local", "mysql", "postgres", "mongodb"]),
        input("database.DB_HOST", "DB host", config.database?.DB_HOST || ""),
        input("database.DB_PORT", "DB port", config.database?.DB_PORT || ""),
        input("database.DB_DATABASE", "DB database", config.database?.DB_DATABASE || ""),
        input("database.DB_USERNAME", "DB username", config.database?.DB_USERNAME || ""),
        input("database.DB_PASSWORD", "DB password", config.database?.DB_PASSWORD || "", "password"),
        input("database.mysql.host", "MySQL host", config.database?.mysql?.host || ""),
        input("database.mysql.port", "MySQL port", config.database?.mysql?.port || ""),
        input("database.mysql.user", "MySQL user", config.database?.mysql?.user || ""),
        input("database.mysql.password", "MySQL password", config.database?.mysql?.password || "", "password"),
        input("database.mysql.database", "MySQL database", config.database?.mysql?.database || ""),
        input("database.mysql.charset", "MySQL charset", config.database?.mysql?.charset || ""),
        checkbox("database.mysql.parse_time", "MySQL parse time", Boolean(config.database?.mysql?.parse_time))
      ])}
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
      ${fieldset("Proyectos", [
        checkbox("projects.enabled", "Proyectos activos", Boolean(config.projects?.enabled)),
        input("projects.default_project", "Proyecto por defecto", config.projects?.default_project || "default"),
        checkbox("projects.require_project", "Proyecto obligatorio", Boolean(config.projects?.require_project)),
        checkbox("projects.allow_dynamic_tenants", "Permitir tenants dinamicos", Boolean(config.projects?.allow_dynamic_tenants)),
        textarea("projects.items", "Proyectos JSON", JSON.stringify(config.projects?.items || [], null, 2))
      ])}
    </div>
    <div class="form-actions">
      <button class="primary" type="submit">Guardar configuracion</button>
      <span class="muted">Los secretos en ${MASKED_SECRET} se conservan sin cambios.</span>
    </div>
  `;

  $("#config-form").addEventListener("submit", saveConfig);
}

async function saveConfig(event) {
  // Envia solo el formulario permitido y deja que el backend valide antes de escribir config.yaml.
  event.preventDefault();
  const payload = collectConfigPayload();
  try {
    const response = await fetch("/admin/api/config", {
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
  if (status) status.textContent = "Reiniciando servicio...";

  try {
    const response = await fetch("/admin/api/service/restart", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    });
    const data = await response.json();
    if (!response.ok || !data.ok) {
      throw new Error(data.error || data.details || `HTTP ${response.status}`);
    }
    if (status) status.textContent = "Servicio reiniciado correctamente.";
    showToast(data.message || "Servicio reiniciado correctamente.");
    if (passwordInput) passwordInput.value = "";
    window.setTimeout(() => {
      const modal = $("#restart-service-modal");
      if (modal) modal.hidden = true;
    }, 1200);
  } catch (error) {
    if (status) status.textContent = `No se pudo reiniciar: ${error.message}`;
    showToast(`No se pudo reiniciar servicio: ${error.message}`, "error");
  } finally {
    if (restartButton) restartButton.disabled = false;
    if (laterButton) laterButton.disabled = false;
  }
}

function collectConfigPayload() {
  const form = $("#config-form");
  const value = (name) => form.elements[name]?.value || "";
  const checked = (name) => Boolean(form.elements[name]?.checked);
  const intValue = (name) => Number.parseInt(value(name), 10) || 0;

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
      DB_CONNECTION: value("database.DB_CONNECTION"),
      DB_HOST: value("database.DB_HOST"),
      DB_PORT: value("database.DB_PORT"),
      DB_DATABASE: value("database.DB_DATABASE"),
      DB_USERNAME: value("database.DB_USERNAME"),
      DB_PASSWORD: value("database.DB_PASSWORD"),
      mysql: {
        host: value("database.mysql.host"),
        port: value("database.mysql.port"),
        user: value("database.mysql.user"),
        password: value("database.mysql.password"),
        database: value("database.mysql.database"),
        charset: value("database.mysql.charset"),
        parse_time: checked("database.mysql.parse_time"),
      },
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
  };
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
