#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

export BT="$PROJECT_DIR/bt"
echo "Building bt..."
(cd "$PROJECT_DIR" && go build -o bt .)

HAS_BR=false
if command -v br &>/dev/null; then
  HAS_BR=true
else
  echo "br not found, skipping" >&2
  exit 1
fi

PARENT_TMP=$(mktemp -d)
trap 'rm -rf "$PARENT_TMP"' EXIT

CONCURRENCY_LEVELS=(1 5 10 50)

# Pre-seed with 50 issues so we're not starting from empty
seed_workspace() {
  local cmd="$1" dir="$2"
  for i in $(seq 1 50); do
    "$cmd" q "Seed issue $i" >/dev/null 2>&1
  done
}

time_ms() {
  local start end
  start=$(date +%s%N)
  "$@"
  end=$(date +%s%N)
  echo $(( (end - start) / 1000000 ))
}

# Run N concurrent writes and measure wall-clock
run_concurrent_writes() {
  local cmd="$1" dir="$2" n="$3"
  cd "$dir"
  for i in $(seq 1 "$n"); do
    "$cmd" q "Concurrent write $i" >/dev/null 2>&1 &
  done
  wait
}

# Run N concurrent reads and measure wall-clock
run_concurrent_reads() {
  local cmd="$1" dir="$2" n="$3"
  cd "$dir"
  for i in $(seq 1 "$n"); do
    "$cmd" list --json >/dev/null 2>&1 &
  done
  wait
}

echo ""
echo "=== Concurrent Write Benchmark (500 pre-seeded issues) ==="
echo ""

declare -a BT_WRITE_RESULTS BR_WRITE_RESULTS

for N in "${CONCURRENCY_LEVELS[@]}"; do
  # bt workspace
  BT_DIR="$PARENT_TMP/bt-write-$N"
  mkdir -p "$BT_DIR"
  (cd "$BT_DIR" && "$BT" init --prefix cw >/dev/null 2>&1)
  (cd "$BT_DIR" && seed_workspace "$BT" "$BT_DIR")
  bt_ms=$(time_ms run_concurrent_writes "$BT" "$BT_DIR" "$N")
  bt_count=$(cd "$BT_DIR" && "$BT" list --all --json | jq length)
  BT_WRITE_RESULTS+=("$N $bt_ms $bt_count")

  # br workspace
  BR_DIR="$PARENT_TMP/br-write-$N"
  mkdir -p "$BR_DIR"
  (cd "$BR_DIR" && br init --prefix cw >/dev/null 2>&1)
  (cd "$BR_DIR" && seed_workspace br "$BR_DIR")
  br_ms=$(time_ms run_concurrent_writes br "$BR_DIR" "$N")
  br_count=$(cd "$BR_DIR" && br list --all --json | jq length)
  BR_WRITE_RESULTS+=("$N $br_ms $br_count")

  echo "  n=$N  bt: ${bt_ms}ms (${bt_count} issues)  br: ${br_ms}ms (${br_count} issues)"
done

echo ""
echo "=== Concurrent Read Benchmark (50 issues) ==="
echo ""

declare -a BT_READ_RESULTS BR_READ_RESULTS

for N in "${CONCURRENCY_LEVELS[@]}"; do
  # bt workspace (reuse a seeded one)
  BT_DIR="$PARENT_TMP/bt-read"
  if [[ ! -d "$BT_DIR" ]]; then
    mkdir -p "$BT_DIR"
    (cd "$BT_DIR" && "$BT" init --prefix cr >/dev/null 2>&1)
    (cd "$BT_DIR" && seed_workspace "$BT" "$BT_DIR")
  fi
  bt_ms=$(time_ms run_concurrent_reads "$BT" "$BT_DIR" "$N")
  BT_READ_RESULTS+=("$N $bt_ms")

  # br workspace
  BR_DIR="$PARENT_TMP/br-read"
  if [[ ! -d "$BR_DIR" ]]; then
    mkdir -p "$BR_DIR"
    (cd "$BR_DIR" && br init --prefix cr >/dev/null 2>&1)
    (cd "$BR_DIR" && seed_workspace br "$BR_DIR")
  fi
  br_ms=$(time_ms run_concurrent_reads br "$BR_DIR" "$N")
  BR_READ_RESULTS+=("$N $br_ms")

  echo "  n=$N  bt: ${bt_ms}ms  br: ${br_ms}ms"
done

echo ""
echo "── Concurrent Writes (50 base + N parallel creates) ──"
echo "  N    │     bt │     br │ winner"
echo "───────┼────────┼────────┼────────"
for r in "${BT_WRITE_RESULTS[@]}"; do
  read -r n bt_ms bt_count <<< "$r"
  # find matching br result
  for br_r in "${BR_WRITE_RESULTS[@]}"; do
    read -r br_n br_ms br_count <<< "$br_r"
    if [[ "$br_n" == "$n" ]]; then
      if [[ "$bt_ms" -le "$br_ms" ]]; then
        winner="bt"
      else
        winner="br"
      fi
      printf "  %-5d │ %4d ms│ %4d ms│ %s\n" "$n" "$bt_ms" "$br_ms" "$winner"
      break
    fi
  done
done

echo ""
echo "── Concurrent Reads (500 issues, N parallel list --json) ──"
echo "  N    │     bt │     br │ winner"
echo "───────┼────────┼────────┼────────"
for i in "${!BT_READ_RESULTS[@]}"; do
  read -r n bt_ms <<< "${BT_READ_RESULTS[$i]}"
  read -r _ br_ms <<< "${BR_READ_RESULTS[$i]}"
  if [[ "$bt_ms" -le "$br_ms" ]]; then
    winner="bt"
  else
    winner="br"
  fi
  printf "  %-5d │ %4d ms│ %4d ms│ %s\n" "$n" "$bt_ms" "$br_ms" "$winner"
done

echo ""
echo "All times wall-clock milliseconds. Lower is better."
