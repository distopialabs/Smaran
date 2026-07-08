#!/usr/bin/env bash
# One-shot Artifact-Evaluation runner. Reviewer just SSHes into node0 and runs:
#   bash /opt/Smaran/KeyTransparencyScripts/run_all.sh
# Total wall-clock: ~90 min (mostly unattended).
set -euo pipefail

echo "==========================================================="
echo "  Smaran Artifact Evaluation - Key Transparency (§7.1)"
echo "  Figures 4a, 4b, 4c, and 5"
echo "  Expected total wall-clock: ~90 minutes"
echo "==========================================================="

REPO_SRC=/opt/Smaran
REPO_DST="$HOME/Smaran"

# Step 1: bring the pre-installed repo into your writable home directory
if [ ! -d "$REPO_DST" ]; then
  echo "[setup] Copying $REPO_SRC to $REPO_DST"
  cp -r "$REPO_SRC" "$REPO_DST"
fi
cd "$REPO_DST"

# Step 2: make sure /usr/local/bin/go exists (for non-interactive SSH sessions)
if [ ! -e /usr/local/bin/go ] && [ -x /usr/local/go/bin/go ]; then
  sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
fi

# Step 3: RSA key for kt.py's paramiko + inter-node SSH
if [ ! -f "$HOME/.ssh/id_rsa" ]; then
  echo "[setup] Generating RSA SSH key"
  ssh-keygen -t rsa -b 4096 -N '' -f "$HOME/.ssh/id_rsa" -q
fi
chmod 600 "$HOME/.ssh/id_rsa"
if ! grep -qxF "$(cat "$HOME/.ssh/id_rsa.pub")" "$HOME/.ssh/authorized_keys" 2>/dev/null; then
  cat "$HOME/.ssh/id_rsa.pub" >> "$HOME/.ssh/authorized_keys"
fi
ssh-keyscan -H node0 node1 >> "$HOME/.ssh/known_hosts" 2>/dev/null || true

# Step 4: nodes.env
[ -f KeyTransparencyScripts/nodes.env ] \
    || cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env

echo
echo "==========================================================="
echo "  Setup complete. Starting experiments."
echo "==========================================================="

./QuickTesting-KeyTransparency/run_fig4a_quick.sh
./QuickTesting-KeyTransparency/run_fig4b_quick.sh
./QuickTesting-KeyTransparency/run_fig4c_quick.sh
./QuickTesting-KeyTransparency/run_fig5_quick.sh

echo
echo "==========================================================="
echo "  All experiments complete."
echo "==========================================================="
echo "PDFs are in $REPO_DST/output/ :"
ls -la "$REPO_DST/output/"
echo
echo "To copy them to your laptop, from your laptop terminal:"
echo "  scp $USER@$(hostname -f):$REPO_DST/output/*.pdf ~/Desktop/"
