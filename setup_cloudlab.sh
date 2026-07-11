#!/usr/bin/env bash
# One-shot setup: verify environment, then ask which experiment to run.
# Run this once on node0 after your CloudLab experiment is Ready.
set -euo pipefail

REPO="$(cd "$(dirname "$0")" && pwd)"

echo
echo '======================================================='
echo '  Smaran Artifact Evaluation - CloudLab setup'
echo '======================================================='
echo

# 1. Ensure repo on KT-artifact / kt branch (whichever is public-facing)
cd "$REPO"
echo '[1/4] Repo:' "$REPO" '(' "$(git branch --show-current)" '@' "$(git log -1 --format=%h)" ')'

# 2. Inter-node SSH
echo '[2/4] Inter-node SSH ...'
[ -f "$HOME/.ssh/id_ed25519" ] || ssh-keygen -t ed25519 -N '' -f "$HOME/.ssh/id_ed25519" -q
grep -qxF "$(cat "$HOME/.ssh/id_ed25519.pub")" "$HOME/.ssh/authorized_keys" 2>/dev/null \
  || cat "$HOME/.ssh/id_ed25519.pub" >> "$HOME/.ssh/authorized_keys"
cat "$HOME/.ssh/id_ed25519.pub" | ssh -o StrictHostKeyChecking=accept-new -o LogLevel=ERROR node1 \
  'grep -qxF "$(cat)" ~/.ssh/authorized_keys || cat >> ~/.ssh/authorized_keys' \
  && echo '  node0 -> node1: OK' \
  || { echo '  ERROR: cannot SSH to node1'; exit 1; }
ssh-keyscan -H node0 node1 >> "$HOME/.ssh/known_hosts" 2>/dev/null || true

# 3. Go on PATH for non-interactive shells
[ -e /usr/local/bin/go ] || sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go

# 4. nodes.env
[ -f KeyTransparencyScripts/nodes.env ] || cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env
echo '[3/4] Environment ready.'

# 5. Clean any stale sockets
sudo rm -f /tmp/coniks.sock 2>/dev/null || true
ssh -o StrictHostKeyChecking=no -o LogLevel=ERROR node1 'sudo rm -f /tmp/coniks.sock' 2>/dev/null || true
echo '[4/4] Cleaned stale state.'

echo
echo '======================================================='
echo '  Setup complete. Which experiment do you want to run?'
echo '======================================================='
echo '    [1] Key Transparency  (Section 7.1: Figs 4a, 4b, 4c, 5)'
echo '    [2] Decentralized Ledger  (Section 7.2: Figs 6, 7)'
echo '    [q] Quit (I will run it later manually)'
read -rp '> Choice [1/2/q]: ' choice
case "$choice" in
  1) exec "$REPO/run_kt.sh" ;;
  2) exec "$REPO/run_dl.sh" ;;
  q|Q) echo 'Ok. When ready, run either ./run_kt.sh or ./run_dl.sh from this directory.'; exit 0 ;;
  *) echo 'Unrecognized choice. Rerun and pick 1, 2, or q.'; exit 2 ;;
esac
