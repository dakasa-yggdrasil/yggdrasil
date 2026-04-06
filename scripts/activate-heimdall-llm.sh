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
HEIMDALL_GUARDIAN_NAME="${HEIMDALL_GUARDIAN_NAME:-Heimdall}"
HEIMDALL_PROVIDER_ORDER="${HEIMDALL_PROVIDER_ORDER:-gpt,claude}"
HEIMDALL_LLM_MODE="${HEIMDALL_LLM_MODE:-fallback}"
HEIMDALL_GPT_BASE_URL="${HEIMDALL_GPT_BASE_URL:-https://api.openai.com/v1}"
HEIMDALL_GPT_MODEL="${HEIMDALL_GPT_MODEL:-gpt-5.4-mini-2026-03-17}"
HEIMDALL_CLAUDE_BASE_URL="${HEIMDALL_CLAUDE_BASE_URL:-https://api.anthropic.com/v1}"
HEIMDALL_CLAUDE_MODEL="${HEIMDALL_CLAUDE_MODEL:-claude-sonnet-4-20250514}"
HEIMDALL_CLAUDE_VERSION="${HEIMDALL_CLAUDE_VERSION:-2023-06-01}"
HEIMDALL_LLM_TEMPERATURE="${HEIMDALL_LLM_TEMPERATURE:-0.1}"
HEIMDALL_LLM_TIMEOUT_SECONDS="${HEIMDALL_LLM_TIMEOUT_SECONDS:-30}"
HEIMDALL_RECOMMENDATION_LIMIT="${HEIMDALL_RECOMMENDATION_LIMIT:-10}"
HEIMDALL_MONTHLY_COST_BUDGET_USD="${HEIMDALL_MONTHLY_COST_BUDGET_USD:-1500}"
HEIMDALL_UTILIZATION_FLOOR="${HEIMDALL_UTILIZATION_FLOOR:-0.35}"
HEIMDALL_IDLE_ENVIRONMENT_HOURS="${HEIMDALL_IDLE_ENVIRONMENT_HOURS:-24}"
HEIMDALL_MAX_RIGHTSIZE_CHANGE_PERCENT="${HEIMDALL_MAX_RIGHTSIZE_CHANGE_PERCENT:-25}"
HEIMDALL_DEFAULT_OWNER_TEAM="${HEIMDALL_DEFAULT_OWNER_TEAM:-team:platform}"
HEIMDALL_DEFAULT_REPOSITORY_OWNER="${HEIMDALL_DEFAULT_REPOSITORY_OWNER:-dakasa-yggdrasil}"
HEIMDALL_DEFAULT_DEPLOY_WORKFLOW="${HEIMDALL_DEFAULT_DEPLOY_WORKFLOW:-deploy.yml}"
HEIMDALL_CRITICAL_AUTO_REMEDIATION_ENABLED="${HEIMDALL_CRITICAL_AUTO_REMEDIATION_ENABLED:-true}"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "$1 is required" >&2
    exit 1
  }
}

require_var() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "${name} is required" >&2
    exit 1
  fi
}

require_tool curl
require_tool python3

require_var HEIMDALL_GPT_API_KEY
require_var HEIMDALL_CLAUDE_API_KEY

payload_file="$(mktemp)"
response_file="$(mktemp)"
trap 'rm -f "${payload_file}" "${response_file}"' EXIT

python3 >"${payload_file}" <<'PY'
import json
import os

payload = {
    "name": os.environ["HEIMDALL_INSTANCE_NAME"],
    "namespace": os.environ["HEIMDALL_NAMESPACE"],
    "description": "Heimdall guardian with bounded GPT and Claude fallback enabled.",
    "labels": {
        "bootstrap_source": "integration-heimdall",
        "provider": "heimdall",
        "llm_enabled": "true",
    },
    "type_ref": {
        "namespace": "global",
        "name": "heimdall",
    },
    "status": "active",
    "owners": [os.environ["HEIMDALL_DEFAULT_OWNER_TEAM"]],
    "config": {
        "guardian_name": os.environ["HEIMDALL_GUARDIAN_NAME"],
        "observation_scope": "global",
        "critical_auto_remediation_enabled": os.environ["HEIMDALL_CRITICAL_AUTO_REMEDIATION_ENABLED"].lower() == "true",
        "recommendation_limit": int(os.environ["HEIMDALL_RECOMMENDATION_LIMIT"]),
        "default_owner_team": os.environ["HEIMDALL_DEFAULT_OWNER_TEAM"],
        "default_repository_owner": os.environ["HEIMDALL_DEFAULT_REPOSITORY_OWNER"],
        "default_deploy_workflow": os.environ["HEIMDALL_DEFAULT_DEPLOY_WORKFLOW"],
        "queue_backlog_threshold": 1000,
        "error_rate_threshold": 0.2,
        "restart_count_threshold": 3,
        "sync_lag_threshold_seconds": 300,
        "secret_expiry_warning_hours": 72,
        "monthly_cost_budget_usd": float(os.environ["HEIMDALL_MONTHLY_COST_BUDGET_USD"]),
        "utilization_floor": float(os.environ["HEIMDALL_UTILIZATION_FLOOR"]),
        "idle_environment_hours": int(os.environ["HEIMDALL_IDLE_ENVIRONMENT_HOURS"]),
        "max_rightsize_change_percent": int(os.environ["HEIMDALL_MAX_RIGHTSIZE_CHANGE_PERCENT"]),
        "llm_enabled": True,
        "llm_mode": os.environ["HEIMDALL_LLM_MODE"],
        "llm_provider_order": os.environ["HEIMDALL_PROVIDER_ORDER"],
        "llm_base_url": os.environ["HEIMDALL_GPT_BASE_URL"],
        "llm_model": os.environ["HEIMDALL_GPT_MODEL"],
        "llm_gpt_base_url": os.environ["HEIMDALL_GPT_BASE_URL"],
        "llm_gpt_model": os.environ["HEIMDALL_GPT_MODEL"],
        "llm_gpt_api_key": os.environ["HEIMDALL_GPT_API_KEY"],
        "llm_claude_base_url": os.environ["HEIMDALL_CLAUDE_BASE_URL"],
        "llm_claude_model": os.environ["HEIMDALL_CLAUDE_MODEL"],
        "llm_claude_api_key": os.environ["HEIMDALL_CLAUDE_API_KEY"],
        "llm_claude_version": os.environ["HEIMDALL_CLAUDE_VERSION"],
        "llm_system_prompt": "",
        "llm_timeout_seconds": int(os.environ["HEIMDALL_LLM_TIMEOUT_SECONDS"]),
        "llm_temperature": float(os.environ["HEIMDALL_LLM_TEMPERATURE"]),
    },
    "discovery": {
        "enabled": False,
        "mode": "manual",
    },
    "execution": {
        "default_dry_run": False,
        "max_batch_size": 50,
    },
}

print(json.dumps(payload))
PY

echo "Activating Heimdall LLM fallback at ${YGGDRASIL_CORE_BASE_URL}..."

curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${YGGDRASIL_CORE_BASE_URL}/api/v1/integration-instances" \
  --data-binary "@${payload_file}" \
  >"${response_file}"

python3 - "${response_file}" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)

manifest = payload["manifest"]
spec = manifest["spec"]
config = spec["config"]

assert config["llm_enabled"] is True
assert config["llm_provider_order"] == "gpt,claude"
assert str(config["llm_gpt_api_key"]).startswith("secret://")
assert str(config["llm_claude_api_key"]).startswith("secret://")

print("Heimdall LLM fallback is active.")
print(f"instance: {manifest['metadata']['namespace']}/{manifest['metadata']['name']}")
print(f"gpt model: {config['llm_gpt_model']}")
print(f"claude model: {config['llm_claude_model']}")
print(f"gpt secret ref: {config['llm_gpt_api_key']}")
print(f"claude secret ref: {config['llm_claude_api_key']}")
PY
