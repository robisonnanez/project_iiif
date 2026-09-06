#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-1.0.0-dev}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse HEAD)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
OUTPUT_DIR="${1:-$ROOT_DIR/release/project-iiif-$VERSION-$COMMIT}"

run_pnpm() {
  if command -v pnpm >/dev/null 2>&1; then
    pnpm "$@"
  else
    corepack pnpm "$@"
  fi
}

if [[ -e "$OUTPUT_DIR" ]]; then
  echo "El directorio de salida ya existe: $OUTPUT_DIR" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
cd "$ROOT_DIR/backend"
go test ./...
go vet ./...
go run ./cmd/check-comments
go build -trimpath -ldflags="-s -w -X 'iiif-pdf-server/internal/buildinfo.Version=$VERSION' -X 'iiif-pdf-server/internal/buildinfo.Commit=$COMMIT' -X 'iiif-pdf-server/internal/buildinfo.BuildDate=$BUILD_DATE'" -o "$OUTPUT_DIR/iiif-server" .

cd "$ROOT_DIR/backend/frontend"
run_pnpm install --frozen-lockfile
run_pnpm test
run_pnpm run build
printf '{"version":"%s","commit":"%s","build_date":"%s"}\n' "$VERSION" "$COMMIT" "$BUILD_DATE" > dist/build-meta.json
tar -czf "$OUTPUT_DIR/frontend-dist.tgz" dist

cp "$ROOT_DIR/backend/docs/swagger.json" "$OUTPUT_DIR/openapi.json"
printf '{"version":"%s","commit":"%s","build_date":"%s"}\n' "$VERSION" "$COMMIT" "$BUILD_DATE" > "$OUTPUT_DIR/build-meta.json"
cd "$OUTPUT_DIR"
sha256sum iiif-server frontend-dist.tgz openapi.json build-meta.json > SHA256SUMS

if [[ -n "${SIGNING_KEY:-}" ]]; then
  gpg --batch --yes --local-user "$SIGNING_KEY" --armor --detach-sign SHA256SUMS
fi

echo "Artefactos generados en $OUTPUT_DIR"
