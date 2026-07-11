#!/usr/bin/env bash
# Decentralized-Ledger experiment runner (Section 7.2 - Figs 6, 7).
#
# The DL portion of the artifact lives on a separate branch (timing_debug).
# This script signposts reviewers who reach it from setup_cloudlab.sh's
# menu, then exits.
set -euo pipefail

REPO="$(cd "$(dirname "$0")" && pwd)"

echo
echo '======================================================='
echo '  Smaran Decentralized Ledger (Section 7.2)'
echo '======================================================='
echo
echo 'The Decentralized-Ledger portion is on a separate branch.'
echo 'Its own setup, scripts, and README are on the timing_debug branch.'
echo
echo 'To run the DL experiments, do the following (still on node0):'
echo
echo "   cd $REPO"
echo '   git fetch origin timing_debug'
echo '   git checkout timing_debug'
echo '   git submodule update --init --recursive --force'
echo '   # then follow the README on the timing_debug branch'
echo
echo 'Reference:'
echo '   https://github.com/distopialabs/Smaran/tree/timing_debug'
echo
exit 0
