#!/usr/bin/env bash
# Figure 7a: commitment generation (ingestion) throughput across protocols
# and user counts (Merkle, Verkle, Smaran; Cauchy series from prebaked logs).
#
# Each data point ingests a fixed number of blocks from a fresh temporary
# database (block-limited, as in the paper; the timeout is only a deadline).
# Parameters and per-tier defaults: ../config.sh. Time estimates: README.
source "$(dirname "$0")/lib/experiments.sh"
echo "Running experiment Figure 7a"

LOGS="$RESULTS_DIR/fig7a/logs"
OUT="$RESULTS_DIR/fig7a"

for proto in merkle verkle smaran; do
    for k in "${K_USERS_LIST[@]}"; do
        echo "Running $(proto_label "$proto") with $k users"
        run_ingest_bench "$proto" "$k" "$LOGS" "$INGEST_BENCH_BLOCKS" "$INGEST_BENCH_TIMEOUT"
    done
done

PROTOCOL_ARGS=(--protocol samurai --protocol merkle --protocol verkle)
if stage_cauchy fig7a "$LOGS/cauchy"; then
    PROTOCOL_ARGS+=(--protocol cauchy)
fi

echo "Plotting"
(cd "$REPO_ROOT" && python3 scripts/paper-figures/fig7a_ingestion_throughput.py \
    --input-root "$LOGS" "${PROTOCOL_ARGS[@]}" --output-dir "$OUT")
echo "Figure 7a: $OUT/fig7a_ingestion_throughput.pdf"
