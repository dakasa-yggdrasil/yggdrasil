#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PARENT="$(dirname "$ROOT")"
cd "$ROOT"

if command -v go >/dev/null 2>&1; then
  exec env GOWORK=off go run ./cmd/yggdrasil "$@"
fi

exec docker run --rm -i \
  -v "$ROOT:/workspace" \
  -v "$PARENT:$PARENT" \
  -w "$ROOT" \
  golang:1.25-bookworm \
  env GOWORK=off /usr/local/go/bin/go run ./cmd/yggdrasil "$@"
