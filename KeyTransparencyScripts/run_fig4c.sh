#!/usr/bin/env bash
# Figure 4c — Monitoring-query response payload size vs number of versions.
# Same sweep as run_fig4a.sh; produces the payload subplot.
#
# HUMAN TIME:   ~10 min
# COMPUTE TIME: ~80 min if fresh; ~5 s if run_fig4a/4b already completed.
# OUTPUT:       $KT_OUTPUT_DIR/fig4c_payload.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
export FIGURE=4c
export TEMPLATE=fig4_full.toml
export CACHE_KEY=fig4_full
source "$HERE/lib/_run_common.sh"
