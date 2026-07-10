#!/usr/bin/env bash
# Smaran AE — one-command laptop-side driver.
# Reviewer runs this on their LAPTOP after instantiating the CloudLab profile.
#
# Usage:  bash run_ae.sh <cloudlab-username> <node0-hostname> [quick|full]
# Default is 'full' (~3 h). Pass 'quick' for the reduced sweep (~90 min).
#
# What it does:
#   1) SSH into node0
#   2) Clone the artifact-eval branch and set up inter-node SSH
#   3) Run all four figures
#   4) scp PDFs back to ~/Desktop/smaran-ae-output/ on your laptop
set -euo pipefail

if [ $# -lt 2 ]; then
  echo "Usage: $0 <cloudlab-username> <node0-hostname> [quick|full]"
  echo "Example: $0 alice clnode123.clemson.cloudlab.us full"
  exit 2
fi

USER=$1
HOST=$2
MODE=${3:-full}
OUT="${HOME}/Desktop/smaran-ae-output"

case "$MODE" in
  quick) ETA='~90 minutes' ;;
  full)  ETA='~3 hours' ;;
  *) echo 'MODE must be "quick" or "full"'; exit 2 ;;
esac

echo '==========================================================='
echo "  Smaran Artifact Evaluation - $MODE sweep"
echo "  Target: ${USER}@${HOST}"
echo "  Estimated wall-clock: $ETA"
echo "  Output: ${OUT}"
echo '==========================================================='

echo '[1/4] Checking SSH...'
ssh -o StrictHostKeyChecking=accept-new "${USER}@${HOST}" 'hostname' \
  || { echo 'SSH failed'; exit 1; }

echo '[2/4] Setting up node0...'
ssh "${USER}@${HOST}" "MODE='$MODE' bash -s" <<'REMOTE'
set -euo pipefail
if [ ! -d ~/Smaran/.git ]; then
  git clone --branch artifact-eval --recurse-submodules \
      https://github.com/distopialabs/Smaran.git ~/Smaran
fi
cd ~/Smaran
[ -f ~/.ssh/id_ed25519 ] || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519
grep -qxF "$(cat ~/.ssh/id_ed25519.pub)" ~/.ssh/authorized_keys 2>/dev/null \
  || cat ~/.ssh/id_ed25519.pub >> ~/.ssh/authorized_keys
cat ~/.ssh/id_ed25519.pub \
  | ssh -o StrictHostKeyChecking=accept-new node1 \
      "grep -qxF \"\$(cat)\" ~/.ssh/authorized_keys || cat >> ~/.ssh/authorized_keys"
ssh-keyscan -H node0 node1 >> ~/.ssh/known_hosts 2>/dev/null || true
[ -e /usr/local/bin/go ] || sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
[ -f KeyTransparencyScripts/nodes.env ] || cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env
sudo pkill -9 coniksserver ktserver 2>/dev/null || true
sudo rm -f /tmp/coniks.sock || true
ssh -o StrictHostKeyChecking=no node1 'sudo pkill -9 -f coniks 2>/dev/null; sudo rm -f /tmp/coniks.sock' || true
echo 'setup complete'
REMOTE

echo "[3/4] Running $MODE experiments on node0..."
if [ "$MODE" = 'quick' ]; then
  SCRIPTS='./QuickTesting-KeyTransparency/run_fig4a_quick.sh ./QuickTesting-KeyTransparency/run_fig4b_quick.sh ./QuickTesting-KeyTransparency/run_fig4c_quick.sh ./QuickTesting-KeyTransparency/run_fig5_quick.sh'
else
  SCRIPTS='./KeyTransparencyScripts/run_fig4a.sh ./KeyTransparencyScripts/run_fig4b.sh ./KeyTransparencyScripts/run_fig4c.sh ./KeyTransparencyScripts/run_fig5.sh'
fi
ssh "${USER}@${HOST}" "cd ~/Smaran && for s in $SCRIPTS; do echo === \$s ===; \$s || { echo \"\$s failed\"; exit 1; }; done"

echo "[4/4] Copying PDFs to $OUT ..."
mkdir -p "$OUT"
scp "${USER}@${HOST}:~/Smaran/output/*.pdf" "$OUT/"

echo ''
echo '==========================================================='
echo '  Done. Compare against paper Figures 4a/4b/4c/5.'
echo "  PDFs: $OUT"
echo "  Reference PDFs to compare against: reference_pdfs/ in the repo"
echo '==========================================================='
ls -la "$OUT/"
