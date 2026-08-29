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

add_compose_file() {
  local file="$1"
  [[ -f "$file" ]] || return
  args+=(-f "$file")

  local repo_name prefix
  repo_name="$(basename "$(dirname "$file")")"
  prefix="$(printf '%s' "$repo_name" | tr '[:lower:]-' '[:upper:]_')"
  export "${prefix}_BUILD_CONTEXT=$ROOT/$(dirname "${file#"$ROOT/"}")"
  export "${prefix}_DOCKERFILE=Dockerfile"
}

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  add_compose_file "$file"
done < <("$ROOT/scripts/yggdrasil.sh" surfaces installed)

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  add_compose_file "$file"
done < <(find "$ROOT/integrations" -mindepth 2 -maxdepth 2 -name 'docker-compose.yml' -type f | sort)

exec docker compose "${args[@]}" "$@"
