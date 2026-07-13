#!/bin/bash
#
# Ingestion benchmark sweep across 4 protocols and multiple k-users values.
#
# Protocols: samuraimpt, samurai (skip-mpt), merkle, verklekzg
# k-users:   0, 100000, 200000, 500000, 1000000, 2000000
#
# Self-daemonizes under nohup and writes all output to a timestamped log file.
# Usage: ./scripts/benchmark/run_ingestion_sweep.sh [--dry-run]

set -euo pipefail

# --- Configuration ---
BIN_DIR=${BIN_DIR:-./bin}
ACCOUNTS_50K=${ACCOUNTS_50K:-account_stats_50k.csv}
ACCOUNTS_ALL=${ACCOUNTS_ALL:-account_stats_all.csv}
DURATION=${DURATION:-15m}
N_BLOCKS=${N_BLOCKS:-50000}
COOLDOWN=${COOLDOWN:-30}  # seconds between runs
LOG_DIR=${LOG_DIR:-/data/local/benchmark_output}

K_USERS_LIST=(0 100000 200000 500000 1000000 2000000)

# --- Self-daemonize under nohup ---
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="${LOG_DIR}/ingestion_bench_${TIMESTAMP}.log"

if [[ "${_BENCH_RUNNING:-}" != "1" ]]; then
    mkdir -p "$LOG_DIR"
    echo "Starting ingestion benchmark sweep."
    echo "Log: $LOG_FILE"
    _BENCH_RUNNING=1 nohup "$0" "$@" >> "$LOG_FILE" 2>&1 &
    echo "PID: $!"
    exit 0
fi

# --- From here on, running in background ---

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

run_bench() {
    local label="$1"; shift
    if [[ "${DRY_RUN:-}" == "1" ]]; then
        log "[dry-run] $*"
    else
        log "START: $label"
        "$@"
        log "DONE:  $label"
    fi
    log "Cooling down for ${COOLDOWN}s..."
    sleep "$COOLDOWN"
}

DRY_RUN=0
for arg in "$@"; do
    [[ "$arg" == "--dry-run" ]] && DRY_RUN=1
done

log "=== Ingestion Benchmark Sweep ==="
log "Protocols:  samuraimpt, samurai, merkle, verklekzg"
log "k-users:    ${K_USERS_LIST[*]}"
log "Duration:   $DURATION"
log "N blocks:   $N_BLOCKS"
log "Cooldown:   ${COOLDOWN}s"
log ""

# --- samuraimpt (samurai with MPT) ---
log "====== Protocol: samuraimpt ======"
for k in "${K_USERS_LIST[@]}"; do
    run_bench "samuraimpt k-users=$k" \
        "${BIN_DIR}/samurai" bench-ingest \
            --accounts-list "$ACCOUNTS_50K" \
            --duration "$DURATION" \
            --n "$N_BLOCKS" \
            --k-users "$k"
done

# --- samurai (skip-mpt) ---
# log "====== Protocol: samurai (skip-mpt) ======"
# for k in "${K_USERS_LIST[@]}"; do
#     run_bench "samurai(skip-mpt) k-users=$k" \
#         "${BIN_DIR}/samurai" bench-ingest \
#             --skip-mpt \
#             --accounts-list "$ACCOUNTS_50K" \
#             --duration "$DURATION" \
#             --n "$N_BLOCKS" \
#             --k-users "$k"
# done

# --- merkle ---
log "====== Protocol: merkle ======"
for k in "${K_USERS_LIST[@]}"; do
    run_bench "merkle k-users=$k" \
        "${BIN_DIR}/merkle" bench-ingest \
            --accounts-list "$ACCOUNTS_50K" \
            --duration "$DURATION" \
            --n "$N_BLOCKS" \
            --k-users "$k"
done

# --- verklekzg ---
# log "====== Protocol: verklekzg ======"
# for k in "${K_USERS_LIST[@]}"; do
#     run_bench "verklekzg k-users=$k" \
#         "${BIN_DIR}/verklekzg" bench-ingest \
#             --accounts-list "$ACCOUNTS_50K" \
#             --duration "$DURATION" \
#             --n "$N_BLOCKS" \
#             --k-users "$k"
# done

# --- verkle ---
# log "====== Protocol: verkle ======"
# for k in "${K_USERS_LIST[@]}"; do
#     run_bench "verkle k-users=$k" \
#         "${BIN_DIR}/verkle" bench-ingest \
#             --accounts-list "$ACCOUNTS_50K" \
#             --duration "$DURATION" \
#             --n "$N_BLOCKS" \
#             --k-users "$k"
# done

log "=== Sweep complete ==="
log "Log: $LOG_FILE"
