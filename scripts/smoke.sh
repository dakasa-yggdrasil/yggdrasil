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
mock_provider_pid=""
mock_provider_script=""
trap 'rm -f "${cookie_jar}" "${mock_provider_script}"; if [[ -n "${mock_provider_pid}" ]]; then kill "${mock_provider_pid}" >/dev/null 2>&1 || true; fi' EXIT

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

echo "Rotating managed secret ${secret_name}..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/secrets/global/${secret_name}/rotate" \
  -d '{"data":{"token":"smoke-token-rotated","endpoint":"https://example.internal"}}' >/dev/null

echo "Disabling managed secret ${secret_name}..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/secrets/global/${secret_name}/disable" \
  -d '{"metadata":{"reason":"smoke-disable"}}' >/dev/null

echo "Revoking managed secret ${secret_name}..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/secrets/global/${secret_name}/revoke" \
  -d '{"metadata":{"reason":"smoke-revoke"}}' >/dev/null

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

external_subject="github-smoke-${slug_suffix}"
external_login="smoke-gh-${slug_suffix}"

echo "Logging in through auth surface with external identity..."
third_party_login_response="$(curl -fsS \
  -c "${cookie_jar}" \
  -X POST \
  -H "Content-Type: application/json" \
  "${auth_url}/api/v1/auth/third-party/login" \
  -d "$(cat <<JSON
{"provider":"github","subject":"${external_subject}","login":"${external_login}","email":"${collaborator_email}","display_name":"Smoke GitHub User","auto_link_by_email":true,"claims":{"organization":"dakasa-yggdrasil"}}
JSON
)")"

third_party_token="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' <<<"${third_party_login_response}")"
if [[ -z "${third_party_token}" ]]; then
  echo "third-party login did not return a session token" >&2
  exit 1
fi

curl -fsS "${core_url}/api/v1/auth/third-party-identities?collaborator_id=${collaborator_id}" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert any(item["provider"] == "github" and item["subject"].startswith("github-smoke-") for item in payload["identities"])
'

echo "Resolving external-auth session through auth surface..."
third_party_session_response="$(curl -fsS -b "${cookie_jar}" "${auth_url}/api/v1/auth/session")"

python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert payload["authenticated"] is True
assert payload["collaborator"]["primary_email"].startswith("smoke-")
assert payload["session"]["metadata"]["auth_method"] == "third_party"
assert payload["session"]["metadata"]["provider"] == "github"
' <<<"${third_party_session_response}"

provider_secret_name="oidc-client-secret-${slug_suffix}"
provider_name="oidc-smoke-${slug_suffix}"
mock_provider_port=9494
mock_provider_base_url="http://127.0.0.1:${mock_provider_port}"
mock_provider_container_base_url="http://host.docker.internal:${mock_provider_port}"

echo "Creating managed secret ${provider_secret_name} for OIDC provider..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/secrets" \
  -d "$(cat <<JSON
{"namespace":"global","name":"${provider_secret_name}","data":{"value":"smoke-oidc-client-secret"}}
JSON
)" >/dev/null

echo "Registering OIDC provider ${provider_name}..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/auth/providers" \
  -d "$(cat <<JSON
{
  "name": "${provider_name}",
  "type": "oidc",
  "display_name": "Smoke OIDC",
  "authorize_url": "${mock_provider_base_url}/authorize",
  "token_url": "${mock_provider_container_base_url}/token",
  "userinfo_url": "${mock_provider_container_base_url}/userinfo",
  "client_id": "smoke-client-id",
  "client_secret_ref": "secret://global/${provider_secret_name}",
  "auto_link_by_email": true,
  "metadata": {"smoke_test": true}
}
JSON
)" >/dev/null

curl -fsS "${core_url}/api/v1/auth/providers/${provider_name}" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert payload["provider"]["name"].startswith("oidc-smoke-")
assert payload["provider"]["type"] == "oidc"
'

mock_provider_script="$(mktemp)"
cat > "${mock_provider_script}" <<PY
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlencode, urlparse
import json

SUBJECT = "oidc-subject-${slug_suffix}"
LOGIN = "smoke-oidc-${slug_suffix}"
EMAIL = "${collaborator_email}"
NAME = "Smoke OIDC User"
AVATAR = "https://example.com/avatar.png"
PROFILE = "https://example.com/profile"

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args, **kwargs):
        return

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == "/.well-known/openid-configuration":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({
                "authorization_endpoint": "${mock_provider_base_url}/authorize",
                "token_endpoint": "${mock_provider_container_base_url}/token",
                "userinfo_endpoint": "${mock_provider_container_base_url}/userinfo"
            }).encode())
            return

        if parsed.path == "/authorize":
            query = parse_qs(parsed.query)
            redirect_uri = query["redirect_uri"][0]
            state = query["state"][0]
            location = redirect_uri + ("&" if "?" in redirect_uri else "?") + urlencode({
                "code": "smoke-auth-code",
                "state": state,
            })
            self.send_response(302)
            self.send_header("Location", location)
            self.end_headers()
            return

        if parsed.path == "/userinfo":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({
                "sub": SUBJECT,
                "preferred_username": LOGIN,
                "email": EMAIL,
                "name": NAME,
                "picture": AVATAR,
                "profile": PROFILE
            }).encode())
            return

        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        parsed = urlparse(self.path)
        if parsed.path == "/token":
            _ = self.rfile.read(int(self.headers.get("Content-Length", "0") or "0"))
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({
                "access_token": "smoke-access-token",
                "token_type": "Bearer"
            }).encode())
            return

        self.send_response(404)
        self.end_headers()

HTTPServer(("127.0.0.1", ${mock_provider_port}), Handler).serve_forever()
PY

python3 "${mock_provider_script}" >/dev/null 2>&1 &
mock_provider_pid=$!
sleep 1

echo "Completing browser OIDC flow through auth surface..."
oidc_session_response="$(curl -fsS -L \
  -c "${cookie_jar}" \
  -b "${cookie_jar}" \
  --get \
  --data-urlencode "redirect_to=${auth_url}/api/v1/auth/session" \
  "${auth_url}/api/v1/auth/third-party/start/${provider_name}")"

python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert payload["authenticated"] is True
assert payload["collaborator"]["primary_email"].startswith("smoke-")
assert payload["session"]["metadata"]["auth_method"] == "third_party"
assert payload["session"]["metadata"]["provider"].startswith("oidc-smoke-")
' <<<"${oidc_session_response}"

curl -fsS "${core_url}/api/v1/auth/third-party-identities?provider=${provider_name}" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
assert len(payload["identities"]) == 1
assert payload["identities"][0]["email"].startswith("smoke-")
'

echo "Smoke checks passed."
