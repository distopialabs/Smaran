#!/usr/bin/env bash
# Key-Transparency experiment runner. Asks Full vs Quick, then runs all four
# Fig scripts in order and copies the PDFs to ~/Smaran-output/.
set -euo pipefail
REPO="$(cd "$(dirname "$0")" && pwd)"
cd "$REPO"

echo
echo '======================================================='
echo '  Smaran Key Transparency (Section 7.1)'
echo '======================================================='
echo '    [1] FULL   sweep (~3 hours; reproduces every point from paper)'
echo '                Fig 4: 11 versions {2,4,8,16,32,64,128,256,512,1024,2047}'
echo '                Fig 5: 6 user counts {10k,50k,100k,200k,500k,1M}'
echo '    [2] QUICK  sweep (~90 min; reduced points, same qualitative shape)'
echo '                Fig 4: 5 versions {2,16,128,256,2047}'
echo '                Fig 5: 3 user counts {10k,0.2M,1M}'
echo '    [3] SMOKE  test (~3 min; validates pipeline only, one point)'
echo '    [q] Quit'
read -rp '> Choice [1/2/3/q]: ' choice

case "$choice" in
  1)
    SCRIPTS=(
      ./KeyTransparencyScripts/run_fig4a.sh
      ./KeyTransparencyScripts/run_fig4b.sh
      ./KeyTransparencyScripts/run_fig4c.sh
      ./KeyTransparencyScripts/run_fig5.sh
    )
    LABEL='FULL'
    ;;
  2)
    SCRIPTS=(
      ./QuickTesting-KeyTransparency/run_fig4a_quick.sh
      ./QuickTesting-KeyTransparency/run_fig4b_quick.sh
      ./QuickTesting-KeyTransparency/run_fig4c_quick.sh
      ./QuickTesting-KeyTransparency/run_fig5_quick.sh
    )
    LABEL='QUICK'
    ;;
  3)
    exec ./KeyTransparencyScripts/smoke_test.sh
    ;;
  q|Q) echo 'Ok, bye.'; exit 0 ;;
  *) echo 'Unrecognized choice.'; exit 2 ;;
esac

echo
echo "Running $LABEL sweep. PDFs will land in ~/Smaran/output/."
echo
for s in "${SCRIPTS[@]}"; do
  echo "=== $(date '+%H:%M:%S')  $s"
  "$s" || { echo "$s FAILED"; exit 1; }
done

echo
echo '======================================================='
echo '  Done. PDFs are here:'
ls -la "$REPO/output/"*.pdf 2>/dev/null || true
echo
echo '  Run "python3 KeyTransparencyScripts/verify.py" for an'
echo '  automated shape-check against the paper claims.'
echo '======================================================='
