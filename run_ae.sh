#!/usr/bin/env bash
# Smaran AE — one-command laptop-side driver.
# Reviewer runs this on their LAPTOP after instantiating the CloudLab profile.
#
# Usage:  bash run_ae.sh <cloudlab-username> <node0-hostname> [kt|dl] [quick|full]
# Defaults: usecase 'kt', mode 'full'.
#
# What it does:
#   1) SSH into node0
#   2) Ensure the repo is present (the profile clones it to /local/repository)
#      and inter-node SSH works
#   3) Run every figure for the chosen usecase
#   4) scp PDFs back to ~/Desktop/smaran-ae-output/ on your laptop
set -euo pipefail

if [ $# -lt 2 ]; then
  echo "Usage: $0 <cloudlab-username> <node0-hostname> [kt|dl] [quick|full]"
  echo "Example: $0 alice clnode123.clemson.cloudlab.us kt full"
  exit 2
fi

USER=$1
HOST=$2
USECASE=${3:-kt}
MODE=${4:-full}
OUT="${HOME}/Desktop/smaran-ae-output"
BRANCH=unified-artifact

case "$USECASE" in
  kt)
    case "$MODE" in
      quick) ETA='~90 minutes' ;;
      full)  ETA='~3 hours' ;;
      *) echo 'MODE must be "quick" or "full"'; exit 2 ;;
    esac ;;
  dl)
    case "$MODE" in
      quick) ETA='~50 minutes' ;;
      full)  ETA='tens of hours (full-scale ingest; see README)' ;;
      *) echo 'MODE must be "quick" or "full"'; exit 2 ;;
    esac ;;
  *) echo 'USECASE must be "kt" or "dl"'; exit 2 ;;
esac

echo '==========================================================='
echo "  Smaran Artifact Evaluation - $USECASE $MODE sweep"
echo "  Target: ${USER}@${HOST}"
echo "  Estimated wall-clock: $ETA"
echo "  Output: ${OUT}"
echo '==========================================================='

echo '[1/4] Checking SSH...'
ssh -o StrictHostKeyChecking=accept-new "${USER}@${HOST}" 'hostname' \
  || { echo 'SSH failed'; exit 1; }

echo '[2/4] Setting up node0...'
ssh "${USER}@${HOST}" "BRANCH='$BRANCH' bash -s" <<'REMOTE'
set -euo pipefail
# The CloudLab profile clones the repo to /local/repository at boot; fall
# back to a home-dir clone when running against a bare machine.
if [ -d /local/repository/.git ]; then
  REPO=/local/repository
else
  REPO="$HOME/Smaran"
  [ -d "$REPO/.git" ] || git clone --branch "$BRANCH" \
      https://github.com/distopialabs/Smaran.git "$REPO"
fi
cd "$REPO"
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
echo "setup complete in $REPO"
REMOTE

REPO_EXPR='$( [ -d /local/repository/.git ] && echo /local/repository || echo ~/Smaran )'

echo "[3/4] Running $USECASE $MODE experiments on node0..."
if [ "$USECASE" = 'kt' ]; then
  if [ "$MODE" = 'quick' ]; then
    SCRIPTS='./QuickTesting-KeyTransparency/run_fig4a_quick.sh ./QuickTesting-KeyTransparency/run_fig4b_quick.sh ./QuickTesting-KeyTransparency/run_fig4c_quick.sh ./QuickTesting-KeyTransparency/run_fig5_quick.sh'
  else
    SCRIPTS='./KeyTransparencyScripts/run_fig4a.sh ./KeyTransparencyScripts/run_fig4b.sh ./KeyTransparencyScripts/run_fig4c.sh ./KeyTransparencyScripts/run_fig5.sh'
  fi
  PDF_GLOB='output/*.pdf'
else
  if [ "$MODE" = 'quick' ]; then
    SCRIPTS='./QuickTesting-DecentralizedLedgerScripts/run_fig6a.sh ./QuickTesting-DecentralizedLedgerScripts/run_fig6b.sh ./QuickTesting-DecentralizedLedgerScripts/run_fig6c.sh ./QuickTesting-DecentralizedLedgerScripts/run_fig7a.sh ./QuickTesting-DecentralizedLedgerScripts/run_fig7b.sh ./QuickTesting-DecentralizedLedgerScripts/run_fig7c.sh'
  else
    SCRIPTS='./DecentralizedLedgerScripts/run_fig6a.sh ./DecentralizedLedgerScripts/run_fig6b.sh ./DecentralizedLedgerScripts/run_fig6c.sh ./DecentralizedLedgerScripts/run_fig7a.sh ./DecentralizedLedgerScripts/run_fig7b.sh ./DecentralizedLedgerScripts/run_fig7c.sh'
  fi
  PDF_GLOB='results/fig*/*.pdf'
fi
ssh "${USER}@${HOST}" "cd $REPO_EXPR && for s in $SCRIPTS; do echo === \$s ===; \$s || { echo \"\$s failed\"; exit 1; }; done"

echo "[4/4] Copying PDFs to $OUT ..."
mkdir -p "$OUT"
scp "${USER}@${HOST}:$REPO_EXPR/$PDF_GLOB" "$OUT/"

echo ''
echo '==========================================================='
if [ "$USECASE" = 'kt' ]; then
  echo '  Done. Compare against paper Figures 4a/4b/4c/5.'
  echo '  Reference PDFs to compare against: reference_pdfs/ in the repo'
else
  echo '  Done. Compare against paper Figures 6a/6b/6c/7a/7b/7c.'
  echo '  (Tier 0 regenerates the paper versions: ./DecentralizedLedgerScripts/plot_paper_figures.sh)'
fi
echo "  PDFs: $OUT"
echo '==========================================================='
ls -la "$OUT/"
