#!/usr/bin/env bash
# Figure 4b (quick) — Monitoring-query throughput vs versions, reduced sweep.
# Versions: {2, 16, 128, 256, 2047}.
#
# HUMAN TIME:   ~5 min
# COMPUTE TIME: ~25-35 min if fresh; near-instant if run_fig4a_quick already ran.
# OUTPUT:       $KT_OUTPUT_DIR/fig4b_throughput.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPTS_ROOT="$(cd "$HERE/../KeyTransparencyScripts" && pwd)"
export FIGURE=4b
export PROFILE=fig4_quick
export CACHE_KEY=fig4_quick
source "$SCRIPTS_ROOT/lib/_run_common.sh"
