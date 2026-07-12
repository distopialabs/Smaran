#!/usr/bin/env bash
# Kick-the-tires (plot-only) mode: regenerate all six paper figures from the
# curated benchmark logs that produced the figures in the paper — no benchmarks
# are run. Exercises the full plotting toolchain (Python + LaTeX) in ~5 minutes.
#
# The logs live in the SmaranEthereumDataset CloudLab dataset (not in git); see
# lib/common.sh resolve_paper_logs for the lookup order (override with
# SMARAN_PAPER_LOGS=<dir>).
#
# Output: results/paper-figures/fig{6a,6b,6c,7a,7b,7c}*.pdf
#
# Estimated time: ~5 minutes (log staging + LaTeX rendering). Requires an
# install script to have been run (for the plot dependencies).

source "$(dirname "$0")/lib/common.sh"

# Plot-only mode needs no block dataset, binaries, or /data/local — just the
# plotting toolchain and the prebaked logs.
require_setup plot-deps paper-logs

echo "Regenerating all paper figures from prebaked logs"

BUNDLE="$(resolve_paper_logs)"
STAGE="$RESULTS_DIR/paper-figures-staging"
OUT="$RESULTS_DIR/paper-figures"
mkdir -p "$OUT"

say "Staging logs from $BUNDLE (decompressing ingestion CSVs)"
stage_paper_logs "$BUNDLE" "$STAGE"

cd "$REPO_ROOT"

echo "Plotting"

say "Figure 7a (commitment generation throughput)"
python3 scripts/paper-figures/fig7a_ingestion_throughput.py \
    --input-root "$STAGE/fig7a" \
    --protocol samurai --protocol merkle --protocol verkle --protocol cauchy \
    --output-dir "$OUT"

say "Figure 7c (impact of sharding)"
python3 scripts/paper-figures/fig7c_shards.py \
    --shards-root "$STAGE/fig7c" --output-dir "$OUT"

say "Figures 6a/6b/6c (query latency / throughput / payload size)"
python3 scripts/paper-figures/fig6_query.py "$STAGE/fig6" --output "$OUT"

say "Figure 7b (impact of archival storage)"
python3 scripts/paper-figures/fig7b_archival_storage.py "$STAGE/fig7b" --output "$OUT"

echo
echo "Done. Figures written to $OUT:"
find "$OUT" -name '*.pdf' | sort | sed 's/^/  /'
echo "Compare against Figures 6a-6c and 7a-7c in the paper."
