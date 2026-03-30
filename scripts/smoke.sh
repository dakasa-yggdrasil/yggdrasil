#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "${ROOT_DIR}/.env" ]]; then
  # shellcheck disable=SC1091
  source "${ROOT_DIR}/.env"
fi

CORE_HTTP_PORT="${CORE_HTTP_PORT:-9080}"
AUTH_SURFACE_PORT="${AUTH_SURFACE_PORT:-9090}"
CONSOLE_PORT="${CONSOLE_PORT:-3080}"

core_url="http://127.0.0.1:${CORE_HTTP_PORT}"
auth_url="http://127.0.0.1:${AUTH_SURFACE_PORT}"
console_url="http://127.0.0.1:${CONSOLE_PORT}"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "$1 is required" >&2
    exit 1
  }
}

require_tool curl
require_tool python3

cookie_jar="$(mktemp)"
trap 'rm -f "${cookie_jar}"' EXIT

wait_for_url() {
  local url="$1"
  local attempts="${2:-30}"
  local sleep_seconds="${3:-2}"

  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS "${url}" >/dev/null; then
      return 0
    fi
    sleep "${sleep_seconds}"
  done

  echo "timed out waiting for ${url}" >&2
  return 1
}

echo "Checking service health endpoints..."
wait_for_url "${core_url}/healthz"
wait_for_url "${auth_url}/healthz"
wait_for_url "${console_url}/"

secret_name="smoke-$(date +%s)"
echo "Creating managed secret ${secret_name}..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/secrets" \
  -d "$(cat <<JSON
{"namespace":"global","name":"${secret_name}","data":{"token":"smoke-token","endpoint":"https://example.internal"}}
JSON
)" >/dev/null

curl -fsS "${core_url}/api/v1/secrets/global/${secret_name}" >/dev/null

slug_suffix="$(date +%s)"
collaborator_slug="col:smoke-${slug_suffix}"
collaborator_email="smoke-${slug_suffix}@example.com"
password="smoke-password-123"
surface_name="smoke-surface-${slug_suffix}"

echo "Registering discovered candidate ${surface_name}..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/catalog/discovery/register" \
  -d "$(cat <<JSON
{
  "registration": {
    "manifest": {
      "apiVersion": "yggdrasil.io/v1alpha1",
      "kind": "surface",
      "metadata": {
        "name": "${surface_name}",
        "namespace": "global",
        "description": "Smoke-registered surface candidate.",
        "labels": {
          "yggdrasil.io/surface-category": "api",
          "yggdrasil.io/smoke-test": "true"
        }
      },
      "spec": {
        "category": "api",
        "owners": ["team:platform"],
        "replaces": ["${surface_name}"],
        "integration_binding": "core_only",
        "runtime": {
          "kind": "http_api",
          "exposure": "internal",
          "port": 9191,
          "base_path": "/",
          "health_path": "/healthz"
        },
        "core_contracts": ["authorization", "surface"],
        "capabilities": [
          {
            "name": "root",
            "kind": "endpoint",
            "audience": "internal",
            "path": "/",
            "methods": ["GET"]
          }
        ]
      }
    }
  }
}
JSON
)" >/dev/null

curl -fsS "${core_url}/api/v1/surfaces?namespace=global" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert any(item["metadata"]["name"].startswith("smoke-surface-") for item in payload["manifests"])
'

echo "Creating collaborator ${collaborator_slug}..."
collaborator_response="$(curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/collaborators" \
  -d "$(cat <<JSON
{"slug":"${collaborator_slug}","display_name":"Smoke User","primary_email":"${collaborator_email}"}
JSON
)")"

collaborator_id="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["collaborator"]["id"])' <<<"${collaborator_response}")"

echo "Setting password through core auth..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/auth/passwords" \
  -d "$(cat <<JSON
{"collaborator_id":"${collaborator_id}","password":"${password}"}
JSON
)" >/dev/null

echo "Logging in through auth surface..."
login_response="$(curl -fsS \
  -c "${cookie_jar}" \
  -X POST \
  -H "Content-Type: application/json" \
  "${auth_url}/api/v1/auth/login" \
  -d "$(cat <<JSON
{"identifier":"${collaborator_email}","password":"${password}"}
JSON
)")"

session_token="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' <<<"${login_response}")"
if [[ -z "${session_token}" ]]; then
  echo "login did not return a session token" >&2
  exit 1
fi

echo "Resolving session through auth surface..."
session_response="$(curl -fsS -b "${cookie_jar}" "${auth_url}/api/v1/auth/session")"

python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert payload["authenticated"] is True
assert payload["collaborator"]["primary_email"].startswith("smoke-")
' <<<"${session_response}"

echo "Logging out through auth surface..."
logout_response="$(curl -fsS -b "${cookie_jar}" -X POST "${auth_url}/api/v1/auth/logout")"

python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert payload["authenticated"] is False
' <<<"${logout_response}"

echo "Smoke checks passed."
