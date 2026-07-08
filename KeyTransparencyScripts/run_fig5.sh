#!/usr/bin/env bash
# Figure 5 — Key-update throughput as directory size scales.
# Sweeps users in {10k, 50k, 100k, 200k, 500k, 1M} across
# protocols {Smaran, Optiks, Coniks}.
#
# HUMAN TIME:   ~15 min
# COMPUTE TIME: ~90-120 min on r6615 (server) + c6420 (client) CloudLab nodes.
#               Larger-user points dominate wall-clock.
# OUTPUT:       $KT_OUTPUT_DIR/fig5_put_throughput.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
export FIGURE=5
export TEMPLATE=fig5_full.toml
export CACHE_KEY=fig5_full
source "$HERE/lib/_run_common.sh"
