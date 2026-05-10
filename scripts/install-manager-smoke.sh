#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_TMP_ROOT="$(dirname "$ROOT")/.yggdrasil-tmp"
mkdir -p "$SHARED_TMP_ROOT"
WORKDIR="$(mktemp -d "$SHARED_TMP_ROOT/ygg-install-smoke.XXXXXX")"
REPO_DIR="$WORKDIR/repo"
PROJECT_NAME="ygg-install-smoke-$(date +%s)"
RUNTIME_SMOKE_SERVICES=(
  "integration-aws"
  "integration-github"
  "integration-rabbitmq"
  "integration-grafana"
)

cleanup_stale_install_smoke_projects() {
  if ! command -v docker >/dev/null 2>&1; then
    return
  fi

  local containers networks volumes
  containers="$(docker ps -aq --filter name=ygg-install-smoke- || true)"
  networks="$(docker network ls -q --filter name=ygg-install-smoke- || true)"
  volumes="$(docker volume ls -q --filter name=ygg-install-smoke- || true)"

  if [ -n "$containers" ]; then
    docker rm -f $containers >/dev/null 2>&1 || true
  fi
  if [ -n "$networks" ]; then
    docker network rm $networks >/dev/null 2>&1 || true
  fi
  if [ -n "$volumes" ]; then
    docker volume rm -f $volumes >/dev/null 2>&1 || true
  fi
}

cleanup() {
  if [ -d "$REPO_DIR/.git" ]; then
    COMPOSE_PROJECT_NAME="$PROJECT_NAME" "$REPO_DIR/scripts/docker-compose.sh" down --remove-orphans >/dev/null 2>&1 || true
  fi
  rm -rf "$WORKDIR" >/dev/null 2>&1 && return 0
  if command -v docker >/dev/null 2>&1 && [ -d "$WORKDIR" ]; then
    docker run --rm -v "$WORKDIR":/workspace postgres:17-alpine sh -lc \
      'rm -rf /workspace/* /workspace/.[!.]* /workspace/..?* 2>/dev/null || true' >/dev/null 2>&1 || true
  fi
  rm -rf "$WORKDIR" >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_for_url() {
  local url="$1"
  local attempts="${2:-30}"
  local sleep_seconds="${3:-2}"

  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then
      return 0
    fi
    sleep "$sleep_seconds"
  done

  return 1
}

dump_compose_logs() {
  if [ -d "$REPO_DIR/.git" ]; then
    COMPOSE_PROJECT_NAME="$PROJECT_NAME" "$REPO_DIR/scripts/docker-compose.sh" logs --no-color || true
  fi
}

wait_for_running_service() {
  local service="$1"
  local attempts="${2:-60}"
  local sleep_seconds="${3:-2}"

  for ((i = 1; i <= attempts; i++)); do
    if COMPOSE_PROJECT_NAME="$PROJECT_NAME" "$REPO_DIR/scripts/docker-compose.sh" ps --services --status running | grep -qx "$service"; then
      return 0
    fi
    sleep "$sleep_seconds"
  done

  return 1
}

build_service_image() {
  local service="$1"

  echo "Building ${service} image in isolated workspace..."
  COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh build "$service"
}

warm_service_runtime() {
  local service="$1"
  local command="$2"

  echo "Warming ${service} runtime dependencies in isolated workspace..."
  COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh run --rm --no-deps "$service" sh -lc "$command"
}

run_isolated_product_smoke() {
  local attempts="${1:-2}"

  for ((i = 1; i <= attempts; i++)); do
    if ./scripts/smoke.sh; then
      return 0
    fi

    if (( i < attempts )); then
      echo "isolated end-to-end smoke failed on attempt ${i}; waiting and retrying..."
      wait_for_url "http://127.0.0.1:19080/readyz" 30 2 || true
      wait_for_url "http://127.0.0.1:19090/readyz" 30 2 || true
      wait_for_url "http://127.0.0.1:13080/healthz" 30 2 || true
      sleep 5
    fi
  done

  return 1
}

echo "Preparing isolated workspace at $REPO_DIR"
cleanup_stale_install_smoke_projects
git clone "$ROOT" "$REPO_DIR" >/dev/null 2>&1

# Overlay the current working tree so the smoke run validates local changes,
# not only the last committed state.
(
  cd "$ROOT"
  tar \
    --exclude=.git \
    --exclude=surfaces/yggdrasil-auth-surface \
    --exclude=surfaces/yggdrasil-console \
    --exclude=surfaces/yggdrasil-console/node_modules \
    --exclude=surfaces/yggdrasil-console/dist \
    -cf - .
) | (
  cd "$REPO_DIR"
  tar -xf -
)

find "$REPO_DIR" -name '.env' -type f -delete

cat > "$REPO_DIR/.env" <<EOF
COMPOSE_PROJECT_NAME=$PROJECT_NAME

POSTGRES_IMAGE=postgres:17-alpine
POSTGRES_USER=postgres
POSTGRES_PASSWORD=someAwesomePassword
POSTGRES_DB=yggdrasil
POSTGRES_CORE_DB=yggdrasil_core
POSTGRES_HOST_PORT=15432

RABBITMQ_IMAGE=rabbitmq:3.13-management
RABBITMQ_HOST_PORT=25672
RABBITMQ_MANAGEMENT_PORT=25673
BROKER_URL=amqp://guest:guest@yggdrasil-rabbitmq:5672/

GO_IMAGE=golang:1.25-bookworm
NODE_IMAGE=node:22-bookworm

CORE_HTTP_PORT=19080
AUTH_SURFACE_PORT=19090
YGGDRASIL_AUTH_SURFACE_PUBLIC_URL=http://127.0.0.1:19090
CONSOLE_PORT=13080

POSSIBLE_ORGS=dakasa-yggdrasil,DaKasa-Co
EOF

cp "$REPO_DIR/.env" "$REPO_DIR/dev/compose/.env"

unset YGGDRASIL_SURFACES_DEV_DIR
unset YGGDRASIL_INTEGRATIONS_DEV_DIR

cd "$REPO_DIR"

catalog_integrations=()
while IFS= read -r entry; do
  catalog_integrations+=("$entry")
done < <(python3 - <<'PY'
import json

with open("catalog/integrations.json", "r", encoding="utf-8") as handle:
    data = json.load(handle)

for integration in data.get("integrations", []):
    print(
        "|".join(
            [
                integration["slug"],
                integration["repo_name"],
                integration["repo_url"],
            ]
        )
    )
PY
)

echo "Installing reference surfaces from remote repositories..."
./scripts/yggdrasil.sh surfaces install yggdrasil-auth-surface >/dev/null
./scripts/yggdrasil.sh surfaces install yggdrasil-console >/dev/null

echo "Installing all catalog integrations from remote repositories..."
for entry in "${catalog_integrations[@]}"; do
  IFS='|' read -r slug _ _ <<<"$entry"
  ./scripts/yggdrasil.sh integrations install "$slug" >/dev/null
done

task env:init >/dev/null

auth_url="$(git config -f .gitmodules --get submodule.surfaces/yggdrasil-auth-surface.url)"
console_url="$(git config -f .gitmodules --get submodule.surfaces/yggdrasil-console.url)"

test "$auth_url" = "https://github.com/dakasa-yggdrasil/surface-auth.git"
test "$console_url" = "https://github.com/dakasa-yggdrasil/surface-console.git"

for entry in "${catalog_integrations[@]}"; do
  IFS='|' read -r _ repo_name repo_url <<<"$entry"
  actual_url="$(git config -f .gitmodules --get "submodule.integrations/${repo_name}.url")"
  test "$actual_url" = "$repo_url"
done

./scripts/ygg.sh surfaces installed | grep -q 'surfaces/yggdrasil-auth-surface/docker-compose.yml'
./scripts/ygg.sh surfaces installed | grep -q 'surfaces/yggdrasil-console/docker-compose.yml'

for entry in "${catalog_integrations[@]}"; do
  IFS='|' read -r _ repo_name _ <<<"$entry"
  ./scripts/ygg.sh integrations installed | grep -q "integrations/${repo_name}/docker-compose.yml"
done

echo "Validating compose in isolated workspace..."
COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh config >/dev/null

for service_and_command in \
  "yggdrasil-core:/usr/local/go/bin/go mod download && /usr/local/go/bin/go build ./scripts/goose && /usr/local/go/bin/go build ." \
  "yggdrasil-auth-surface:/usr/local/go/bin/go mod download && /usr/local/go/bin/go build ." \
  "yggdrasil-console:npm ci"; do
  service="${service_and_command%%:*}"
  command="${service_and_command#*:}"
  if ! warm_service_runtime "$service" "$command"; then
    echo "failed to warm ${service} runtime dependencies in the isolated workspace" >&2
    exit 1
  fi
done

echo "Starting shared infra in isolated workspace..."
if ! COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh up -d \
  yggdrasil-postgres \
  yggdrasil-rabbitmq >/dev/null; then
  dump_compose_logs
  echo "isolated shared infra failed to start" >&2
  exit 1
fi

for service in yggdrasil-postgres yggdrasil-rabbitmq; do
  if ! wait_for_running_service "$service" 30 2; then
    dump_compose_logs
    echo "expected ${service} to be running before starting the rest of the stack" >&2
    exit 1
  fi
done

echo "Bringing isolated core and representative integrations up..."
for service in \
  yggdrasil-core \
  yggdrasil-auth-surface \
  yggdrasil-console \
  "${RUNTIME_SMOKE_SERVICES[@]}"; do
  if ! build_service_image "$service"; then
    dump_compose_logs
    echo "failed to build ${service} in the isolated workspace" >&2
    exit 1
  fi
done

if ! COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh up -d --no-build \
  yggdrasil-core \
  yggdrasil-auth-surface \
  yggdrasil-console \
  "${RUNTIME_SMOKE_SERVICES[@]}" >/dev/null; then
  dump_compose_logs
  echo "isolated core/runtime slice failed to start" >&2
  exit 1
fi

echo "Waiting for isolated core..."
if ! wait_for_url "http://127.0.0.1:19080/readyz" 120 2; then
  dump_compose_logs
  echo "isolated core did not become ready in time" >&2
  exit 1
fi

echo "Bootstrapping isolated core..."
if ! COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh exec yggdrasil-core sh -lc "/usr/local/go/bin/go run ./scripts/bootstrap --path ./docs/bootstrap/manifests" >/dev/null; then
  dump_compose_logs
  echo "isolated core bootstrap failed" >&2
  exit 1
fi

echo "Checking isolated runtime..."
if ! wait_for_url "http://127.0.0.1:19080/healthz" 15 2; then
  dump_compose_logs
  echo "isolated core health check failed" >&2
  exit 1
fi
if ! wait_for_url "http://127.0.0.1:19090/readyz" 90 2; then
  dump_compose_logs
  echo "isolated auth surface did not become ready in time" >&2
  exit 1
fi
if ! wait_for_url "http://127.0.0.1:13080/healthz" 120 2; then
  dump_compose_logs
  echo "isolated console did not become healthy in time" >&2
  exit 1
fi

for service in "${RUNTIME_SMOKE_SERVICES[@]}"; do
  if ! wait_for_running_service "$service" 90 2; then
    dump_compose_logs
    echo "expected ${service} to be running in the isolated workspace" >&2
    exit 1
  fi
done

echo "Running isolated end-to-end product smoke..."
if ! run_isolated_product_smoke 2; then
  dump_compose_logs
  echo "isolated end-to-end smoke failed" >&2
  exit 1
fi

echo "Installation manager smoke passed."
