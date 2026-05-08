const state = {
  documents: [],
  config: null,
};

const viewMeta = {
  summary: ["Inicio", "Estado general del servidor IIIF."],
  upload: ["Subir PDF", "Carga un documento PDF y lanza la conversion."],
  documents: ["Documentos", "Consulta documentos, estados y manifiestos."],
  config: ["Configuracion", "Valores activos del config.yaml sin secretos."],
};

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => Array.from(document.querySelectorAll(selector));

document.addEventListener("DOMContentLoaded", () => {
  $$(".nav-item").forEach((button) => {
    button.addEventListener("click", () => showView(button.dataset.view));
  });

  $("#refresh-button").addEventListener("click", refreshCurrentView);
  $("#upload-form").addEventListener("submit", uploadPDF);

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
  if (name === "config") loadConfig();
}

function refreshCurrentView() {
  const active = $(".nav-item.active")?.dataset.view || "summary";
  if (active === "config") {
    loadConfig();
  } else {
    loadDocuments();
  }
}

async function loadDocuments() {
  try {
    const response = await fetch("/api/documents");
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    state.documents = await response.json();
    renderSummary();
    renderDocuments();
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
    renderConfig();
  } catch (error) {
    $("#config-output").innerHTML = `<div class="config-card"><pre>No se pudo cargar configuracion: ${escapeHTML(error.message)}</pre></div>`;
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
    await loadDocuments();
  } catch (error) {
    result.textContent = `Error: ${error.message}`;
  }
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

function renderConfig() {
  const config = state.config;
  if (!config) return;

  const sections = [
    ["Servidor", config.server],
    ["Storage", config.storage],
    ["Base de datos", config.database],
    ["Frontend", config.frontend],
    ["IIIF", config.iiif],
  ];

  $("#config-output").innerHTML = sections.map(([title, value]) => `
    <article class="config-card">
      <h3>${escapeHTML(title)}</h3>
      <pre>${escapeHTML(JSON.stringify(value, null, 2))}</pre>
    </article>
  `).join("");
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
