#!/bin/bash
#
# Samurai micro-benchmark sweep across multiple shards values with multiple k-users.
#
# Shards: 1, 10, 50, 100, 500, 1000
# k-users:   0, 100000, 200000, 500000, 1000000, 2000000
#
# Self-daemonizes under nohup and writes all output to a timestamped log file.
# Usage: ./scripts/benchmark/run_ingestion_sweep.sh [--dry-run]

set -euo pipefail

# --- Configuration ---
BIN_DIR=${BIN_DIR:-./bin}
ACCOUNTS_50K=${ACCOUNTS_50K:-account_stats_50k.csv}
DURATION=${DURATION:-5m}
N_BLOCKS=${N_BLOCKS:-2616996}
COOLDOWN=${COOLDOWN:-30}  # seconds between runs
BASE_OUTPUT_DIR=${BASE_OUTPUT_DIR:-/data/local/microbench_shards_output}
LOG_DIR=${LOG_DIR:-${BASE_OUTPUT_DIR}}

SHARDS_LIST=(1 10 50 100 500 1000)
K_USERS_LIST=(0 100000 200000 500000 1000000 2000000)

# --- Self-daemonize under nohup ---
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="${LOG_DIR}/microbench_shards_${TIMESTAMP}.log"

if [[ "${_BENCH_RUNNING:-}" != "1" ]]; then
    mkdir -p "$LOG_DIR"
    echo "Starting micro-benchmark sweep."
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

log "=== Micro-Benchmark Sweep ==="
log "Shards:     ${SHARDS_LIST[*]}"
log "k-users:    ${K_USERS_LIST[*]}"
log "Duration:   $DURATION"
log "N blocks:   $N_BLOCKS"
log "Cooldown:   ${COOLDOWN}s"
log ""

# --- samuraimpt (samurai with MPT) ---
log "====== Protocol: samuraimpt ======"
for s in "${SHARDS_LIST[@]}"; do
    OUT_DIR="${BASE_OUTPUT_DIR}/shard${s}"
    mkdir -p "$OUT_DIR"
    for k in "${K_USERS_LIST[@]}"; do
        run_bench "samuraimpt shards=$s k-users=$k" \
            "${BIN_DIR}/samurai" bench-ingest \
                --accounts-list "$ACCOUNTS_50K" \
                --duration "$DURATION" \
                --n "$N_BLOCKS" \
                --shards "$s" \
                --k-users "$k" \
                --output-dir "$OUT_DIR"
    done
done

# --- samurai (skip-mpt) ---
log "====== Protocol: samurai (skip-mpt) ======"
for s in "${SHARDS_LIST[@]}"; do
    OUT_DIR="${BASE_OUTPUT_DIR}/shard${s}"
    mkdir -p "$OUT_DIR"
    for k in "${K_USERS_LIST[@]}"; do
        run_bench "samurai(skip-mpt) shards=$s k-users=$k" \
            "${BIN_DIR}/samurai" bench-ingest \
                --skip-mpt \
                --accounts-list "$ACCOUNTS_50K" \
                --duration "$DURATION" \
                --n "$N_BLOCKS" \
                --shards "$s" \
                --k-users "$k" \
                --output-dir "$OUT_DIR"
    done
done



log "=== Sweep complete ==="
log "Log: $LOG_FILE"
