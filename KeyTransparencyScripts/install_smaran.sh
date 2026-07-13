#!/usr/bin/env bash
# Install Smaran (and the shared ktserver/ktbench binaries used for OPTIKS).
# Human time:   ~2 min interactive
# Compute time: ~3-5 min on a fresh CloudLab r6615/c6420 node
set -euo pipefail

echo "Installing Smaran"

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HERE/.." && pwd)"

GO_VERSION="${GO_VERSION:-1.24.0}"
GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz"

need_sudo() {
  if [ "$(id -u)" -ne 0 ]; then echo "sudo"; else echo ""; fi
}
SUDO="$(need_sudo)"

install_apt_packages() {
  $SUDO apt-get update -qq
  $SUDO DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
      build-essential curl git make protobuf-compiler python3 python3-pip rsync
}

install_go() {
  if command -v go >/dev/null 2>&1 && go version | grep -q "go${GO_VERSION%.0}"; then
    echo "  go already installed: $(go version)"
    return
  fi
  echo "  installing Go ${GO_VERSION}"
  curl -fsSL "https://go.dev/dl/${GO_TARBALL}" -o "/tmp/${GO_TARBALL}"
  $SUDO rm -rf /usr/local/go
  $SUDO tar -C /usr/local -xzf "/tmp/${GO_TARBALL}"
  rm -f "/tmp/${GO_TARBALL}"
  if ! grep -q '/usr/local/go/bin' "${HOME}/.profile" 2>/dev/null; then
    echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> "${HOME}/.profile"
  fi
}

install_python_deps() {
  python3 -m pip install --quiet --user -r "${REPO_ROOT}/experiments/requirements.txt"
}

install_apt_packages
install_go
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
install_python_deps

echo "  building Smaran (ktserver, ktbench, samurai, proofc)"
cd "$REPO_ROOT"
go build -o bin/ktserver ./cmd/ktserver
go build -o bin/ktbench ./cmd/ktbench
go build -o bin/samurai ./cmd/samurai
go build -o bin/proofc  ./cmd/proofc

echo "  Smaran binaries installed to ${REPO_ROOT}/bin/"
