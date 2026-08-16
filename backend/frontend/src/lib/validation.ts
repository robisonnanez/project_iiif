export function normalizePageSelection(value: string, totalPages: number): string {
  const trimmed = value.trim();
  if (!trimmed) throw new Error("Escribe páginas, por ejemplo 1-5,8,10-12.");
  const selected = new Set<number>();
  for (const rawPart of trimmed.split(",")) {
    const part = rawPart.trim();
    if (!part) throw new Error("La selección contiene un segmento vacío.");
    if (!/^\d+(?:\s*-\s*\d+)?$/.test(part)) throw new Error(`El segmento “${part}” no es válido.`);
    const [startText, endText = startText] = part.split("-").map((item) => item.trim());
    const start = Number(startText);
    const end = Number(endText);
    if (start < 1 || end < 1 || start > end) throw new Error(`El rango “${part}” no es válido.`);
    if (start > totalPages || end > totalPages) throw new Error(`Las páginas deben estar entre 1 y ${totalPages}.`);
    for (let page = start; page <= end; page += 1) selected.add(page);
  }
  return [...selected].sort((a, b) => a - b).join(",");
}

export function mongoURIFromConfig(mongo: {
  host: string; port: string; user: string; password: string; database: string; auth_source: string; direct_connection: boolean; server_selection_timeout_ms: number;
}): string {
  const credentials = mongo.user ? `${encodeURIComponent(mongo.user)}:${mongo.password === "********" ? "" : encodeURIComponent(mongo.password)}@` : "";
  const params = new URLSearchParams();
  if (mongo.auth_source) params.set("authSource", mongo.auth_source);
  params.set("directConnection", String(mongo.direct_connection));
  params.set("serverSelectionTimeoutMS", String(mongo.server_selection_timeout_ms || 2000));
  return `mongodb://${credentials}${mongo.host || "localhost"}:${mongo.port || "27017"}/${mongo.database || "project_iiif"}?${params}`;
}

export function applyMongoURI<T extends { database: { mongodb: Record<string, unknown>; DB_HOST: string; DB_PORT: string; DB_DATABASE: string; DB_USERNAME: string; DB_PASSWORD: string } }>(config: T, uri: string): T {
  const parsed = new URL(uri);
  if (parsed.protocol !== "mongodb:") throw new Error("La URI debe comenzar con mongodb://");
  const database = parsed.pathname.replace(/^\//, "");
  if (!parsed.hostname || !database) throw new Error("La URI debe incluir host y base de datos.");
  return {
    ...config,
    database: {
      ...config.database,
      DB_HOST: parsed.hostname,
      DB_PORT: parsed.port || "27017",
      DB_DATABASE: database,
      DB_USERNAME: decodeURIComponent(parsed.username),
      DB_PASSWORD: parsed.password ? decodeURIComponent(parsed.password) : String(config.database.mongodb.password ?? ""),
      mongodb: {
        ...config.database.mongodb,
        host: parsed.hostname,
        port: parsed.port || "27017",
        user: decodeURIComponent(parsed.username),
        password: parsed.password ? decodeURIComponent(parsed.password) : config.database.mongodb.password,
        database,
        auth_source: parsed.searchParams.get("authSource") || "admin",
        direct_connection: parsed.searchParams.get("directConnection") !== "false",
        server_selection_timeout_ms: Number(parsed.searchParams.get("serverSelectionTimeoutMS") || 2000),
      },
    },
  };
}
