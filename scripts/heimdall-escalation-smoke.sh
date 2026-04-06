#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "${ROOT_DIR}/.env" ]]; then
  # shellcheck disable=SC1091
  source "${ROOT_DIR}/.env"
fi

CORE_HTTP_PORT="${CORE_HTTP_PORT:-9080}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-yggdrasil-dev}"
core_url="http://127.0.0.1:${CORE_HTTP_PORT}"
docker_compose="${ROOT_DIR}/scripts/docker-compose.sh"
tmp_dir="$(mktemp -d)"
requests_file="${tmp_dir}/requests.jsonl"
mock_container="yggdrasil-heimdall-github-mock-${COMPOSE_PROJECT_NAME}"
network_name="${COMPOSE_PROJECT_NAME}_default"
github_runtime_dir="${tmp_dir}/integration-github"
github_runtime_image="yggdrasil-heimdall-smoke-integration-github:${COMPOSE_PROJECT_NAME}"
github_runtime_container="yggdrasil-heimdall-integration-github-${COMPOSE_PROJECT_NAME}"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "$1 is required" >&2
    exit 1
  }
}

cleanup() {
  docker rm -f "${mock_container}" >/dev/null 2>&1 || true
  docker rm -f "${github_runtime_container}" >/dev/null 2>&1 || true
  if curl -fsS -o /dev/null "${core_url}/readyz" 2>/dev/null; then
    COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME}" "${docker_compose}" exec yggdrasil-core \
      sh -lc "/usr/local/go/bin/go run ./scripts/bootstrap --path ./docs/bootstrap/manifests" >/dev/null 2>&1 || true
  fi
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

wait_for_url() {
  local url="$1"
  local attempts="${2:-30}"
  local sleep_seconds="${3:-2}"

  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS -o /dev/null "${url}" 2>/dev/null; then
      return 0
    fi
    sleep "${sleep_seconds}"
  done

  echo "timed out waiting for ${url}" >&2
  return 1
}

wait_for_running_service() {
  local service="$1"
  local attempts="${2:-60}"
  local sleep_seconds="${3:-2}"

  for ((i = 1; i <= attempts; i++)); do
    if COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME}" "${docker_compose}" ps --services --status running | grep -qx "${service}"; then
      return 0
    fi
    sleep "${sleep_seconds}"
  done

  echo "timed out waiting for service ${service}" >&2
  return 1
}

wait_for_deploy_request() {
  local bundle_name="$1"
  local incident_title="$2"
  local creation_summary="$3"
  local attempts="${4:-30}"

  for ((i = 1; i <= attempts; i++)); do
    docker logs "${mock_container}" > "${requests_file}" 2>/dev/null || true
    if python3 - "${requests_file}" "${bundle_name}" "${incident_title}" "${creation_summary}" <<'PY'
import json
import sys

requests_file, bundle_name, incident_title, creation_summary = sys.argv[1:]
expected_path = "/repos/dakasa-yggdrasil/yggdrasil/actions/workflows/deploy.yml/dispatches"

with open(requests_file, "r", encoding="utf-8") as handle:
    for line in handle:
        line = line.strip()
        if not line:
            continue
        record = json.loads(line)
        if record.get("path") != expected_path:
            continue
        body = record.get("body") or {}
        inputs = body.get("inputs") or {}
        assert body.get("ref") == "main", body
        assert inputs.get("component_kind") == "product", inputs
        assert inputs.get("component_name") == "yggdrasil", inputs
        assert inputs.get("component_namespace") == "global", inputs
        assert inputs.get("incident_title") == incident_title, inputs
        assert inputs.get("remediation_bundle_name") == bundle_name, inputs
        assert inputs.get("remediation_bundle_namespace") == "global", inputs
        assert inputs.get("remediation_bundle_kind") == "workflow_patch", inputs
        assert inputs.get("remediation_bundle_creation_reason_summary") == creation_summary, inputs
        assert inputs.get("remediation_bundle_approval_status") == "approved", inputs
        assert inputs.get("remediation_bundle_approval_comment") == "Heimdall remediation bundle smoke approval", inputs
        sys.exit(0)

sys.exit(1)
PY
    then
      return 0
    fi
    sleep 1
  done

  echo "timed out waiting for deploy dispatch for bundle ${bundle_name}" >&2
  return 1
}

wait_for_escalation_request() {
  local expected_path="$1"
  local expected_severity="$2"
  local expected_kind="$3"
  local expected_title="$4"
  local bundle_name="$5"
  local creation_summary="$6"
  local promotion_status="$7"
  local promotion_summary="$8"
  local attempts="${9:-30}"

  for ((i = 1; i <= attempts; i++)); do
    docker logs "${mock_container}" > "${requests_file}" 2>/dev/null || true
    if python3 - "${requests_file}" "${expected_path}" "${expected_severity}" "${expected_kind}" "${expected_title}" "${bundle_name}" "${creation_summary}" "${promotion_status}" "${promotion_summary}" <<'PY'
import json
import sys

(
    requests_file,
    expected_path,
    expected_severity,
    expected_kind,
    expected_title,
    bundle_name,
    creation_summary,
    promotion_status,
    promotion_summary,
) = sys.argv[1:]

with open(requests_file, "r", encoding="utf-8") as handle:
    for line in handle:
        line = line.strip()
        if not line:
            continue
        record = json.loads(line)
        if record.get("path") != expected_path:
            continue
        body = record.get("body") or {}
        inputs = body.get("inputs") or {}
        assert body.get("ref") == "main", body
        assert inputs.get("component_kind") == "product", inputs
        assert inputs.get("component_name") == "yggdrasil", inputs
        assert inputs.get("component_namespace") == "global", inputs
        assert inputs.get("environment") == "production", inputs
        assert inputs.get("event_name") == "incident_escalation", inputs
        assert inputs.get("incident_severity") == expected_severity, inputs
        assert inputs.get("incident_title") == expected_title, inputs
        assert inputs.get("escalation_kind") == expected_kind, inputs
        assert inputs.get("remediation_bundle_name") == bundle_name, inputs
        assert inputs.get("remediation_bundle_creation_reason_summary") == creation_summary, inputs
        assert inputs.get("remediation_bundle_approval_status") == "approved", inputs
        assert inputs.get("remediation_bundle_promotion_status") == promotion_status, inputs
        assert inputs.get("remediation_bundle_promotion_summary") == promotion_summary, inputs
        expected_postmortem = expected_kind == "postmortem"
        assert bool(inputs.get("postmortem_required")) == expected_postmortem, inputs
        sys.exit(0)

sys.exit(1)
PY
    then
      return 0
    fi
    sleep 1
  done

  echo "timed out waiting for request ${expected_path}" >&2
  return 1
}

create_bundle() {
  local bundle_name="$1"
  local incident_title="$2"
  local creation_summary="$3"
  local expires_at="$4"

  curl -fsS \
    -X POST \
    -H "Content-Type: application/json" \
    "${core_url}/api/v1/remediation-bundles" \
    -d "$(cat <<JSON
{
  "name": "${bundle_name}",
  "namespace": "global",
  "description": "Smoke remediation bundle used to validate Heimdall workflow dispatch context.",
  "labels": {
    "guardian": "heimdall",
    "yggdrasil.io/smoke-test": "true"
  },
  "spec": {
    "guardian_ref": {
      "namespace": "global",
      "name": "heimdall-guardian"
    },
    "status": "pending_approval",
    "source": "llm_generated",
    "bundle_kind": "workflow_patch",
    "summary": "Smoke remediation bundle for deploy validation.",
    "component_kind": "product",
    "component_namespace": "global",
    "component_name": "yggdrasil",
    "expires_at": "${expires_at}",
    "trigger_action": {
      "type": "remediation_bundle",
      "component_kind": "product",
      "component_namespace": "global",
      "component_name": "yggdrasil",
      "incident_title": "${incident_title}",
      "reason": "Smoke remediation bundle execution",
      "target": {
        "remediation_bundle": {
          "namespace": "global",
          "name": "${bundle_name}"
        }
      }
    },
    "incident": {
      "title": "${incident_title}",
      "category": "oom_killed",
      "severity": "critical"
    },
    "creation_reason": {
      "kind": "generated_hotfix_bundle",
      "summary": "${creation_summary}",
      "comment": "Smoke validation is exercising the generated bundle path end-to-end.",
      "source": "heimdall",
      "actor": "heimdall"
    },
    "steps": [
      {
        "name": "deploy-hotfix",
        "mode": "workflow_dispatch",
        "description": "Dispatch the product deploy workflow with structured remediation bundle context.",
        "blast_radius": "low",
        "workflow_dispatch": {
          "repository": "dakasa-yggdrasil/yggdrasil",
          "workflow": "deploy.yml",
          "ref": "main",
          "inputs": {
            "remediation_type": "bundle_hotfix",
            "remediation_reason": "Smoke remediation bundle dispatch"
          }
        }
      }
    ],
    "metadata": {
      "promotion_target": "learned_lightweight",
      "approval_required": true,
      "smoke_test": true
    }
  }
}
JSON
)" >/dev/null
}

promote_bundle_review() {
  local bundle_name="$1"
  local promotion_summary="$2"
  local response_file="${tmp_dir}/${bundle_name}-response.json"
  local payload_file="${tmp_dir}/${bundle_name}-promotion.json"

  curl -fsS "${core_url}/api/v1/remediation-bundles?namespace=global&name=${bundle_name}&active_only=true" > "${response_file}"
  python3 - "${bundle_name}" "${promotion_summary}" "${response_file}" > "${payload_file}" <<'PY'
import json
import sys
from datetime import datetime, timezone

bundle_name, promotion_summary, response_file = sys.argv[1:]
with open(response_file, "r", encoding="utf-8") as handle:
    response = json.load(handle)
manifests = response.get("manifests") or []
if not manifests:
    raise SystemExit(f"remediation bundle {bundle_name} not found")

manifest = manifests[0]
spec = manifest.get("spec") or {}
spec["promotion_review"] = {
    "kind": "promoted",
    "status": "promoted",
    "summary": promotion_summary,
    "comment": "Smoke validation promoted this generated pattern after a successful dry verification path.",
    "source": "console",
    "actor": "smoke-test",
    "recorded_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    "metadata": {
        "review_source": "heimdall-escalation-smoke"
    },
}

payload = {
    "name": manifest["metadata"]["name"],
    "namespace": manifest["metadata"]["namespace"],
    "description": manifest["metadata"].get("description", ""),
    "labels": manifest["metadata"].get("labels", {}),
    "spec": spec,
}
json.dump(payload, sys.stdout)
PY

  curl -fsS \
    -X POST \
    -H "Content-Type: application/json" \
    "${core_url}/api/v1/remediation-bundles" \
    --data-binary "@${payload_file}" >/dev/null
}

enable_generated_bundles_for_smoke() {
  local response_file="${tmp_dir}/heimdall-policy-response.json"
  local payload_file="${tmp_dir}/heimdall-policy-smoke.json"

  curl -fsS "${core_url}/api/v1/guardian-policies?namespace=global&name=heimdall-default&active_only=true" > "${response_file}"
  python3 - "${response_file}" > "${payload_file}" <<'PY'
import json
import sys

response_file = sys.argv[1]
with open(response_file, "r", encoding="utf-8") as handle:
    response = json.load(handle)
manifests = response.get("manifests") or []
if not manifests:
    raise SystemExit("guardian policy global/heimdall-default not found")

manifest = manifests[0]
spec = manifest.get("spec") or {}
generated = dict(spec.get("generated_bundles") or {})
generated.update({
    "enabled": True,
    "require_approval": True,
    "allow_workflow_patch": True,
    "allow_integration_composition": True,
    "allow_ephemeral_executor": True,
    "max_ttl_seconds": 7200,
})
spec["generated_bundles"] = generated

payload = {
    "name": manifest["metadata"]["name"],
    "namespace": manifest["metadata"]["namespace"],
    "description": manifest["metadata"].get("description", ""),
    "labels": manifest["metadata"].get("labels", {}),
    "spec": spec,
}
json.dump(payload, sys.stdout)
PY

  curl -fsS \
    -X POST \
    -H "Content-Type: application/json" \
    "${core_url}/api/v1/guardian-policies" \
    --data-binary "@${payload_file}" >/dev/null
}

create_bundle_approval() {
  local approval_name="$1"
  local bundle_name="$2"
  local incident_title="$3"

  curl -fsS \
    -X POST \
    -H "Content-Type: application/json" \
    "${core_url}/api/v1/guardian-approvals" \
    -d "$(cat <<JSON
{
  "name": "${approval_name}",
  "namespace": "global",
  "description": "Smoke approval used to validate Heimdall remediation bundle execution.",
  "labels": {
    "yggdrasil.io/smoke-test": "true"
  },
  "spec": {
    "guardian_ref": {
      "namespace": "global",
      "name": "heimdall-guardian"
    },
    "status": "pending",
    "source": "remediation_bundle_smoke",
    "summary": "Approve smoke remediation bundle execution",
    "action": {
      "type": "remediation_bundle",
      "component_kind": "product",
      "component_namespace": "global",
      "component_name": "yggdrasil",
      "incident_title": "${incident_title}",
      "reason": "Smoke remediation bundle execution",
      "target": {
        "remediation_bundle": {
          "namespace": "global",
          "name": "${bundle_name}"
        }
      }
    },
    "incident": {
      "title": "${incident_title}",
      "category": "oom_killed",
      "severity": "critical"
    },
    "metadata": {
      "smoke_test": true,
      "bundle_name": "${bundle_name}",
      "bundle_namespace": "global"
    }
  }
}
JSON
)" >/dev/null
}

create_escalation_approval() {
  local approval_name="$1"
  local bundle_name="$2"
  local incident_title="$3"
  local incident_category="$4"
  local incident_severity="$5"
  local workflow_name="$6"
  local escalation_kind="$7"
  local postmortem_required="$8"

  curl -fsS \
    -X POST \
    -H "Content-Type: application/json" \
    "${core_url}/api/v1/guardian-approvals" \
    -d "$(cat <<JSON
{
  "name": "${approval_name}",
  "namespace": "global",
  "description": "Smoke approval used to validate Heimdall escalation dispatch.",
  "labels": {
    "yggdrasil.io/smoke-test": "true"
  },
  "spec": {
    "guardian_ref": {
      "namespace": "global",
      "name": "heimdall-guardian"
    },
    "status": "pending",
    "source": "incident_escalation",
    "summary": "Heimdall escalation smoke validation",
    "action": {
      "type": "dispatch_workflow",
      "component_kind": "product",
      "component_namespace": "global",
      "component_name": "yggdrasil",
      "reason": "Smoke escalation validation",
      "workflow": {
        "workflow": "${workflow_name}",
        "ref": "main",
        "inputs": {
          "escalation_kind": "${escalation_kind}",
          "escalation_reason": "Smoke escalation validation",
          "incident_severity": "${incident_severity}",
          "incident_title": "${incident_title}",
          "incident_category": "${incident_category}",
          "postmortem_required": ${postmortem_required}
        }
      },
      "target": {
        "remediation_bundle": {
          "namespace": "global",
          "name": "${bundle_name}"
        }
      },
      "incident_title": "${incident_title}",
      "incident_category": "${incident_category}",
      "incident_severity": "${incident_severity}",
      "incident": {
        "title": "${incident_title}",
        "category": "${incident_category}",
        "severity": "${incident_severity}"
      }
    },
    "incident": {
      "title": "${incident_title}",
      "category": "${incident_category}",
      "severity": "${incident_severity}"
    },
    "metadata": {
      "smoke_test": true
    }
  }
}
JSON
)" >/dev/null
}

approve_approval() {
  local approval_name="$1"
  local decision_comment="${2:-Heimdall escalation smoke approval}"
  local response
  local body
  local status

  response="$(
    curl -sS \
    -X POST \
    -H "Content-Type: application/json" \
    "${core_url}/api/v1/guardian-approvals/global/${approval_name}/decision" \
    -d "$(cat <<JSON
{"status":"approved","comment":"${decision_comment}"}
JSON
)" \
    -w '\n%{http_code}'
  )"
  body="${response%$'\n'*}"
  status="${response##*$'\n'}"
  if [[ "${status}" != "200" && "${status}" != "201" ]]; then
    echo "guardian approval decision failed with status ${status}: ${body}" >&2
    return 1
  fi
}

require_tool curl
require_tool python3
require_tool docker

wait_for_url "${core_url}/readyz" 60 2

docker network inspect "${network_name}" >/dev/null 2>&1 || {
  echo "docker network ${network_name} not found" >&2
  exit 1
}

if COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME}" "${docker_compose}" config --services | grep -qx "integration-github"; then
  echo "Ensuring installed integration-github is running..."
  COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME}" "${docker_compose}" up -d integration-github >/dev/null
  wait_for_running_service "integration-github" 60 2
else
  require_tool git
  echo "integration-github is not installed in the monorepo; starting a temporary runtime from the catalog remote..."
  github_repo_url="$(ROOT_DIR="${ROOT_DIR}" python3 - <<'PY'
import json
import os

catalog_path = os.path.join(os.environ["ROOT_DIR"], "catalog", "integrations.json")

with open(catalog_path, "r", encoding="utf-8") as handle:
    data = json.load(handle)

for integration in data.get("integrations", []):
    if integration.get("slug") == "github":
        print(integration["repo_url"])
        break
else:
    raise SystemExit("github integration repo_url not found in catalog")
PY
)"
  git clone --depth 1 "${github_repo_url}" "${github_runtime_dir}" >/dev/null 2>&1
  docker build -t "${github_runtime_image}" "${github_runtime_dir}" >/dev/null
  docker rm -f "${github_runtime_container}" >/dev/null 2>&1 || true
  docker run -d --rm \
    --name "${github_runtime_container}" \
    --network "${network_name}" \
    -e BROKER_URL="amqp://guest:guest@yggdrasil-rabbitmq:5672/" \
    "${github_runtime_image}" >/dev/null

  for ((i = 1; i <= 30; i++)); do
  if [[ "$(docker inspect -f '{{.State.Running}}' "${github_runtime_container}" 2>/dev/null || true)" == "true" ]]; then
      break
    fi
    sleep 1
  done

  if [[ "$(docker inspect -f '{{.State.Running}}' "${github_runtime_container}" 2>/dev/null || true)" != "true" ]]; then
    echo "temporary integration-github runtime did not start" >&2
    docker logs "${github_runtime_container}" || true
    exit 1
  fi
  sleep 5
fi

: > "${requests_file}"

echo "Starting GitHub mock on Docker network ${network_name}..."
docker rm -f "${mock_container}" >/dev/null 2>&1 || true
mock_python="$(cat <<'PY'
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers.get("Content-Length", "0"))
        payload = self.rfile.read(content_length) if content_length else b""
        body = payload.decode("utf-8") if payload else ""
        try:
            parsed_body = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed_body = {"raw": body}

        sys.stdout.write(json.dumps({
            "method": "POST",
            "path": self.path,
            "headers": dict(self.headers),
            "body": parsed_body,
        }) + "\n")
        sys.stdout.flush()

        if self.path.startswith("/repos/") and self.path.endswith("/dispatches"):
            self.send_response(204)
            self.end_headers()
            return

        self.send_response(404)
        self.end_headers()

    def log_message(self, format, *args):
        return


server = ThreadingHTTPServer(("0.0.0.0", 8080), Handler)
server.serve_forever()
PY
)"
docker run -d --rm \
  --name "${mock_container}" \
  --network "${network_name}" \
  python:3.12-alpine \
  python -u -c "${mock_python}" >/dev/null

for ((i = 1; i <= 20; i++)); do
  if [[ "$(docker inspect -f '{{.State.Running}}' "${mock_container}" 2>/dev/null || true)" == "true" ]]; then
    break
  fi
  sleep 1
done

if [[ "$(docker inspect -f '{{.State.Running}}' "${mock_container}" 2>/dev/null || true)" != "true" ]]; then
  echo "mock GitHub container did not start" >&2
  docker logs "${mock_container}" || true
  exit 1
fi

for ((i = 1; i <= 20; i++)); do
  if docker run --rm --network "${network_name}" python:3.12-alpine python - <<PY >/dev/null 2>&1
import urllib.request

request = urllib.request.Request(
    "http://${mock_container}:8080/ready",
    data=b"{}",
    method="POST",
)
with urllib.request.urlopen(request, timeout=2) as response:
    raise SystemExit(0 if response.status == 204 else 1)
PY
  then
    break
  fi
  sleep 1
done

if ! docker run --rm --network "${network_name}" python:3.12-alpine python - <<PY >/dev/null 2>&1
import urllib.request

request = urllib.request.Request(
    "http://${mock_container}:8080/ready",
    data=b"{}",
    method="POST",
)
with urllib.request.urlopen(request, timeout=2) as response:
    raise SystemExit(0 if response.status == 204 else 1)
PY
then
  echo "mock GitHub container did not accept connections in time" >&2
  docker logs "${mock_container}" || true
  exit 1
fi

slug_suffix="$(date +%s)"
github_secret_name="github-caller-smoke-${slug_suffix}"
bundle_name="heimdall-smoke-bundle-${slug_suffix}"
bundle_approval="heimdall-smoke-bundle-approval-${slug_suffix}"
bundle_title="Smoke remediation bundle deploy ${slug_suffix}"
bundle_creation_summary="Smoke hotfix bundle generated because the bounded execute path was intentionally unavailable."
promotion_summary="Smoke validation promoted this generated bundle to a lightweight learned playbook."
issue_title="Smoke issue escalation ${slug_suffix}"
postmortem_title="Smoke postmortem escalation ${slug_suffix}"
issue_approval="heimdall-smoke-issue-${slug_suffix}"
postmortem_approval="heimdall-smoke-postmortem-${slug_suffix}"
bundle_expires_at="$(python3 - <<'PY'
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc) + timedelta(hours=2)).isoformat().replace("+00:00", "Z"))
PY
)"

echo "Creating managed secret for github-caller smoke credentials..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/secrets" \
  -d "$(cat <<JSON
{
  "namespace": "global",
  "name": "${github_secret_name}",
  "status": "active",
  "data": {
    "token": "smoke-github-token"
  },
  "metadata": {
    "source_kind": "integration_instance",
    "integration_instance": {
      "namespace": "global",
      "name": "github-caller"
    },
    "smoke_test": true
  },
  "rotation": {
    "mode": "manual"
  }
}
JSON
)" >/dev/null

echo "Pointing github-caller to the local GitHub mock..."
curl -fsS \
  -X POST \
  -H "Content-Type: application/json" \
  "${core_url}/api/v1/integration-instances" \
  -d "$(cat <<JSON
{
  "name": "github-caller",
  "namespace": "global",
  "description": "Smoke GitHub caller override.",
  "labels": {
    "bootstrap_source": "integration-github",
    "provider": "github",
    "yggdrasil.io/smoke-test": "true"
  },
  "type_ref": {
    "namespace": "global",
    "name": "github"
  },
  "status": "active",
  "owners": ["team:platform"],
  "credentials_ref": "secret://global/${github_secret_name}",
  "config": {
    "default_owner": "dakasa-yggdrasil",
    "default_ref": "main",
    "api_base_url": "http://${mock_container}:8080"
  },
  "discovery": {
    "enabled": false,
    "mode": "manual",
    "sync_interval_seconds": 0
  },
  "execution": {
    "default_dry_run": false,
    "max_batch_size": 10
  }
}
JSON
)" >/dev/null

echo "Temporarily enabling generated remediation bundles in the Heimdall policy..."
enable_generated_bundles_for_smoke

echo "Creating remediation bundle and approval..."
create_bundle "${bundle_name}" "${bundle_title}" "${bundle_creation_summary}" "${bundle_expires_at}"
create_bundle_approval "${bundle_approval}" "${bundle_name}" "${bundle_title}"
approve_approval "${bundle_approval}" "Heimdall remediation bundle smoke approval"
wait_for_deploy_request "${bundle_name}" "${bundle_title}" "${bundle_creation_summary}" 40

echo "Promoting remediation bundle after successful deploy dispatch..."
promote_bundle_review "${bundle_name}" "${promotion_summary}"

echo "Dispatching Heimdall issue escalation with remediation bundle context..."
create_escalation_approval "${issue_approval}" "${bundle_name}" "${issue_title}" "oom_killed" "high" "incident-escalation.yml" "issue" "false"
approve_approval "${issue_approval}" "Heimdall escalation smoke approval"
wait_for_escalation_request \
  "/repos/dakasa-yggdrasil/yggdrasil/actions/workflows/incident-escalation.yml/dispatches" \
  "high" \
  "issue" \
  "${issue_title}" \
  "${bundle_name}" \
  "${bundle_creation_summary}" \
  "promoted" \
  "${promotion_summary}" \
  40

echo "Dispatching Heimdall postmortem escalation with remediation bundle context..."
create_escalation_approval "${postmortem_approval}" "${bundle_name}" "${postmortem_title}" "oom_killed" "critical" "postmortem.yml" "postmortem" "true"
approve_approval "${postmortem_approval}" "Heimdall escalation smoke approval"
wait_for_escalation_request \
  "/repos/dakasa-yggdrasil/yggdrasil/actions/workflows/postmortem.yml/dispatches" \
  "critical" \
  "postmortem" \
  "${postmortem_title}" \
  "${bundle_name}" \
  "${bundle_creation_summary}" \
  "promoted" \
  "${promotion_summary}" \
  40

docker logs "${mock_container}" > "${requests_file}" 2>/dev/null || true
python3 - "${requests_file}" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    rows = [json.loads(line) for line in handle if line.strip()]

assert len(rows) >= 3, rows
paths = {row["path"] for row in rows}
assert "/repos/dakasa-yggdrasil/yggdrasil/actions/workflows/deploy.yml/dispatches" in paths, paths
assert "/repos/dakasa-yggdrasil/yggdrasil/actions/workflows/incident-escalation.yml/dispatches" in paths, paths
assert "/repos/dakasa-yggdrasil/yggdrasil/actions/workflows/postmortem.yml/dispatches" in paths, paths
PY

echo "Heimdall remediation bundle smoke passed."
