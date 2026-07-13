#!/usr/bin/env bash
# Figure 4a (quick) — versions {2, 16, 128, 256, 2047}.
# HUMAN TIME:   ~5 min
# COMPUTE TIME: ~25-35 min
# OUTPUT:       $KT_OUTPUT_DIR/fig4a_latency.pdf
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPTS_ROOT="$(cd "$HERE/../KeyTransparencyScripts" && pwd)"
export FIGURE=4a
export TEMPLATE=fig4_quick.toml
export CACHE_KEY=fig4_quick
source "$SCRIPTS_ROOT/lib/_run_common.sh"
