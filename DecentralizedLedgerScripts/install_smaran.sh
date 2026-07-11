#!/usr/bin/env bash
# Install Smaran (this paper's system; called "samurai" in binary/package names).
# All three protocols live in one Go module, so this installs the shared
# toolchain and builds every binary; it exists as a separate script so each
# protocol has its own install entry point.
source "$(dirname "$0")/lib/common.sh"
install_protocol "Smaran"
