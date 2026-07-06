#!/usr/bin/env bash
# Shared runner used by run_fig*.sh scripts. Not called directly.
#
# Required env from caller: FIGURE (4a/4b/4c/5), PROFILE (fig4_full/fig4_quick/
# fig5_full/fig5_quick), CACHE_KEY.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_ROOT="$(cd "$HERE/.." && pwd)"
REPO_ROOT="$(cd "$SCRIPTS_ROOT/.." && pwd)"

# Optional per-user overrides.
NODES_ENV="${NODES_ENV:-$SCRIPTS_ROOT/nodes.env}"
if [ -f "$NODES_ENV" ]; then
  # shellcheck disable=SC1090
  source "$NODES_ENV"
fi

# Ensure python deps present for kt.py + plotters.
python3 -c "import tomli, matplotlib, seaborn, pandas" 2>/dev/null || {
  echo "  installing python dependencies (one-time)"
  python3 -m pip install --quiet --user -r "$REPO_ROOT/experiments/requirements.txt"
}

TMPCFG="$(mktemp --suffix=.toml)"
trap 'rm -f "$TMPCFG"' EXIT

python3 "$SCRIPTS_ROOT/lib/render_config.py" --profile "$PROFILE" --out "$TMPCFG"

OUTPUT_DIR="${KT_OUTPUT_DIR:-$REPO_ROOT/output}"

cd "$REPO_ROOT"
exec python3 "$SCRIPTS_ROOT/lib/ae_driver.py" \
  --figure "$FIGURE" \
  --config "$TMPCFG" \
  --output-dir "$OUTPUT_DIR" \
  --cache-key "$CACHE_KEY"
