#!/usr/bin/env bash
# Figure 4b — Monitoring-query throughput vs number of versions.
# Same sweep as run_fig4a.sh; produces the throughput subplot.
#
# HUMAN TIME:   ~10 min
# COMPUTE TIME: ~60-75 min if fresh; ~30s if run_fig4a/4c already completed.
# OUTPUT:       $KT_OUTPUT_DIR/fig4b_throughput.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
export FIGURE=4b
export PROFILE=fig4_full
export CACHE_KEY=fig4_full
source "$HERE/lib/_run_common.sh"
