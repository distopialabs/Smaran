#!/usr/bin/env bash
# Figure 5 (quick) — Key-update throughput vs users, reduced sweep.
# Users: {10k, 200k, 1M}. Captures the low/mid/high trend.
#
# HUMAN TIME:   ~10 min
# COMPUTE TIME: ~40-60 min (3 user counts x 3 protocols x ~90s + build).
#               The 1M-user point dominates.
# OUTPUT:       $KT_OUTPUT_DIR/fig5_put_throughput.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPTS_ROOT="$(cd "$HERE/../KeyTransparencyScripts" && pwd)"
export FIGURE=5
export PROFILE=fig5_quick
export CACHE_KEY=fig5_quick
source "$SCRIPTS_ROOT/lib/_run_common.sh"
