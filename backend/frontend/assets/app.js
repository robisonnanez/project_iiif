const MASKED_SECRET = "********";

const state = {
  documents: [],
  config: null,
  polling: new Map(),
};

const viewMeta = {
  summary: ["Inicio", "Estado general del servidor IIIF."],
  upload: ["Subir PDF", "Carga un documento PDF y lanza la conversion."],
  documents: ["Documentos", "Consulta documentos, estados y manifiestos."],
  images: ["Imagenes", "Galeria de paginas servidas por IIIF."],
  config: ["Configuracion", "Edita valores permitidos del config.yaml sin exponer secretos."],
};

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => Array.from(document.querySelectorAll(selector));

document.addEventListener("DOMContentLoaded", () => {
  $$(".nav-item").forEach((button) => {
    button.addEventListener("click", () => showView(button.dataset.view));
  });

  $("#refresh-button").addEventListener("click", refreshCurrentView);
  $("#upload-form").addEventListener("submit", uploadPDF);
  $("#logout-button").addEventListener("click", logout);
  $("#load-images-button").addEventListener("click", loadSelectedImages);

  loadDocuments();
  loadConfig();
});

function showView(name) {
  $$(".nav-item").forEach((button) => button.classList.toggle("active", button.dataset.view === name));
  $$(".view").forEach((view) => view.classList.toggle("active", view.id === `${name}-view`));

  const meta = viewMeta[name] || viewMeta.summary;
  $("#view-title").textContent = meta[0];
  $("#view-subtitle").textContent = meta[1];

  if (name === "documents") loadDocuments();
  if (name === "images") {
    loadDocuments().then(() => renderImageDocumentOptions());
  }
  if (name === "config") loadConfig();
}

function refreshCurrentView() {
  const active = $(".nav-item.active")?.dataset.view || "summary";
  if (active === "config") {
    loadConfig();
  } else if (active === "images") {
    loadSelectedImages();
  } else {
    loadDocuments();
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

async function loadDocuments() {
  try {
    const response = await fetch("/api/documents");
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.documents = await response.json();
    renderSummary();
    renderDocuments();
    renderImageDocumentOptions();
  } catch (error) {
    $("#recent-documents").textContent = `No se pudieron cargar documentos: ${error.message}`;
    $("#documents-table").innerHTML = `<tr><td colspan="5">No se pudieron cargar documentos.</td></tr>`;
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

async function uploadPDF(event) {
  event.preventDefault();
  const fileInput = $("#pdf-file");
  const result = $("#upload-result");
  const file = fileInput.files[0];

  if (!file) return;

  const formData = new FormData();
  formData.append("pdf", file);

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
    await loadDocuments();
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
        await loadDocuments();
      } else if (doc.status === "error") {
        window.clearInterval(timer);
        state.polling.delete(documentID);
        showToast("La conversion del PDF fallo.", "error");
        await loadDocuments();
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
  const docs = Array.isArray(state.documents) ? state.documents : [];
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
  const docs = Array.isArray(state.documents) ? state.documents : [];
  if (!docs.length) {
    $("#documents-table").innerHTML = `<tr><td colspan="5">Sin documentos cargados.</td></tr>`;
    return;
  }

  $("#documents-table").innerHTML = docs.map((doc) => `
    <tr>
      <td><code>${escapeHTML(doc.id || "")}</code></td>
      <td>${escapeHTML(doc.name || "")}</td>
      <td>${statusBadge(doc.status)}</td>
      <td>${Number(doc.convertedPages || 0)} / ${Number(doc.totalPages || 0)}</td>
      <td>${doc.manifestUrl ? `<a href="${escapeHTML(doc.manifestUrl)}" target="_blank" rel="noreferrer">Ver</a>` : "-"}</td>
    </tr>
  `).join("");
}

function renderImageDocumentOptions() {
  const select = $("#images-document");
  if (!select) return;

  const completedDocs = (state.documents || []).filter((doc) => doc.status === "completed");
  if (!completedDocs.length) {
    select.innerHTML = `<option value="">Sin documentos completados</option>`;
    $("#images-gallery").className = "image-gallery empty";
    $("#images-gallery").textContent = "No hay imagenes disponibles.";
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
    renderImages(data.images || []);
  } catch (error) {
    gallery.className = "image-gallery empty";
    gallery.textContent = `No se pudieron cargar imagenes: ${error.message}`;
  }
}

function renderImages(images) {
  const gallery = $("#images-gallery");
  if (!images.length) {
    gallery.className = "image-gallery empty";
    gallery.textContent = "Este documento no tiene paginas convertidas.";
    return;
  }

  gallery.className = "table-wrap images-table-wrap";
  gallery.innerHTML = `
    <table class="images-table">
      <thead>
        <tr>
          <th>Miniatura</th>
          <th>Pagina</th>
          <th>ID imagen</th>
          <th>Dimensiones</th>
          <th>URL IIIF</th>
          <th>Accion</th>
        </tr>
      </thead>
      <tbody>
        ${images.map((image) => `
          <tr>
            <td>
              <a class="thumb-link" href="${escapeHTML(image.iiif_url)}" target="_blank" rel="noreferrer">
                <img class="image-thumb" src="${escapeHTML(image.iiif_url)}" alt="Pagina ${Number(image.page_number || 0)}">
              </a>
            </td>
            <td>${Number(image.page_number || 0)}</td>
            <td><code>${escapeHTML(image.image_id || "")}</code></td>
            <td>${Number(image.width || 0)} x ${Number(image.height || 0)} px</td>
            <td><a class="iiif-url" href="${escapeHTML(image.iiif_url)}" target="_blank" rel="noreferrer">${escapeHTML(image.iiif_url)}</a></td>
            <td><a class="table-action" href="${escapeHTML(image.iiif_url)}" target="_blank" rel="noreferrer">Abrir</a></td>
          </tr>
        `).join("")}
      </tbody>
    </table>
  `;
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
  } catch (error) {
    showToast(`No se pudo guardar: ${error.message}`, "error");
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
  };
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
