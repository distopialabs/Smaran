#!/usr/bin/env bash
# Install CONIKS (from the Coniks/ submodule).
# Human time:   ~1 min interactive
# Compute time: ~2-3 min on a fresh CloudLab node
set -euo pipefail

echo "Installing Coniks"

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/.." && pwd)"

need_sudo() { if [ "$(id -u)" -ne 0 ]; then echo "sudo"; else echo ""; fi; }
SUDO="$(need_sudo)"

if ! command -v go >/dev/null 2>&1; then
  "$HERE/install_smaran.sh" >/dev/null 2>&1 || true
fi
$SUDO apt-get update -qq
$SUDO DEBIAN_FRONTEND=noninteractive apt-get install -y -qq build-essential git make

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

cd "$REPO_ROOT"
if [ ! -f Coniks/Makefile ] && [ ! -f Coniks/go.mod ]; then
  echo "  fetching Coniks submodule"
  git submodule update --init --recursive Coniks
fi

echo "  building CONIKS binaries"
make coniks

echo "  CONIKS binaries at ${REPO_ROOT}/bin/coniks{server,client,bench,bot}"
