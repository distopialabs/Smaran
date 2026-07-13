#!/bin/bash
#
# Open-loop throughput sweep for Samurai proof server.
#
# For each (range_size, num_clients) combination:
#   1. Start the server with --bench logging to a timestamped CSV
#   2. Run the open-loop client for DURATION
#   3. Stop the server and cooldown
#
# The server-side CSVs contain per-request timestamps for throughput analysis.
# The client prints a summary (sent, completed, dropped, errors) to stdout.

set -euo pipefail

# --- Configuration ---
SERVER_BIN=${SERVER_BIN:-./bin/samurai}
CLIENT_BIN=${CLIENT_BIN:-./bin/proofc}
DB_DIR=${DB_DIR:-/data/local/samurai_db}
ACCOUNTS_LIST=${ACCOUNTS_LIST:-./cmd/proofc/top_1k_accounts_200k_blocks.csv}
OUTPUT_DIR=${OUTPUT_DIR:-/data/local/benchmark_output/samuraimpt}
DURATION=${DURATION:-60s}
RPS_PER_CLIENT=${RPS_PER_CLIENT:-10}
MAX_CONCURRENT=${MAX_CONCURRENT:-100}
SERVER_PORT=${SERVER_PORT:-50051}

RANGE_SIZES=(${RANGE_SIZES:-1000 5000 10000 50000})
NUM_CLIENTS_LIST=(${NUM_CLIENTS_LIST:-1 16 32})

# --- Setup ---
mkdir -p "$OUTPUT_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "=== Open-Loop Throughput Sweep ==="
echo "Timestamp:       $TIMESTAMP"
echo "Range sizes:     ${RANGE_SIZES[*]}"
echo "Client counts:   ${NUM_CLIENTS_LIST[*]}"
echo "RPS per client:  $RPS_PER_CLIENT"
echo "Max concurrent:  $MAX_CONCURRENT"
echo "Duration:        $DURATION"
echo ""

for range_size in "${RANGE_SIZES[@]}"; do
  for num_clients in "${NUM_CLIENTS_LIST[@]}"; do
    bench_csv="${OUTPUT_DIR}/openloop_range${range_size}_clients${num_clients}_${TIMESTAMP}.csv"

    echo "=== range=$range_size clients=$num_clients (offered=$(( num_clients * RPS_PER_CLIENT )) rps) ==="

    # Start server with bench logging.
    $SERVER_BIN serve \
      --db-dir "$DB_DIR" \
      --port "$SERVER_PORT" \
      --bench \
      --bench-output "$bench_csv" &
    SERVER_PID=$!
    sleep 5  # wait for server startup

    # Run open-loop client.
    $CLIENT_BIN openloop \
      --server-addr "localhost:${SERVER_PORT}" \
      --range-size "$range_size" \
      --accounts-list "$ACCOUNTS_LIST" \
      --num-clients "$num_clients" \
      --rps-per-client "$RPS_PER_CLIENT" \
      --max-concurrent "$MAX_CONCURRENT" \
      --duration "$DURATION"

    # Stop server gracefully.
    kill "$SERVER_PID" 2>/dev/null
    wait "$SERVER_PID" 2>/dev/null || true
    sleep 5  # cooldown between runs

    echo "Server CSV: $bench_csv"
    echo ""
  done
done

echo "=== Sweep complete ==="
echo "Output directory: $OUTPUT_DIR"
