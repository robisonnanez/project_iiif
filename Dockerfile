FROM node:22-bookworm-slim AS frontend
WORKDIR /src/backend/frontend
RUN corepack enable && corepack prepare pnpm@10.17.1 --activate
COPY backend/frontend/package.json backend/frontend/pnpm-lock.yaml backend/frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY backend/frontend/ ./
RUN pnpm run build

FROM golang:1.24-bookworm AS backend
WORKDIR /src/backend
RUN apt-get update && apt-get install -y --no-install-recommends build-essential ca-certificates && rm -rf /var/lib/apt/lists/*
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/project-iiif . \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/s3-smoke ./cmd/s3-smoke \
    && CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/migrate-local-to-db ./cmd/migrate-local-to-mysql

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tini && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --create-home --home-dir /app project-iiif
WORKDIR /app
COPY --from=backend /out/project-iiif /usr/local/bin/project-iiif
COPY --from=backend /out/s3-smoke /usr/local/bin/s3-smoke
COPY --from=backend /out/migrate-local-to-db /usr/local/bin/migrate-local-to-db
COPY --from=frontend /src/backend/frontend/dist ./frontend/dist
COPY backend/migrations ./migrations
RUN mkdir -p /app/data/temp && chown -R project-iiif:project-iiif /app
USER project-iiif
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=12 CMD curl -fsS http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/usr/local/bin/project-iiif"]
