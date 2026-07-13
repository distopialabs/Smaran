#!/usr/bin/env bash
# Verbose setup verifier — troubleshooting tool and Path-B (own server)
# install check. Runs every setup check with a per-item verdict.
#
# The experiment/plot scripts run the same checks quietly before doing any
# work (require_setup in lib/common.sh — one source of truth), so on the
# happy path you never need this script; reach for it when a script reports
# "Setup incomplete" or after a manual install.
source "$(dirname "$0")/lib/common.sh"
export PATH="$PATH:/usr/local/go/bin"

echo "Smaran artifact setup check"
fails=0
for item in "${SETUP_CHECK_ITEMS[@]}"; do
    if _setup_check "$item"; then
        echo "  ✓ $(_setup_desc "$item")"
    else
        echo "  ✗ $(_setup_desc "$item") — $SETUP_FAIL"
        echo "      Fix: $SETUP_HINT"
        fails=$((fails + 1))
    fi
done
echo
if [ "$fails" -eq 0 ]; then
    echo "Setup complete — all checks passed."
else
    echo "Setup INCOMPLETE — $fails check(s) failed; see fixes above."
    exit 1
fi
