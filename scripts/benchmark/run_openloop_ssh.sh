#!/bin/bash
#
# Open-loop throughput sweep via SSH.
#
# For each (range_size, num_clients) combination:
#   1. SSH to remote server, start the proof server with --bench logging
#   2. Run the open-loop client locally
#   3. SSH to stop the server, wait for exit
#   4. SCP the bench CSV back to local machine, clean up remote files
#   5. Cooldown and continue
#
# Usage:
#   PROTOCOL=samurai  bash scripts/benchmark/run_openloop_ssh.sh
#   PROTOCOL=merkle   bash scripts/benchmark/run_openloop_ssh.sh
#   PROTOCOL=verkle   bash scripts/benchmark/run_openloop_ssh.sh
#   PROTOCOL=verklekzg bash scripts/benchmark/run_openloop_ssh.sh

set -euo pipefail

# --- Configuration ---
PROTOCOL=${PROTOCOL:-samurai}
SSH_USER=${SSH_USER:-anepal}
SSH_HOST=${SSH_HOST:-clnode332.clemson.cloudlab.us}
SSH_TARGET="${SSH_USER}@${SSH_HOST}"
SERVER_PORT=${SERVER_PORT:-50051}
REMOTE_BIN_DIR=${REMOTE_BIN_DIR:-/proj/distopialabs-PG0/asim/Samurai/bin}
LOCAL_BIN_DIR=${LOCAL_BIN_DIR:-./bin}
ACCOUNTS_LIST=${ACCOUNTS_LIST:-account_stats_all.csv}
LOCAL_OUTPUT_ROOT=${LOCAL_OUTPUT_ROOT:-/data/local/openloop_benchmark_output}
DURATION=${DURATION:-2m}
RPS_PER_CLIENT=${RPS_PER_CLIENT:-100}
MAX_CONCURRENT=${MAX_CONCURRENT:-100}
STARTUP_WAIT=${STARTUP_WAIT:-30}
COOLDOWN=${COOLDOWN:-30}

RANGE_SIZES=(${RANGE_SIZES:-1 1000 7000})
NUM_CLIENTS_LIST=(${NUM_CLIENTS_LIST:-32 64 128 256})

# --- Protocol-specific settings ---
case "$PROTOCOL" in
  samurai)
    SERVER_BIN=samurai
    CLIENT_BIN=proofc
    REMOTE_DB_DIR=${REMOTE_DB_DIR:-/data/local/final_samurai}
    EXTRA_SERVER_FLAGS=""
    ;;
  merkle)
    SERVER_BIN=merkle
    CLIENT_BIN=merkle-proofc
    REMOTE_DB_DIR=${REMOTE_DB_DIR:-/data/local/merkle}
    EXTRA_SERVER_FLAGS=""
    ;;
  verkle)
    SERVER_BIN=verkle
    CLIENT_BIN=verkle-proofc
    REMOTE_DB_DIR=${REMOTE_DB_DIR:-/data/local/verkle}
    EXTRA_SERVER_FLAGS="--db-backend pebble"
    ;;
  verklekzg)
    SERVER_BIN=verklekzg
    CLIENT_BIN=verklekzg-proofc
    REMOTE_DB_DIR=${REMOTE_DB_DIR:-/data/local/verkle}
    EXTRA_SERVER_FLAGS="--db-backend pebble"
    ;;
  *)
    echo "ERROR: unknown PROTOCOL=$PROTOCOL (expected: samurai, merkle, verkle, verklekzg)" >&2
    exit 1
    ;;
esac

REMOTE_PID_FILE="/tmp/openloop_${PROTOCOL}.pid"
REMOTE_LOG_FILE="/tmp/openloop_${PROTOCOL}.log"

# --- Setup ---
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "=== Open-Loop Throughput Sweep (SSH) ==="
echo "Protocol:        $PROTOCOL"
echo "Timestamp:       $TIMESTAMP"
echo "SSH target:      $SSH_TARGET"
echo "Server binary:   $REMOTE_BIN_DIR/$SERVER_BIN"
echo "Client binary:   $LOCAL_BIN_DIR/$CLIENT_BIN"
echo "Remote DB:       $REMOTE_DB_DIR"
echo "Server port:     $SERVER_PORT"
echo "Accounts list:   $ACCOUNTS_LIST"
echo "Output root:     $LOCAL_OUTPUT_ROOT"
echo "Range sizes:     ${RANGE_SIZES[*]}"
echo "Client counts:   ${NUM_CLIENTS_LIST[*]}"
echo "RPS per client:  $RPS_PER_CLIENT"
echo "Max concurrent:  $MAX_CONCURRENT"
echo "Duration:        $DURATION"
echo "Startup wait:    ${STARTUP_WAIT}s"
echo "Cooldown:        ${COOLDOWN}s"
echo ""

for range_size in "${RANGE_SIZES[@]}"; do
  for num_clients in "${NUM_CLIENTS_LIST[@]}"; do
    offered_rps=$(( num_clients * RPS_PER_CLIENT ))
    echo "=== range=$range_size clients=$num_clients (offered=${offered_rps} rps) ==="

    # Remote bench CSV path.
    REMOTE_CSV="/tmp/openloop_${PROTOCOL}_range${range_size}_clients${num_clients}_${TIMESTAMP}.csv"

    # 1. Start server on remote machine.
    echo "Starting remote server..."
    ssh -n "$SSH_TARGET" "nohup ${REMOTE_BIN_DIR}/${SERVER_BIN} serve \
      --db-dir ${REMOTE_DB_DIR} \
      --port ${SERVER_PORT} \
      ${EXTRA_SERVER_FLAGS} \
      --bench --bench-output ${REMOTE_CSV} \
      > ${REMOTE_LOG_FILE} 2>&1 & echo \$! > ${REMOTE_PID_FILE}"

    echo "Waiting ${STARTUP_WAIT}s for server startup..."
    sleep "$STARTUP_WAIT"

    # 2. Run open-loop client locally.
    echo "Running client..."
    "${LOCAL_BIN_DIR}/${CLIENT_BIN}" openloop \
      --server-addr "${SSH_HOST}:${SERVER_PORT}" \
      --range-size "$range_size" \
      --accounts-list "$ACCOUNTS_LIST" \
      --num-clients "$num_clients" \
      --rps-per-client "$RPS_PER_CLIENT" \
      --max-concurrent "$MAX_CONCURRENT" \
      --duration "$DURATION" || true

    # 3. Stop remote server and wait for exit.
    echo "Stopping remote server..."
    ssh "$SSH_TARGET" "
      PID=\$(cat ${REMOTE_PID_FILE} 2>/dev/null)
      if [ -n \"\$PID\" ]; then
        kill \$PID 2>/dev/null || true
        for i in \$(seq 1 30); do
          kill -0 \$PID 2>/dev/null || break
          sleep 1
        done
        kill -9 \$PID 2>/dev/null || true
      fi
    " || true

    # 4. Check if server was OOM-killed.
    OOM_CHECK=$(ssh "$SSH_TARGET" "dmesg -T 2>/dev/null | grep -i 'oom.*${SERVER_BIN}\|killed process.*${SERVER_BIN}' | tail -5" 2>/dev/null || true)
    if [ -n "$OOM_CHECK" ]; then
      echo "WARNING: Server may have been OOM-killed:"
      echo "$OOM_CHECK"
    fi

    # 5. Copy bench CSV and server log back, then clean up remote files.
    LOCAL_DIR="${LOCAL_OUTPUT_ROOT}/numclients${num_clients}/${PROTOCOL}"
    mkdir -p "$LOCAL_DIR"
    LOCAL_LOG="${LOCAL_DIR}/openloop_${PROTOCOL}_range${range_size}_clients${num_clients}_${TIMESTAMP}.log"
    scp "${SSH_TARGET}:${REMOTE_CSV}" "${LOCAL_DIR}/" 2>/dev/null || echo "WARNING: could not copy bench CSV"
    scp "${SSH_TARGET}:${REMOTE_LOG_FILE}" "${LOCAL_LOG}" 2>/dev/null || echo "WARNING: could not copy server log"
    ssh "$SSH_TARGET" "rm -f ${REMOTE_CSV} ${REMOTE_LOG_FILE} ${REMOTE_PID_FILE}" 2>/dev/null || true

    echo "Local CSV: ${LOCAL_DIR}/$(basename ${REMOTE_CSV})"
    echo "Local log: ${LOCAL_LOG}"

    # 5. Cooldown.
    echo "Cooling down for ${COOLDOWN}s..."
    sleep "$COOLDOWN"
    echo ""
  done
done

echo "=== Sweep complete ==="
echo "Output root: $LOCAL_OUTPUT_ROOT"
