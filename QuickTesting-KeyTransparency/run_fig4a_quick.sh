#!/usr/bin/env bash
# Figure 4a (quick) — Monitoring-query latency vs versions, reduced sweep.
# Versions: {2, 16, 128, 256, 2047}. Enough to see the trend.
#
# HUMAN TIME:   ~5 min
# COMPUTE TIME: ~25-35 min (5 versions x 3 protocols x ~90s + build)
# OUTPUT:       $KT_OUTPUT_DIR/fig4a_latency.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPTS_ROOT="$(cd "$HERE/../KeyTransparencyScripts" && pwd)"
export FIGURE=4a
export PROFILE=fig4_quick
export CACHE_KEY=fig4_quick
source "$SCRIPTS_ROOT/lib/_run_common.sh"
