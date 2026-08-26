import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import { applyMongoURI, mongoURIFromConfig } from "../lib/validation";
import type { AppConfig, BinaryMode, DBMigrationResult, Engine } from "../types";
import { Alert, Button, Card, Checkbox, FormField, Input, Modal, PageHeader, Select, Spinner } from "../components/ui";

export function ConfigPage({ initial, onSaved }: { initial: AppConfig; onSaved: (config: AppConfig) => void }) {
  const [config, setConfig] = useState(initial);
  const [mongoURI, setMongoURI] = useState(() => mongoURIFromConfig(initial.database.mongodb));
  const [corsOrigins, setCorsOrigins] = useState(() => initial.security.cors_origins.join("\n"));
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [restartOpen, setRestartOpen] = useState(false);
  const [sudoPassword, setSudoPassword] = useState("");
  const [restartBusy, setRestartBusy] = useState(false);
  const [restartError, setRestartError] = useState("");
  const [restartScheduled, setRestartScheduled] = useState(false);
  const [migrationResult, setMigrationResult] = useState<DBMigrationResult | null>(null);
  const [migrationBusy, setMigrationBusy] = useState(false);
  const [migrationError, setMigrationError] = useState("");
  const engine = (config.storage.backend === "local" ? "mysql" : config.storage.backend) as Engine;

  useEffect(() => {
    if (!restartScheduled) return;
    let cancelled = false;
    let timer = 0;
    let attempts = 0;
    const check = async () => {
      try {
        const health = await api.serviceHealth();
        if (!cancelled && health.status === "ok") {
          setRestartScheduled(false);
          setMessage("Configuración aplicada. El servicio ya está disponible en el puerto 8080.");
          return;
        }
      } catch { /* El servicio puede estar temporalmente fuera de línea durante el reinicio. */ }
      attempts += 1;
      if (!cancelled && attempts < 60) timer = window.setTimeout(check, 1000);
      else if (!cancelled) {
        setRestartScheduled(false);
        setError("La configuración fue guardada, pero no se pudo confirmar que el servicio volviera a estar disponible.");
      }
    };
    timer = window.setTimeout(check, 2000);
    return () => { cancelled = true; window.clearTimeout(timer); };
  }, [restartScheduled]);

  useEffect(() => {
    void api.dbMigrationStatus().then((status) => setMigrationResult(status.result)).catch(() => undefined);
  }, []);

  const setEngine = (value: Engine) => setConfig((current) => ({ ...current, storage: { ...current.storage, backend: value }, database: { ...current.database, DB_CONNECTION: value } }));
  const setBinaryMode = (value: BinaryMode) => setConfig((current) => ({ ...current, binary_storage: { ...current.binary_storage, mode: value }, s3: { ...current.s3, filesystem_disk: value === "s3" ? "s3" : "local" } }));
  const updateDatabase = (name: string, value: string | boolean) => setConfig((current) => ({ ...current, database: { ...current.database, [engine]: { ...current.database[engine], [name]: value } } }));

  const save = async () => {
    setBusy(true); setError(""); setMessage("");
    try {
      let payload = { ...config, security: { ...config.security, cors_origins: parseCorsOrigins(corsOrigins) } };
      if (engine === "mongodb") payload = applyMongoURI(payload, mongoURI) as AppConfig;
      const active = payload.database[engine];
      payload = { ...payload, database: {
        ...payload.database,
        DB_CONNECTION: engine,
        DB_HOST: active.host,
        DB_PORT: active.port,
        DB_DATABASE: active.database,
        DB_USERNAME: active.user,
        DB_PASSWORD: active.password,
      } };
      await api.saveConfig(payload);
      setConfig(payload); onSaved(payload);
      setMessage("Configuración guardada.");
      setRestartError("");
      setRestartOpen(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar la configuración.");
    } finally { setBusy(false); }
  };

  const closeRestart = useCallback(() => {
    if (restartBusy) return;
    setRestartOpen(false);
    setSudoPassword("");
    setRestartError("");
  }, [restartBusy]);

  const restart = async () => {
    if (!sudoPassword.trim()) {
      setRestartError("Ingresa la contraseña del servidor para reiniciar el servicio.");
      return;
    }
    setRestartBusy(true); setRestartError("");
    try {
      await api.restartService(sudoPassword);
      setRestartOpen(false);
      setSudoPassword("");
      setRestartScheduled(true);
      setMessage("Configuración guardada. Reinicio en curso; comprobando disponibilidad…");
    } catch (cause) {
      setRestartError(cause instanceof Error ? cause.message : "No se pudo reiniciar el servicio.");
      setSudoPassword("");
    } finally { setRestartBusy(false); }
  };

  const runMigrations = async () => {
    setMigrationBusy(true); setMigrationError("");
    try {
      const result = await api.runDBMigrations();
      setMigrationResult(result);
      setMessage(result.applied > 0 ? `${result.applied} migración(es) aplicadas en ${result.engine}.` : `El esquema de ${result.engine} ya está actualizado.`);
    } catch (cause) {
      setMigrationError(cause instanceof Error ? cause.message : "No se pudieron ejecutar las migraciones.");
    } finally { setMigrationBusy(false); }
  };

  return <>
    <PageHeader eyebrow="Administración" title="Configuración" description="Metadatos, almacenamiento, conversión, OCR y seguridad desde una sola vista." actions={<Button onClick={save} disabled={busy}>{busy ? <Spinner label="Guardando" /> : "Guardar cambios"}</Button>} />
    {message && <Alert tone="success">{message}</Alert>}{error && <Alert tone="danger">{error}</Alert>}
    <div className="settings-grid">
      <Card><div className="section-heading"><span className="step">1</span><div><h2>Motor de metadatos</h2><p>Selecciona dónde se guardan documentos y metadatos.</p></div></div>
        <FormField label="Motor">{(id) => <Select id={id} value={engine} onChange={(e) => setEngine(e.target.value as Engine)}><option value="mysql">MySQL</option><option value="postgres">PostgreSQL</option><option value="mongodb">MongoDB</option></Select>}</FormField>
        {engine === "mongodb" ? (
          <FormField label="Mongo URI" help="Deja la contraseña vacía para conservar la configurada.">
            {(id) => (
              <Input
                id={id}
                value={mongoURI}
                onChange={(event) => setMongoURI(event.target.value)}
                autoComplete="off"
              />
            )}
          </FormField>
        ) : <div className="form-grid two-columns">
          <FormField label="Host">{(id) => <Input id={id} value={config.database[engine].host} onChange={(e) => updateDatabase("host", e.target.value)} />}</FormField>
          <FormField label="Puerto">{(id) => <Input id={id} value={config.database[engine].port} onChange={(e) => updateDatabase("port", e.target.value)} />}</FormField>
          <FormField label="Base de datos">{(id) => <Input id={id} value={config.database[engine].database} onChange={(e) => updateDatabase("database", e.target.value)} />}</FormField>
          <FormField label="Usuario">{(id) => <Input id={id} value={config.database[engine].user} onChange={(e) => updateDatabase("user", e.target.value)} />}</FormField>
          <FormField label="Contraseña" help={config.database[engine].password_configured ? "Ya existe una contraseña. Déjala vacía para conservarla." : undefined}>{(id) => <Input id={id} type="password" value={config.database[engine].password === "********" ? "" : config.database[engine].password} onChange={(e) => updateDatabase("password", e.target.value)} autoComplete="new-password" />}</FormField>
          {engine === "postgres" && <FormField label="SSL mode">{(id) => <Select id={id} value={config.database.postgres.sslmode} onChange={(e) => updateDatabase("sslmode", e.target.value)}><option value="disable">disable</option><option value="require">require</option><option value="verify-full">verify-full</option></Select>}</FormField>}
        </div>}
        <div className="config-subsection database-migrations">
          <h3>Migraciones del esquema</h3>
          <p>Crea y actualiza las tablas o índices del motor activo. Guarda primero cualquier cambio de conexión.</p>
          <Checkbox label="Ejecutar migraciones pendientes al iniciar el servicio" checked={config.database.auto_migrate} onChange={(event) => setConfig({ ...config, database: { ...config.database, auto_migrate: event.target.checked } })} />
          <div className="tenant-sync-row"><Button type="button" variant="secondary" disabled={migrationBusy || config.storage.backend === "local"} onClick={() => void runMigrations()}>{migrationBusy ? <Spinner label="Ejecutando migraciones" /> : "Ejecutar migraciones ahora"}</Button>{migrationResult && <span>Motor: {migrationResult.engine || "—"} · Aplicadas: {migrationResult.applied || 0} · Omitidas: {migrationResult.skipped || 0}</span>}</div>
          {migrationError && <Alert tone="danger">{migrationError}</Alert>}
        </div>
      </Card>

      <Card><div className="section-heading"><span className="step">2</span><div><h2>Almacenamiento binario</h2><p>Los PDF y las imágenes permanecen separados de los metadatos.</p></div></div>
        <FormField label="Modo">{(id) => <Select id={id} value={config.binary_storage.mode} onChange={(e) => setBinaryMode(e.target.value as BinaryMode)}><option value="local">Local</option><option value="database">Base de datos</option><option value="s3">S3 / RustFS</option></Select>}</FormField>
        {config.binary_storage.mode === "s3" && <div className="form-grid two-columns" data-testid="s3-fields">
          <FormField label="Endpoint">{(id) => <Input id={id} type="url" value={config.s3.endpoint} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, endpoint: e.target.value } })} placeholder="http://rustfs:9000" />}</FormField>
          <FormField label="Bucket">{(id) => <Input id={id} value={config.s3.bucket} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, bucket: e.target.value } })} />}</FormField>
          <FormField label="Región">{(id) => <Input id={id} value={config.s3.region} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, region: e.target.value } })} />}</FormField>
          <FormField label="Clave de acceso" help={config.s3.access_key_configured ? "Configurada; deja vacío para conservarla." : undefined}>{(id) => <Input id={id} type="password" value={config.s3.access_key_id === "********" ? "" : config.s3.access_key_id} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, access_key_id: e.target.value } })} />}</FormField>
          <FormField label="Clave secreta" help={config.s3.secret_key_configured ? "Configurada; deja vacío para conservarla." : undefined}>{(id) => <Input id={id} type="password" value={config.s3.secret_access_key === "********" ? "" : config.s3.secret_access_key} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, secret_access_key: e.target.value } })} />}</FormField>
          <Checkbox label="Usar direcciones con estilo de ruta" checked={config.s3.use_path_style_endpoint} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, use_path_style_endpoint: e.target.checked } })} />
        </div>}
      </Card>

      <Card><div className="section-heading"><span className="step">3</span><div><h2>Conversión de PDF</h2><p>El ancho y el alto son límites; se conserva la relación de aspecto.</p></div></div>
        <div className="form-grid two-columns">
          <FormField label="Ancho máximo">{(id) => <Input id={id} type="number" min="256" max="8192" value={config.conversion.default_width} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_width: Number(e.target.value) } })} />}</FormField>
          <FormField label="Alto máximo">{(id) => <Input id={id} type="number" min="256" max="8192" value={config.conversion.default_height} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_height: Number(e.target.value) } })} />}</FormField>
          <FormField label="DPI">{(id) => <Input id={id} type="number" min="72" max="600" value={config.conversion.dpi} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, dpi: Number(e.target.value) } })} />}</FormField>
          <FormField label="Formato">{(id) => <Select id={id} value={config.conversion.default_format} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_format: e.target.value as "jpg" | "png" } })}><option value="jpg">JPG</option><option value="png">PNG</option></Select>}</FormField>
          <FormField label="Calidad JPG">{(id) => <Input id={id} type="number" min="1" max="100" value={config.conversion.default_quality} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_quality: Number(e.target.value) } })} />}</FormField>
        </div>
      </Card>

      <Card><div className="section-heading"><span className="step">4</span><div><h2>OCR e indexación</h2><p>Configura Tesseract, la detección de idioma y el procesamiento automático.</p></div></div>
        <div className="form-grid two-columns">
          <Checkbox label="Activar OCR" checked={config.ocr.enabled} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, enabled: event.target.checked }, conversion: { ...config.conversion, enable_ocr: event.target.checked } })} />
          <Checkbox label="Ejecutar después de convertir" checked={config.ocr.auto_after_conversion} disabled={!config.ocr.enabled} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, auto_after_conversion: event.target.checked } })} />
          <FormField label="Modo predeterminado">{(id) => <Select id={id} value={config.ocr.default_mode} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, default_mode: event.target.value as AppConfig["ocr"]["default_mode"] } })}><option value="hybrid">Híbrido por página</option><option value="exhaustive">Exhaustivo</option><option value="ocr_only">Solo OCR</option></Select>}</FormField>
          <FormField label="Procesos concurrentes" help="Para este servidor se recomiendan 2.">{(id) => <Input id={id} type="number" min="1" max="16" value={config.ocr.workers} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, workers: Number(event.target.value) } })} />}</FormField>
          <FormField label="DPI para OCR">{(id) => <Input id={id} type="number" min="150" max="600" value={config.ocr.render_dpi} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, render_dpi: Number(event.target.value) } })} />}</FormField>
          <FormField label="Tiempo límite por página (segundos)">{(id) => <Input id={id} type="number" min="10" max="3600" value={config.ocr.page_timeout_seconds} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, page_timeout_seconds: Number(event.target.value) } })} />}</FormField>
          <FormField label="Reintentos por página">{(id) => <Input id={id} type="number" min="1" max="10" value={config.ocr.retries_per_page} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, retries_per_page: Number(event.target.value) } })} />}</FormField>
          <FormField label="Caracteres mínimos de capa PDF">{(id) => <Input id={id} type="number" min="1" max="10000" value={config.ocr.min_text_chars} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, min_text_chars: Number(event.target.value) } })} />}</FormField>
        </div>
        <div className="config-subsection"><h3>Idiomas instalados</h3><p>Selecciona los idiomas que podrá usar la detección automática.</p><div className="language-options">{languageOptions.map(({ code, label }) => <Checkbox key={code} label={`${label} (${code})`} checked={config.ocr.candidate_languages.includes(code)} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, candidate_languages: toggleList(config.ocr.candidate_languages, code, event.target.checked), fallback_languages: event.target.checked ? config.ocr.fallback_languages : config.ocr.fallback_languages.filter((item) => item !== code) } })} />)}</div></div>
        <div className="config-subsection"><h3>Idioma de respaldo</h3><p>Se usa cuando no hay texto suficiente para una detección confiable.</p><div className="language-options">{languageOptions.map(({ code, label }) => <Checkbox key={code} label={`${label} (${code})`} disabled={!config.ocr.candidate_languages.includes(code)} checked={config.ocr.fallback_languages.includes(code)} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, fallback_languages: toggleList(config.ocr.fallback_languages, code, event.target.checked) } })} />)}</div></div>
        <div className="form-grid two-columns config-subsection">
          <Checkbox label="Detectar idioma automáticamente" checked={config.ocr.language_detection.enabled} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, language_detection: { ...config.ocr.language_detection, enabled: event.target.checked } } })} />
          <Checkbox label="Comprimir artefactos JSON" checked={config.ocr.artifacts.gzip} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, artifacts: { ...config.ocr.artifacts, gzip: event.target.checked } } })} />
          <FormField label="Páginas de muestra">{(id) => <Input id={id} type="number" min="1" max="20" value={config.ocr.language_detection.sample_pages} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, language_detection: { ...config.ocr.language_detection, sample_pages: Number(event.target.value) } } })} />}</FormField>
          <FormField label="Caracteres mínimos de muestra">{(id) => <Input id={id} type="number" min="20" max="10000" value={config.ocr.language_detection.min_sample_chars} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, language_detection: { ...config.ocr.language_detection, min_sample_chars: Number(event.target.value) } } })} />}</FormField>
          <FormField label="Confianza mínima">{(id) => <Input id={id} type="number" min="0.01" max="1" step="0.05" value={config.ocr.language_detection.minimum_confidence} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, language_detection: { ...config.ocr.language_detection, minimum_confidence: Number(event.target.value) } } })} />}</FormField>
          <FormField label="Máximo de idiomas">{(id) => <Input id={id} type="number" min="1" max="4" value={config.ocr.language_detection.max_languages} onChange={(event) => setConfig({ ...config, ocr: { ...config.ocr, language_detection: { ...config.ocr.language_detection, max_languages: Number(event.target.value) } } })} />}</FormField>
        </div>
      </Card>

      <Card><div className="section-heading"><span className="step">5</span><div><h2>Seguridad y CORS</h2><p>Autoriza los sitios web que pueden consumir la API desde el navegador.</p></div></div>
        <div className="form-grid two-columns">
          <Checkbox label="Activar autenticación de API" checked={config.security.enable_auth} onChange={(event) => setConfig({ ...config, security: { ...config.security, enable_auth: event.target.checked } })} />
          <FormField label="Máximo de cargas concurrentes">{(id) => <Input id={id} type="number" min="1" max="100" value={config.security.max_concurrent_uploads} onChange={(event) => setConfig({ ...config, security: { ...config.security, max_concurrent_uploads: Number(event.target.value) } })} />}</FormField>
        </div>
        <FormField label="Direcciones URL permitidas por CORS" help="Una URL por línea. Se aceptan puertos y subdominios con comodín, por ejemplo https://*.dominio.com.">{(id) => <textarea id={id} className="input cors-textarea" rows={6} value={corsOrigins} onChange={(event) => setCorsOrigins(event.target.value)} placeholder={"https://app.ejemplo.com\nhttp://localhost:5173"} />}</FormField>
      </Card>

      <Card><div className="section-heading"><span className="step">6</span><div><h2>Apariencia del panel</h2><p>Elige cómo se presenta la navegación principal en pantallas de escritorio.</p></div></div>
        <FormField label="Orientación del menú" help="En móviles siempre se utiliza el menú compacto.">{(id) => <Select id={id} value={config.frontend.menu_orientation} onChange={(event) => setConfig({ ...config, frontend: { ...config.frontend, menu_orientation: event.target.value as AppConfig["frontend"]["menu_orientation"] } })}><option value="horizontal">Horizontal superior</option><option value="vertical">Vertical lateral</option></Select>}</FormField>
      </Card>
    </div>
    {restartOpen && <Modal title="Reiniciar servicio" description="La configuración ya fue guardada. Reinicia el servicio para aplicar los cambios." onClose={closeRestart}>
      <form onSubmit={(event) => { event.preventDefault(); void restart(); }}>
        <FormField label="Contraseña del servidor" help="Se usa únicamente para autorizar este reinicio y no se guarda.">{(id) => <Input id={id} type="password" value={sudoPassword} onChange={(event) => setSudoPassword(event.target.value)} autoComplete="current-password" disabled={restartBusy} />}</FormField>
        {restartError && <Alert tone="danger">{restartError}</Alert>}
        <div className="modal-actions"><Button type="button" variant="secondary" disabled={restartBusy} onClick={closeRestart}>Ahora no</Button><Button type="submit" disabled={restartBusy}>{restartBusy ? <Spinner label="Reiniciando" /> : "Reiniciar servicio"}</Button></div>
      </form>
    </Modal>}
  </>;
}

const languageOptions = [{ code: "spa", label: "Español" }, { code: "eng", label: "Inglés" }, { code: "fra", label: "Francés" }, { code: "por", label: "Portugués" }];
const toggleList = (values: string[], value: string, checked: boolean) => checked ? [...new Set([...values, value])] : values.filter((item) => item !== value);
const parseCorsOrigins = (value: string) => [...new Set(value.split(/\r?\n|,/).map((item) => item.trim()).filter(Boolean))];
