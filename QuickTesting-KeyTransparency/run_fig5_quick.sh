#!/usr/bin/env bash
# Figure 5 (quick) — users {10k, 200k, 1M}.
# HUMAN TIME:   ~10 min
# COMPUTE TIME: ~40-60 min. The 1M-user point dominates.
# OUTPUT:       $KT_OUTPUT_DIR/fig5_put_throughput.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPTS_ROOT="$(cd "$HERE/../KeyTransparencyScripts" && pwd)"
export FIGURE=5
export TEMPLATE=fig5_quick.toml
export CACHE_KEY=fig5_quick
source "$SCRIPTS_ROOT/lib/_run_common.sh"
