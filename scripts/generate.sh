#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
make generate
cp "$ROOT/openapi/openapi.yaml" "$ROOT/services/backend/internal/spec/openapi.yaml"
