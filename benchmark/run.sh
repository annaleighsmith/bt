#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Parse flags
RUN_BT=true
RUN_BR=true
COUNTS=()
ITERATIONS=3

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bt-only) RUN_BR=false; shift ;;
    --br-only) RUN_BT=false; shift ;;
    --iterations) ITERATIONS="$2"; shift 2 ;;
    *) COUNTS+=("$1"); shift ;;
  esac
done

if [[ ${#COUNTS[@]} -eq 0 ]]; then
  COUNTS=(100 1000 5000 10000)
fi

HAS_BR=false
if $RUN_BR && command -v br &>/dev/null; then
  HAS_BR=true
elif $RUN_BR && ! command -v br &>/dev/null; then
  echo "warning: br not found on PATH, skipping" >&2
fi

# Build bt once
BT="$PROJECT_DIR/bt"
if $RUN_BT; then
  echo "Building bt..."
  (cd "$PROJECT_DIR" && go build -o bt .)
fi

# Timing helper: runs command, prints elapsed ms
time_cmd() {
  local start end elapsed
  start=$(date +%s%N)
  "$@" >/dev/null 2>&1
  end=$(date +%s%N)
  elapsed=$(( (end - start) / 1000000 ))
  echo "$elapsed"
}

# Median of space-separated values
median() {
  local sorted
  sorted=$(echo "$@" | tr ' ' '\n' | sort -n)
  local count
  count=$(echo "$sorted" | wc -l | tr -d ' ')
  local mid=$(( (count + 1) / 2 ))
  echo "$sorted" | sed -n "${mid}p"
}

# Run a command N times and return median ms
time_median() {
  local times=()
  for (( run=0; run<ITERATIONS; run++ )); do
    times+=("$(time_cmd "$@")")
  done
  median "${times[@]}"
}

print_read_header() {
  echo "  Count  │ list    │ list -a │ show    │ ready   │ list -j │ ready -j"
  echo "─────────┼─────────┼─────────┼─────────┼─────────┼─────────┼─────────"
}

print_read_row() {
  printf " %7d │ %4d ms │ %4d ms │ %4d ms │ %4d ms │ %4d ms │ %4d ms\n" \
    "$1" "$2" "$3" "$4" "$5" "$6" "$7"
}

print_write_header() {
  echo "  Count  │ q       │ update  │ close   │ dep add"
  echo "─────────┼─────────┼─────────┼─────────┼─────────"
}

print_write_row() {
  printf " %7d │ %4d ms │ %4d ms │ %4d ms │ %4d ms\n" \
    "$1" "$2" "$3" "$4" "$5"
}

# Single parent tempdir — clean up everything on exit
PARENT_TMP=$(mktemp -d)
trap "rm -rf '$PARENT_TMP'" EXIT

declare -a BT_READ BT_WRITE BR_READ BR_WRITE

for N in "${COUNTS[@]}"; do
  WORKDIR="$PARENT_TMP/run-$N"
  mkdir -p "$WORKDIR"

  # Generate data
  JSONL="$WORKDIR/issues.jsonl"
  go run "$SCRIPT_DIR/generate.go" -n "$N" -o "$JSONL" 2>/dev/null

  # Set up bt workspace
  BEADS="$WORKDIR/.beads"
  mkdir -p "$BEADS"
  cp "$JSONL" "$BEADS/issues.jsonl"
  cat > "$BEADS/config.yaml" <<EOF
issue_prefix: bench
EOF

  # Grab first issue ID for show/update/close/dep commands
  FIRST_ID=$(head -1 "$BEADS/issues.jsonl" | jq -r '.id')
  SECOND_ID=$(sed -n '2p' "$BEADS/issues.jsonl" | jq -r '.id')

  if $RUN_BT; then
    echo "bt: benchmarking n=$N (${ITERATIONS} iterations, median)..."

    # Read benchmarks
    t_list=$(cd "$WORKDIR" && time_median "$BT" list)
    t_list_all=$(cd "$WORKDIR" && time_median "$BT" list --all)
    t_show=$(cd "$WORKDIR" && time_median "$BT" show "$FIRST_ID")
    t_ready=$(cd "$WORKDIR" && time_median "$BT" ready)

    # JSON variants
    t_list_json=$(cd "$WORKDIR" && time_median "$BT" list --json)
    t_ready_json=$(cd "$WORKDIR" && time_median "$BT" ready --json)

    # Write benchmarks (each on a fresh copy to avoid state pollution)
    # q: create a new issue
    cp "$JSONL" "$BEADS/issues.jsonl"
    t_q=$(cd "$WORKDIR" && time_median "$BT" q "Benchmark new issue")

    # update: update priority on first issue
    cp "$JSONL" "$BEADS/issues.jsonl"
    t_update=$(cd "$WORKDIR" && time_median "$BT" update "$FIRST_ID" --priority P1)

    # close: close first issue
    cp "$JSONL" "$BEADS/issues.jsonl"
    t_close=$(cd "$WORKDIR" && time_median "$BT" close "$FIRST_ID")

    # dep add: add dep between first two issues
    cp "$JSONL" "$BEADS/issues.jsonl"
    t_dep=$(cd "$WORKDIR" && time_median "$BT" dep add "$SECOND_ID" "$FIRST_ID")

    BT_READ+=("$N $t_list $t_list_all $t_show $t_ready $t_list_json $t_ready_json")
    BT_WRITE+=("$N $t_q $t_update $t_close $t_dep")
  fi

  if $HAS_BR; then
    echo "br: benchmarking n=$N (${ITERATIONS} iterations, median)..."
    BR_WORKDIR="$PARENT_TMP/br-$N"
    mkdir -p "$BR_WORKDIR"
    (cd "$BR_WORKDIR" && br init --prefix bench &>/dev/null || true)
    (cd "$BR_WORKDIR" && br import "$JSONL" &>/dev/null || true)

    BR_FIRST_ID=$(head -1 "$JSONL" | jq -r '.id')
    BR_SECOND_ID=$(sed -n '2p' "$JSONL" | jq -r '.id')

    b_list=$(cd "$BR_WORKDIR" && time_median br list)
    b_list_all=$(cd "$BR_WORKDIR" && time_median br list --all)
    b_show=$(cd "$BR_WORKDIR" && time_median br show "$BR_FIRST_ID")
    b_ready=$(cd "$BR_WORKDIR" && time_median br ready)
    b_q=$(cd "$BR_WORKDIR" && time_median br q "Benchmark new issue")
    b_update=$(cd "$BR_WORKDIR" && time_median br update "$BR_FIRST_ID" --priority P1)
    b_close=$(cd "$BR_WORKDIR" && time_median br close "$BR_FIRST_ID")
    b_dep=$(cd "$BR_WORKDIR" && time_median br dep add "$BR_SECOND_ID" "$BR_FIRST_ID")
    b_list_json=$(cd "$BR_WORKDIR" && time_median br list --json)
    b_ready_json=$(cd "$BR_WORKDIR" && time_median br ready --json)

    BR_READ+=("$N $b_list $b_list_all $b_show $b_ready $b_list_json $b_ready_json")
    BR_WRITE+=("$N $b_q $b_update $b_close $b_dep")
  fi
done

# Print results
if $RUN_BT; then
  echo ""
  echo "bt reads (median of $ITERATIONS runs)"
  print_read_header
  for r in "${BT_READ[@]}"; do
    # shellcheck disable=SC2086
    print_read_row $r
  done
  echo ""
  echo "bt writes (median of $ITERATIONS runs)"
  print_write_header
  for r in "${BT_WRITE[@]}"; do
    # shellcheck disable=SC2086
    print_write_row $r
  done
fi

if $HAS_BR; then
  echo ""
  echo "br reads (median of $ITERATIONS runs)"
  print_read_header
  for r in "${BR_READ[@]}"; do
    # shellcheck disable=SC2086
    print_read_row $r
  done
  echo ""
  echo "br writes (median of $ITERATIONS runs)"
  print_write_header
  for r in "${BR_WRITE[@]}"; do
    # shellcheck disable=SC2086
    print_write_row $r
  done
fi

echo ""
echo "All times in milliseconds (median of $ITERATIONS). Lower is better."
