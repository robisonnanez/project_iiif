#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
swag init -g main.go -o docs
echo "Swagger generado en backend/docs"
