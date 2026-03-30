#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="${COMPOSE_PROJECT_NAME:-yggdrasil-dev}"

args=(
  --project-name "$PROJECT_NAME"
  --env-file "$ROOT/.env"
  -f "$ROOT/dev/compose/infra.yml"
  -f "$ROOT/services/yggdrasil-core/docker-compose.yml"
)

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ -f "$file" ]] || continue
  args+=(-f "$file")
done < <("$ROOT/scripts/ygg.sh" surfaces installed)

while IFS= read -r file; do
  args+=(-f "$file")
done < <(find "$ROOT/integrations" -mindepth 2 -maxdepth 2 -name 'docker-compose.yml' -type f | sort)

exec docker compose "${args[@]}" "$@"
