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
  args+=(-f "$file")
done < <(
  if [[ -f "$ROOT/catalog/surfaces.active" ]]; then
    while IFS= read -r surface; do
      surface="$(printf '%s' "$surface" | xargs)"
      [[ -z "$surface" ]] && continue
      [[ "$surface" == \#* ]] && continue
      file="$ROOT/surfaces/$surface/docker-compose.yml"
      [[ -f "$file" ]] && printf '%s\n' "$file"
    done < "$ROOT/catalog/surfaces.active"
  else
    find "$ROOT/surfaces" -mindepth 2 -maxdepth 2 -name 'docker-compose.yml' -type f | sort
  fi
)

while IFS= read -r file; do
  args+=(-f "$file")
done < <(find "$ROOT/integrations" -mindepth 2 -maxdepth 2 -name 'docker-compose.yml' -type f | sort)

exec docker compose "${args[@]}" "$@"
