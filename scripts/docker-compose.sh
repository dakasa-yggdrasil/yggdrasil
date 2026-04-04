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

integration_files=()

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ -f "$file" ]] || continue
  args+=(-f "$file")
  integration_files+=("$file")
done < <("$ROOT/scripts/ygg.sh" surfaces installed)

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ -f "$file" ]] || continue
  args+=(-f "$file")
  integration_files+=("$file")
done < <(find "$ROOT/integrations" -mindepth 2 -maxdepth 2 -name 'docker-compose.yml' -type f | sort)

for file in "${integration_files[@]}"; do
  repo_name="$(basename "$(dirname "$file")")"
  prefix="$(printf '%s' "$repo_name" | tr '[:lower:]-' '[:upper:]_')"
  export "${prefix}_BUILD_CONTEXT=$ROOT/integrations/${repo_name}"
  export "${prefix}_DOCKERFILE=Dockerfile"
done

exec docker compose "${args[@]}" "$@"
