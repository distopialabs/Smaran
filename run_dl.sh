#!/usr/bin/env bash
# Decentralized-Ledger experiment runner (Section 7.2 — Figs 6, 7).
# Asks which tier to run, then runs the figure scripts in order.
# Figures land under results/ (see README.md, "Where everything lives").
set -euo pipefail
REPO="$(cd "$(dirname "$0")" && pwd)"
cd "$REPO"

echo
echo '======================================================='
echo '  Smaran Decentralized Ledger (Section 7.2)'
echo '======================================================='
echo '    [0] TIER 0  plot-only (~1 min; all six figures from curated paper logs)'
echo '    [1] FULL    sweep (tens of hours; full-scale ingest + all six figures)'
echo '    [2] QUICK   sweep (~50 min; reduced scale, same qualitative trends)'
echo '    [q] Quit'
read -rp '> Choice [0/1/2/q]: ' choice

case "$choice" in
  0)
    exec ./DecentralizedLedgerScripts/plot_paper_figures.sh
    ;;
  1)
    SCRIPTS=(
      ./DecentralizedLedgerScripts/run_fig6a.sh
      ./DecentralizedLedgerScripts/run_fig6b.sh
      ./DecentralizedLedgerScripts/run_fig6c.sh
      ./DecentralizedLedgerScripts/run_fig7a.sh
      ./DecentralizedLedgerScripts/run_fig7b.sh
      ./DecentralizedLedgerScripts/run_fig7c.sh
    )
    LABEL='FULL'
    ;;
  2)
    SCRIPTS=(
      ./QuickTesting-DecentralizedLedgerScripts/run_fig6a.sh
      ./QuickTesting-DecentralizedLedgerScripts/run_fig6b.sh
      ./QuickTesting-DecentralizedLedgerScripts/run_fig6c.sh
      ./QuickTesting-DecentralizedLedgerScripts/run_fig7a.sh
      ./QuickTesting-DecentralizedLedgerScripts/run_fig7b.sh
      ./QuickTesting-DecentralizedLedgerScripts/run_fig7c.sh
    )
    LABEL='QUICK'
    ;;
  q|Q) echo 'Ok, bye.'; exit 0 ;;
  *) echo 'Unrecognized choice.'; exit 2 ;;
esac

echo
echo "Running $LABEL sweep. Figures will land under $REPO/results/."
echo
for s in "${SCRIPTS[@]}"; do
  echo "=== $(date '+%H:%M:%S')  $s"
  "$s" || { echo "$s FAILED"; exit 1; }
done

echo
echo '======================================================='
echo '  Done. Figures are under results/:'
find "$REPO/results" -name '*.pdf' 2>/dev/null | sort | head -20 || true
echo
echo '  Compare against the paper figures (and results/paper-figures/'
echo '  from Tier 0). See README.md "The six figures" for what to check.'
echo '======================================================='
