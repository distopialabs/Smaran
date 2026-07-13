#!/usr/bin/env bash
# Smoke test — validates the full pipeline (build, SSH between nodes, ktserver,
# ktbench, coniksserver, coniksbench, plotting) in ~3 minutes total.
#
# HUMAN TIME:   ~1 min
# COMPUTE TIME: ~3 min
# OUTPUT:       ~/Smaran/output/fig4a_latency.pdf (with a single sweep point)
#
# Run this BEFORE the full ~3-hour sweep to catch environment problems early.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
export FIGURE=4a
export TEMPLATE=smoke_test.toml
export CACHE_KEY=smoke_test
# stale-state auto-reset
sudo pkill -9 coniksserver ktserver ktbench coniksbench 2>/dev/null || true
sudo rm -f /tmp/coniks.sock || true
ssh -o StrictHostKeyChecking=no node1 'sudo pkill -9 -f coniks 2>/dev/null; sudo rm -f /tmp/coniks.sock' 2>/dev/null || true
rm -rf "$HOME/Smaran/logs/ae_cache/smoke_test" 2>/dev/null || true
source "$HERE/lib/_run_common.sh"
