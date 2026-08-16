import { useState } from "react";
import { api } from "../api";
import { applyMongoURI, mongoURIFromConfig } from "../lib/validation";
import type { AppConfig, BinaryMode, Engine } from "../types";
import { Alert, Button, Card, Checkbox, FormField, Input, PageHeader, Select, Spinner } from "../components/ui";

export function ConfigPage({ initial, onSaved }: { initial: AppConfig; onSaved: (config: AppConfig) => void }) {
  const [config, setConfig] = useState(initial);
  const [mongoURI, setMongoURI] = useState(() => mongoURIFromConfig(initial.database.mongodb));
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const engine = (config.storage.backend === "local" ? "mysql" : config.storage.backend) as Engine;

  const setEngine = (value: Engine) => setConfig((current) => ({ ...current, storage: { ...current.storage, backend: value }, database: { ...current.database, DB_CONNECTION: value } }));
  const setBinaryMode = (value: BinaryMode) => setConfig((current) => ({ ...current, binary_storage: { ...current.binary_storage, mode: value }, s3: { ...current.s3, filesystem_disk: value === "s3" ? "s3" : "local" } }));
  const updateDatabase = (name: string, value: string | boolean) => setConfig((current) => ({ ...current, database: { ...current.database, [engine]: { ...current.database[engine], [name]: value } } }));

  const save = async () => {
    setBusy(true); setError(""); setMessage("");
    try {
      let payload = config;
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
      setMessage("Configuración guardada. Los cambios de motor o almacenamiento requieren reiniciar el servicio.");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo guardar la configuración.");
    } finally { setBusy(false); }
  };

  return <>
    <PageHeader eyebrow="Administración" title="Configuración" description="Metadata, almacenamiento binario y conversión desde una sola vista." actions={<Button onClick={save} disabled={busy}>{busy ? <Spinner label="Guardando" /> : "Guardar cambios"}</Button>} />
    {message && <Alert tone="success">{message}</Alert>}{error && <Alert tone="danger">{error}</Alert>}
    <div className="settings-grid">
      <Card><div className="section-heading"><span className="step">1</span><div><h2>Motor de metadata</h2><p>Selecciona dónde se guardan documentos y metadatos.</p></div></div>
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
      </Card>

      <Card><div className="section-heading"><span className="step">2</span><div><h2>Binary storage</h2><p>Los PDFs e imágenes permanecen separados de la metadata.</p></div></div>
        <FormField label="Modo">{(id) => <Select id={id} value={config.binary_storage.mode} onChange={(e) => setBinaryMode(e.target.value as BinaryMode)}><option value="local">Local</option><option value="database">Base de datos</option><option value="s3">S3 / RustFS</option></Select>}</FormField>
        {config.binary_storage.mode === "s3" && <div className="form-grid two-columns" data-testid="s3-fields">
          <FormField label="Endpoint">{(id) => <Input id={id} type="url" value={config.s3.endpoint} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, endpoint: e.target.value } })} placeholder="http://rustfs:9000" />}</FormField>
          <FormField label="Bucket">{(id) => <Input id={id} value={config.s3.bucket} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, bucket: e.target.value } })} />}</FormField>
          <FormField label="Región">{(id) => <Input id={id} value={config.s3.region} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, region: e.target.value } })} />}</FormField>
          <FormField label="Access key" help={config.s3.access_key_configured ? "Configurada; deja vacío para conservarla." : undefined}>{(id) => <Input id={id} type="password" value={config.s3.access_key_id === "********" ? "" : config.s3.access_key_id} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, access_key_id: e.target.value } })} />}</FormField>
          <FormField label="Secret key" help={config.s3.secret_key_configured ? "Configurada; deja vacío para conservarla." : undefined}>{(id) => <Input id={id} type="password" value={config.s3.secret_access_key === "********" ? "" : config.s3.secret_access_key} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, secret_access_key: e.target.value } })} />}</FormField>
          <Checkbox label="Usar path-style" checked={config.s3.use_path_style_endpoint} onChange={(e) => setConfig({ ...config, s3: { ...config.s3, use_path_style_endpoint: e.target.checked } })} />
        </div>}
      </Card>

      <Card><div className="section-heading"><span className="step">3</span><div><h2>Conversión PDF</h2><p>Ancho y alto son límites; el aspect ratio se conserva.</p></div></div>
        <div className="form-grid two-columns">
          <FormField label="Ancho máximo">{(id) => <Input id={id} type="number" min="256" max="8192" value={config.conversion.default_width} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_width: Number(e.target.value) } })} />}</FormField>
          <FormField label="Alto máximo">{(id) => <Input id={id} type="number" min="256" max="8192" value={config.conversion.default_height} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_height: Number(e.target.value) } })} />}</FormField>
          <FormField label="DPI">{(id) => <Input id={id} type="number" min="72" max="600" value={config.conversion.dpi} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, dpi: Number(e.target.value) } })} />}</FormField>
          <FormField label="Formato">{(id) => <Select id={id} value={config.conversion.default_format} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_format: e.target.value as "jpg" | "png" } })}><option value="jpg">JPG</option><option value="png">PNG</option></Select>}</FormField>
          <FormField label="Calidad JPG">{(id) => <Input id={id} type="number" min="1" max="100" value={config.conversion.default_quality} onChange={(e) => setConfig({ ...config, conversion: { ...config.conversion, default_quality: Number(e.target.value) } })} />}</FormField>
        </div>
      </Card>
    </div>
  </>;
}
