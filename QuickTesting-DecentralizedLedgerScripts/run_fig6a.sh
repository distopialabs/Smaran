#!/usr/bin/env bash
# Quick-turnaround variant of Figure 6a: same pipeline as
# DecentralizedLedgerScripts/run_fig6a.sh with reduced parameters (the
# QUICK_* defaults in ../DecentralizedLedgerScripts/config.sh) so the
# figure's overall trend is visible without paper-scale runtimes.
export QUICK=1
exec "$(dirname "$0")/../DecentralizedLedgerScripts/run_fig6a.sh" "$@"
