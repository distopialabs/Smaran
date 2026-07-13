#!/usr/bin/env bash
# Install OPTIKS. OPTIKS ships as a mode inside the shared ktserver/ktbench
# binaries (see Optiks/README.md), so this reuses the Smaran build path.
# Human time:   ~1 min interactive
# Compute time: ~2-3 min on a fresh CloudLab node
set -euo pipefail

echo "Installing Optiks"

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/.." && pwd)"

need_sudo() { if [ "$(id -u)" -ne 0 ]; then echo "sudo"; else echo ""; fi; }
SUDO="$(need_sudo)"

# Reuse the Smaran installer's toolchain setup (Go, apt packages) if it hasn't
# already run. This is idempotent — nothing gets reinstalled if already present.
if ! command -v go >/dev/null 2>&1; then
  "$HERE/install_smaran.sh" >/dev/null 2>&1 || true
fi

$SUDO apt-get update -qq
$SUDO DEBIAN_FRONTEND=noninteractive apt-get install -y -qq build-essential git make

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
cd "$REPO_ROOT"
go build -o bin/ktserver ./cmd/ktserver
go build -o bin/ktbench  ./cmd/ktbench

echo "  OPTIKS-capable binaries at ${REPO_ROOT}/bin/ktserver and ${REPO_ROOT}/bin/ktbench"
echo "  Run with: ${REPO_ROOT}/bin/ktserver --protocol optiks"
