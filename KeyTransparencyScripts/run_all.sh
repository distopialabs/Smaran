#!/usr/bin/env bash
# Executes on node0 after the reviewer's SSH lands.
# All setup + all four experiments in one shot.
set -euo pipefail

echo "==========================================================="
echo "  Smaran Artifact Evaluation - Key Transparency (§7.1)"
echo "  Figures 4a, 4b, 4c, 5   |   ~90 min wall-clock"
echo "==========================================================="

REPO_SRC=/opt/Smaran
REPO_DST="$HOME/Smaran"

# Always sync from the pre-installed /opt copy (idempotent, reruns are safe)
mkdir -p "$REPO_DST"
rsync -a --delete --exclude="logs/" --exclude="output/" "$REPO_SRC/" "$REPO_DST/"
cd "$REPO_DST"

[ -e /usr/local/bin/go ] || sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go

if [ ! -f "$HOME/.ssh/id_rsa" ]; then
  ssh-keygen -t rsa -b 4096 -N '' -f "$HOME/.ssh/id_rsa" -q
fi
chmod 600 "$HOME/.ssh/id_rsa"
grep -qxF "$(cat "$HOME/.ssh/id_rsa.pub")" "$HOME/.ssh/authorized_keys" 2>/dev/null \
  || cat "$HOME/.ssh/id_rsa.pub" >> "$HOME/.ssh/authorized_keys"
ssh-keyscan -H node0 node1 >> "$HOME/.ssh/known_hosts" 2>/dev/null || true

[ -f KeyTransparencyScripts/nodes.env ] \
  || cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env

./QuickTesting-KeyTransparency/run_fig4a_quick.sh
./QuickTesting-KeyTransparency/run_fig4b_quick.sh
./QuickTesting-KeyTransparency/run_fig4c_quick.sh
./QuickTesting-KeyTransparency/run_fig5_quick.sh

echo ""
echo "==========================================================="
echo "  All experiments complete."
echo "==========================================================="
ls -la "$REPO_DST/output/"
