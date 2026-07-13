#!/bin/bash
#
# Samurai (MPT) proof benchmark sweep against a running samurai gRPC server.
#
# For each (range_size, duration) pair (same index in RANGE_SIZES and DURATIONS),
# runs proofc bench, then sleeps for cooldown. Does not start a server;
# point SERVER_ADDR at your samurai serve instance.
#
# When VERIFY=1, proofc needs PARAMS_DIR; set STATE_ROOT to your chain root (override the default).
# Summaries are written under OUTPUT_DIR (see benchutil / proofc --help).

set -euo pipefail

# --- Configuration ---
CLIENT_BIN=${CLIENT_BIN:-./bin/proofc}
SERVER_ADDR=${SERVER_ADDR:-clnode332.clemson.cloudlab.us:50051}
ACCOUNTS_LIST=${ACCOUNTS_LIST:-account_stats_all.csv}
OUTPUT_DIR=${OUTPUT_DIR:-/data/local/proof_benchmark_output}
NUM_CLIENTS=${NUM_CLIENTS:-32}
COOLDOWN=${COOLDOWN:-10}
# ass numclient postfix to output dir
OUTPUT_DIR=${OUTPUT_DIR}/numclients${NUM_CLIENTS}/non_optimus

PARAMS_DIR=${PARAMS_DIR:-./data/params}
STATE_ROOT=${STATE_ROOT:-0x23d8d542bf8049c57540df2b673db6287aa9a0191a85eaf1049d1c89c9d965a2}

# Set USE_OLD=1 to pass proofc --old (slow path).
USE_OLD=${USE_OLD:-1}

# One duration per range size (must be the same length).
RANGE_SIZES=(${RANGE_SIZES:-1 100 500 1000 5000 7000 50000 200000 600000 1200000 2600000})
DURATIONS=(${DURATIONS:-2m 2m 2m 2m 2m 2m 2m 2m 2m 2m 2m})
# RANGE_SIZES=(${RANGE_SIZES:-1000 5000 7000 50000 200000 600000 1200000 2600000})
# DURATIONS=(${DURATIONS:-2m 2m 2m 2m 2m 2m 2m 2m})

# Set VERIFY=0 to disable local proof verification.
VERIFY=${VERIFY:-1}

# --- Setup ---
mkdir -p "$OUTPUT_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

if [[ "$VERIFY" == "1" ]]; then
  VERIFY_ARGS=(--verify)
else
  VERIFY_ARGS=(--verify=false)
fi

EXTRA_BENCH_ARGS=(--params-dir "$PARAMS_DIR" --state-root "$STATE_ROOT")
if [[ "$USE_OLD" == "1" ]]; then
  EXTRA_BENCH_ARGS+=(--old)
fi

echo "=== Samurai Proof Bench Sweep ==="
echo "Timestamp:          $TIMESTAMP"
echo "Server:             $SERVER_ADDR"
echo "Accounts list:      $ACCOUNTS_LIST"
echo "Clients:            $NUM_CLIENTS"
echo "Output dir:         $OUTPUT_DIR"
echo "Params dir:         $PARAMS_DIR"
echo "State root:         $STATE_ROOT"
echo "Use old proof path: $USE_OLD"
echo "Range sizes:        ${RANGE_SIZES[*]}"
echo "Durations:          ${DURATIONS[*]}"
echo "Cooldown:           ${COOLDOWN}s"
echo ""

if ((${#RANGE_SIZES[@]} != ${#DURATIONS[@]})); then
  echo "error: RANGE_SIZES (${#RANGE_SIZES[@]} entries) and DURATIONS (${#DURATIONS[@]} entries) must have the same length" >&2
  exit 1
fi

for i in "${!RANGE_SIZES[@]}"; do
  range_size="${RANGE_SIZES[$i]}"
  duration="${DURATIONS[$i]}"

  echo "=== range_size=$range_size duration=$duration ==="

  "$CLIENT_BIN" bench \
    "${VERIFY_ARGS[@]}" \
    "${EXTRA_BENCH_ARGS[@]}" \
    --server-addr "$SERVER_ADDR" \
    --accounts-list "$ACCOUNTS_LIST" \
    --num-clients "$NUM_CLIENTS" \
    --duration "$duration" \
    --range-size "$range_size" \
    --output-dir "$OUTPUT_DIR"

  sleep "$COOLDOWN"
  echo ""
done

echo "=== Sweep complete ==="
echo "Output directory: $OUTPUT_DIR"
