#!/usr/bin/env bash
# Figure 4a — Monitoring-query latency vs number of versions.
# Sweeps versions in {2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2047}
# across protocols {Smaran, Optiks, Coniks}, at 10K users.
#
# HUMAN TIME:   ~10 min (setup + occasional monitoring)
# COMPUTE TIME: ~80 min on r6615 (server) + c6420 (client) CloudLab nodes.
# OUTPUT:       $KT_OUTPUT_DIR/fig4a_latency.pdf   (defaults to <repo>/output/)
#
# Figures 4a, 4b, 4c share a single underlying sweep; if run_fig4b.sh or
# run_fig4c.sh has completed for the same config, this script reuses that
# sweep and only re-plots. Set KT_FORCE_RERUN=1 to always redo the sweep.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
export FIGURE=4a
export TEMPLATE=fig4_full.toml
export CACHE_KEY=fig4_full
source "$HERE/lib/_run_common.sh"
