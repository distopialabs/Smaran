#!/usr/bin/env bash
# Figure 7b: impact of archival (root) storage on Smaran query latency —
# optimus (stored roots) vs non_optimus (tree rebuilt per query).
#
# Runs the Smaran query benchmark twice against the same ingested database:
# once with stored roots (optimus) and once with the --old client path
# (non_optimus). The server is started and stopped for each leg. Estimated
# time: see README — the ingest is cached and shared with Figure 6.
source "$(dirname "$0")/lib/experiments.sh"
echo "Running experiment Figure 7b"

LOGS="$RESULTS_DIR/fig7b/logs"
OUT="$RESULTS_DIR/fig7b"

# The plot reads every proof_range*.csv under $LOGS; logs from an earlier or
# aborted run (or different RANGES_7B) must not leak into this one.
rm -rf "$LOGS"

run_proof_sweep smaran "$LOGS/raw/optimus" optimus "${RANGES_7B[@]}"
run_proof_sweep smaran "$LOGS/raw/non_optimus" non_optimus "${RANGES_7B[@]}"

# fig7b_archival_storage.py expects flat optimus/ + non_optimus/ dirs; the
# benchmark writes into a samuraimpt/ subdirectory of each leg.
for leg in optimus non_optimus; do
    mkdir -p "$LOGS/$leg"
    cp "$LOGS/raw/$leg/samuraimpt/"proof_range*.csv "$LOGS/$leg/"
done

echo "Plotting"
(cd "$REPO_ROOT" && python3 scripts/paper-figures/fig7b_archival_storage.py "$LOGS" --output "$OUT")
echo "Figure 7b: $OUT/fig7b_archival_storage.pdf"
