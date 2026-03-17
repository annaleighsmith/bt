#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Build bt
BT_BIN="$PROJECT_DIR/bt"
echo "Building bt..."
(cd "$PROJECT_DIR" && go build -o bt .)

HAS_BR=false
if command -v br &>/dev/null; then
  HAS_BR=true
fi

# Parent temp dir — one cleanup handles all
PARENT_TMP=$(mktemp -d)
trap 'rm -rf "$PARENT_TMP"' EXIT

time_ms() {
  local start end
  start=$(date +%s%N)
  "$@"
  end=$(date +%s%N)
  echo $(( (end - start) / 1000000 ))
}

# ── Generic workflow functions ──
# All take: $1=workdir, use $CMD as the binary

init_workspace() {
  mkdir -p "$1"
  (cd "$1" && "$CMD" init --prefix agt >/dev/null 2>&1)
}

wf_sprint() {
  cd "$1"
  local IDS=()
  for i in $(seq 1 20); do
    IDS+=("$("$CMD" q "Sprint task $i")")
  done

  "$CMD" list --json | jq -r '.[].id' >/dev/null

  for i in 0 1 2 3 4; do
    "$CMD" update "${IDS[$i]}" --claim >/dev/null
  done
  for i in 0 1 2 3 4; do
    "$CMD" close "${IDS[$i]}" >/dev/null
  done
}

wf_depchain() {
  cd "$1"
  local IDS=()
  for i in $(seq 1 10); do
    IDS+=("$("$CMD" q "Chain task $i")")
  done

  for i in $(seq 1 9); do
    "$CMD" dep add "${IDS[$i]}" "${IDS[$((i-1))]}" >/dev/null
  done

  for i in $(seq 0 9); do
    "$CMD" close "${IDS[$i]}" >/dev/null
    "$CMD" ready --json >/dev/null
  done
}

wf_bulk() {
  cd "$1"
  local IDS=()
  for i in $(seq 1 50); do
    IDS+=("$("$CMD" q "Bulk issue $i")")
  done

  for i in $(seq 0 9); do
    "$CMD" update "${IDS[$i]}" --claim >/dev/null
  done
  for i in $(seq 0 9); do
    "$CMD" close "${IDS[$i]}" >/dev/null
  done

  local COUNT
  COUNT=$("$CMD" list --json | jq length)
  if [[ "$COUNT" -ne 40 ]]; then
    echo "ERROR: expected 40 open, got $COUNT" >&2
  fi
}

wf_pipe() {
  cd "$1"
  for i in $(seq 1 10); do
    "$CMD" q "Pipe issue $i" >/dev/null
    local FIRST
    FIRST=$("$CMD" ready --json | jq -r '.[0].id')
    if [[ -n "$FIRST" && "$FIRST" != "null" ]]; then
      "$CMD" update "$FIRST" --claim >/dev/null
    fi
  done
}

# ── Run all workflows for a given tool ──
run_all() {
  local tool="$1"
  export CMD="$2"
  local prefix="$PARENT_TMP/$tool"

  echo ""
  echo "$tool"
  echo "▸ AI Sprint: q×20 → list --json → claim×5 → close×5"
  init_workspace "$prefix-sprint"
  local t1
  t1=$(time_ms wf_sprint "$prefix-sprint")
  echo "  ${t1} ms"

  echo "▸ Dep Chain: q×10 → chain deps → close bottom-up"
  init_workspace "$prefix-depchain"
  local t2
  t2=$(time_ms wf_depchain "$prefix-depchain")
  echo "  ${t2} ms"

  echo "▸ Bulk Ops: q×50 → claim×10 → close×10 → verify"
  init_workspace "$prefix-bulk"
  local t3
  t3=$(time_ms wf_bulk "$prefix-bulk")
  echo "  ${t3} ms"

  echo "▸ JSON Pipe: (create → ready --json → jq → claim) ×10"
  init_workspace "$prefix-pipe"
  local t4
  t4=$(time_ms wf_pipe "$prefix-pipe")
  echo "  ${t4} ms"

  # Stash results in global arrays
  eval "${tool}_RESULTS=($t1 $t2 $t3 $t4)"
}

echo ""
echo "=== Agent Workflow Benchmarks ==="

run_all "bt" "$BT_BIN"

if $HAS_BR; then
  run_all "br" "br"
fi

# ── Comparison table ──
echo ""
WORKFLOWS=("AI Sprint (20 issues)" "Dep Chain (10 issues)" "Bulk Ops (50 issues)" "JSON Pipe (10 cycles)")

if $HAS_BR; then
  echo "  Workflow                       │     bt │     br"
  echo "─────────────────────────────────┼────────┼────────"
  for i in 0 1 2 3; do
    printf "  %-31s │ %4d ms│ %4d ms\n" "${WORKFLOWS[$i]}" "${bt_RESULTS[$i]}" "${br_RESULTS[$i]}"
  done
else
  echo "  Workflow                       │     bt"
  echo "─────────────────────────────────┼────────"
  for i in 0 1 2 3; do
    printf "  %-31s │ %4d ms\n" "${WORKFLOWS[$i]}" "${bt_RESULTS[$i]}"
  done
fi

echo ""
echo "All times wall-clock milliseconds. Lower is better."
