#!/usr/bin/env bash
# Figure 7c: impact of sharding on Smaran ingestion throughput (shard-count
# sweep x user counts; Smaran only).
#
# Each data point runs a duration-bounded ingestion benchmark from a fresh
# temporary database with the given shard count (duration-limited, as in the
# paper). Parameters and per-tier defaults: ../config.sh. Time estimates:
# README.
source "$(dirname "$0")/lib/experiments.sh"

echo "Running experiment Figure 7c"

LOGS="$RESULTS_DIR/fig7c/logs"
OUT="$RESULTS_DIR/fig7c"

for shards in "${SHARDS_LIST[@]}"; do
    for k in "${K_USERS_LIST[@]}"; do
        echo "Running Smaran with $shards shards ($k users)"
        run_ingest_bench smaran "$k" "$LOGS/shard${shards}" \
            "$SHARDS_BENCH_BLOCKS" "$SHARDS_BENCH_DURATION" "$shards"
    done
done

echo "Plotting"
(cd "$REPO_ROOT" && python3 scripts/paper-figures/fig7c_shards.py \
    --shards-root "$LOGS" --output-dir "$OUT")
echo "Figure 7c: $OUT/fig7c_shards_throughput.pdf"
