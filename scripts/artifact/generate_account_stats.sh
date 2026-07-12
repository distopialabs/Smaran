#!/usr/bin/env bash
# Regenerate the account-statistics CSVs that drive account selection in the
# benchmarks. The CloudLab datasets ship both files prebuilt; this script is
# how Path-B (own server) users produce them, since Zenodo carries only the
# block dataset:
#
#   account_stats_50k.csv — first 50,000 blocks (fig7a/fig7c ingestion points)
#   account_stats_all.csv — the full 2,616,996-block window (full-scale query
#                           benchmarks; quick runs generate their own list)
#
# One deterministic pass over data/blocks per file (only the order of
# equal-count accounts can vary between runs, which the benchmarks don't care
# about). Measured on the paper's r6615 server: ~5 s for the 50k file,
# ~4 min for the full window (needs ~10 GB RAM for the 60 M-account map).
#
# Usage: scripts/artifact/generate_account_stats.sh [50k|all|both]  (default: both)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../DecentralizedLedgerScripts/lib/common.sh"
source "$SCRIPT_DIR/../../DecentralizedLedgerScripts/config.sh"
export PATH="$PATH:/usr/local/go/bin"

require_setup go blocks

# Where the CSVs land; the benchmarks read them from the repo root.
OUTPUT_DIR="${OUTPUT_DIR:-$REPO_ROOT}"

generate() {
    local name="$1" blocks="$2"
    local out="$OUTPUT_DIR/$name"
    say "Generating $name from the first $blocks blocks (output: $out)"
    # Write to a temp name so an interrupted run can't leave a truncated file
    # that looks complete.
    (cd "$REPO_ROOT" && go run ./cmd/tools/count_account_updates \
        -n "$blocks" -o "$out.tmp")
    mv "$out.tmp" "$out"
    say "Done: $out ($(wc -l <"$out") lines)"
}

case "${1:-both}" in
    50k)  generate account_stats_50k.csv 50000 ;;
    all)  generate account_stats_all.csv "$FULL_N_BLOCKS" ;;
    both) generate account_stats_50k.csv 50000
          generate account_stats_all.csv "$FULL_N_BLOCKS" ;;
    *)    die "usage: $0 [50k|all|both]" ;;
esac
