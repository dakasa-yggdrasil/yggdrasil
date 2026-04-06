#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "${ROOT_DIR}/.env" ]]; then
  # shellcheck disable=SC1091
  source "${ROOT_DIR}/.env"
fi

CORE_HTTP_PORT="${CORE_HTTP_PORT:-9080}"
YGGDRASIL_CORE_BASE_URL="${YGGDRASIL_CORE_BASE_URL:-http://127.0.0.1:${CORE_HTTP_PORT}}"
HEIMDALL_NAMESPACE="${HEIMDALL_NAMESPACE:-global}"
HEIMDALL_INSTANCE_NAME="${HEIMDALL_INSTANCE_NAME:-heimdall-guardian}"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "$1 is required" >&2
    exit 1
  }
}

require_tool curl
require_tool python3

instance_file="$(mktemp)"
gpt_secret_file="$(mktemp)"
claude_secret_file="$(mktemp)"
trap 'rm -f "${instance_file}" "${gpt_secret_file}" "${claude_secret_file}"' EXIT

curl -fsS \
  "${YGGDRASIL_CORE_BASE_URL}/api/v1/integration-instances?namespace=${HEIMDALL_NAMESPACE}&name=${HEIMDALL_INSTANCE_NAME}" \
  >"${instance_file}"

readarray -t parsed < <(python3 - "${instance_file}" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)

manifests = payload.get("manifests", [])
if not manifests:
    raise SystemExit("heimdall instance not found")

manifest = manifests[0]
config = manifest["spec"]["config"]
if config.get("llm_enabled") is not True:
    raise SystemExit("heimdall llm_enabled is not true")

gpt_ref = str(config.get("llm_gpt_api_key", "")).strip()
claude_ref = str(config.get("llm_claude_api_key", "")).strip()
if not gpt_ref.startswith("secret://"):
    raise SystemExit("llm_gpt_api_key is not a secret:// ref")
if not claude_ref.startswith("secret://"):
    raise SystemExit("llm_claude_api_key is not a secret:// ref")

def parse_secret_ref(ref: str):
    target = ref.removeprefix("secret://")
    path, _, key = target.partition("#")
    namespace, _, name = path.partition("/")
    if not namespace or not name:
        raise SystemExit(f"invalid secret ref: {ref}")
    return namespace, name, key or "value"

gpt_ns, gpt_name, _ = parse_secret_ref(gpt_ref)
claude_ns, claude_name, _ = parse_secret_ref(claude_ref)

print(manifest["metadata"]["namespace"])
print(manifest["metadata"]["name"])
print(config.get("llm_provider_order", ""))
print(config.get("llm_gpt_model", ""))
print(config.get("llm_claude_model", ""))
print(gpt_ref)
print(claude_ref)
print(gpt_ns)
print(gpt_name)
print(claude_ns)
print(claude_name)
PY
)

instance_ns="${parsed[0]}"
instance_name="${parsed[1]}"
provider_order="${parsed[2]}"
gpt_model="${parsed[3]}"
claude_model="${parsed[4]}"
gpt_ref="${parsed[5]}"
claude_ref="${parsed[6]}"
gpt_secret_ns="${parsed[7]}"
gpt_secret_name="${parsed[8]}"
claude_secret_ns="${parsed[9]}"
claude_secret_name="${parsed[10]}"

curl -fsS "${YGGDRASIL_CORE_BASE_URL}/api/v1/secrets/${gpt_secret_ns}/${gpt_secret_name}" >"${gpt_secret_file}"
curl -fsS "${YGGDRASIL_CORE_BASE_URL}/api/v1/secrets/${claude_secret_ns}/${claude_secret_name}" >"${claude_secret_file}"

python3 - "${gpt_secret_file}" "${claude_secret_file}" <<'PY'
import json
import sys

for path in sys.argv[1:]:
    with open(path, "r", encoding="utf-8") as handle:
        payload = json.load(handle)
    if payload.get("status") == "error":
        raise SystemExit(f"managed secret lookup failed: {path}")
    if payload.get("version", 0) < 1:
        raise SystemExit(f"managed secret version invalid: {path}")
    keys = payload.get("keys", [])
    if "value" not in keys:
        raise SystemExit(f"managed secret missing value key: {path}")
PY

echo "Heimdall LLM verification passed."
echo "instance: ${instance_ns}/${instance_name}"
echo "provider order: ${provider_order}"
echo "gpt model: ${gpt_model}"
echo "claude model: ${claude_model}"
echo "gpt secret ref: ${gpt_ref}"
echo "claude secret ref: ${claude_ref}"
