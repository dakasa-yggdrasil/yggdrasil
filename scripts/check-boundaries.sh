#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

errors=0
runtime_dirs=()
runtime_modules=()

check_no_matches() {
  local label="$1"
  local pattern="$2"
  shift 2
  local output
  if output="$(rg -n "$pattern" "$@" -g '*.go' 2>/dev/null)"; then
    echo "boundary violation: $label"
    echo "$output"
    echo
    errors=1
  fi
}

regex_escape() {
  printf '%s' "$1" | sed 's/[.[\*^$()+?{}|]/\\&/g'
}

discover_runtime_dirs() {
  local base
  for base in "$ROOT/services" "$ROOT/surfaces"; do
    if [[ ! -d "$base" ]]; then
      continue
    fi
    while IFS= read -r dir; do
      [[ -f "$dir/go.mod" ]] || continue
      runtime_dirs+=("$dir")
      runtime_modules+=("$(awk '/^module / { print $2; exit }' "$dir/go.mod")")
    done < <(find "$base" -mindepth 1 -maxdepth 1 -type d | sort)
  done
}

check_cross_runtime_imports() {
  local i j dir module other other_base pattern
  for ((i = 0; i < ${#runtime_dirs[@]}; i++)); do
    dir="${runtime_dirs[$i]}"
    module="${runtime_modules[$i]}"
    for ((j = 0; j < ${#runtime_modules[@]}; j++)); do
      other="${runtime_modules[$j]}"
      [[ "$module" == "$other" ]] && continue
      other_base="$(basename "${other}")"
      pattern="^\\s*\"$(regex_escape "$other")(/|\\\")"
      check_no_matches \
        "$(basename "$dir") must not import runtime module $other_base" \
        "$pattern" \
        "$dir"
    done
  done
}

discover_runtime_dirs
check_cross_runtime_imports

check_no_matches \
  "runtime services and surfaces must not import the workspace root module" \
  '^\s*"github\.com/dakasa-yggdrasil/yggdrasil/' \
  "${runtime_dirs[@]}"

check_no_matches \
  "runtime services and surfaces must not reference plugin runtime code directly" \
  '^\s*"github\.com/(dakasa-co|dakasa-yggdrasil)/yggdrasil-integration-' \
  "${runtime_dirs[@]}"

if [[ "$errors" -ne 0 ]]; then
  exit 1
fi

echo "architecture boundary checks passed"
