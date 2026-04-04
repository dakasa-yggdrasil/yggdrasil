#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_TMP_ROOT="$(dirname "$ROOT")/.yggdrasil-tmp"
mkdir -p "$SHARED_TMP_ROOT"
WORKDIR="$(mktemp -d "$SHARED_TMP_ROOT/ygg-install-smoke.XXXXXX")"
REPO_DIR="$WORKDIR/repo"
PROJECT_NAME="ygg-install-smoke-$(date +%s)"

cleanup() {
  if [ -d "$REPO_DIR/.git" ]; then
    COMPOSE_PROJECT_NAME="$PROJECT_NAME" "$REPO_DIR/scripts/docker-compose.sh" down --remove-orphans >/dev/null 2>&1 || true
  fi
  rm -rf "$WORKDIR"
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

echo "Preparing isolated workspace at $REPO_DIR"
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
CONSOLE_PORT=13080

POSSIBLE_ORGS=dakasa-yggdrasil,DaKasa-Co
EOF

cp "$REPO_DIR/.env" "$REPO_DIR/dev/compose/.env"

unset YGGDRASIL_SURFACES_DEV_DIR
unset YGGDRASIL_INTEGRATIONS_DEV_DIR

cd "$REPO_DIR"

echo "Installing reference surfaces from remote repositories..."
./scripts/ygg.sh surfaces install yggdrasil-auth-surface >/dev/null
./scripts/ygg.sh surfaces install yggdrasil-console >/dev/null

echo "Installing catalog integrations from remote repositories..."
./scripts/ygg.sh integrations install aws >/dev/null
./scripts/ygg.sh integrations install github >/dev/null

task env:init >/dev/null

aws_url="$(git config -f .gitmodules --get submodule.integrations/integration-aws.url)"
github_url="$(git config -f .gitmodules --get submodule.integrations/integration-github.url)"
auth_url="$(git config -f .gitmodules --get submodule.surfaces/yggdrasil-auth-surface.url)"
console_url="$(git config -f .gitmodules --get submodule.surfaces/yggdrasil-console.url)"

test "$aws_url" = "https://github.com/dakasa-yggdrasil/integration-aws.git"
test "$github_url" = "https://github.com/dakasa-yggdrasil/integration-github.git"
test "$auth_url" = "https://github.com/dakasa-yggdrasil/surface-auth.git"
test "$console_url" = "https://github.com/dakasa-yggdrasil/surface-console.git"

./scripts/ygg.sh surfaces installed | grep -q 'surfaces/yggdrasil-auth-surface/docker-compose.yml'
./scripts/ygg.sh surfaces installed | grep -q 'surfaces/yggdrasil-console/docker-compose.yml'
./scripts/ygg.sh integrations installed | grep -q 'integrations/integration-aws/docker-compose.yml'
./scripts/ygg.sh integrations installed | grep -q 'integrations/integration-github/docker-compose.yml'

echo "Validating compose in isolated workspace..."
COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh config >/dev/null

echo "Removing catalog integrations after install validation..."
./scripts/ygg.sh integrations remove aws >/dev/null
./scripts/ygg.sh integrations remove github >/dev/null

if ./scripts/ygg.sh integrations installed | grep -q 'integrations/integration-aws/docker-compose.yml'; then
  echo "integration-aws should have been removed from the isolated workspace" >&2
  exit 1
fi
if ./scripts/ygg.sh integrations installed | grep -q 'integrations/integration-github/docker-compose.yml'; then
  echo "integration-github should have been removed from the isolated workspace" >&2
  exit 1
fi

echo "Bringing isolated stack up..."
COMPOSE_PROJECT_NAME="$PROJECT_NAME" ./scripts/docker-compose.sh up -d >/dev/null

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

echo "Installation manager smoke passed."
