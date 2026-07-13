#!/usr/bin/env bash
# Reviewer runs this ONE script on their LAPTOP after instantiating the
# smaran-kt-ae CloudLab profile. Takes the node0 SSH hostname as its only
# argument. SSHes in, runs everything, pulls PDFs back to ~/Desktop.
#
# Usage:
#   bash run_ae_from_laptop.sh <cloudlab-username> <node0-hostname>
#
# Example:
#   bash run_ae_from_laptop.sh alice clnode123.clemson.cloudlab.us
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "Usage: $0 <cloudlab-username> <node0-hostname>"
  exit 2
fi

USER=$1
HOST=$2
OUTPUT_DIR="${HOME}/Desktop/smaran-ae-output"

echo "==========================================================="
echo "  Smaran Artifact Evaluation - driving from laptop"
echo "  Target: ${USER}@${HOST}"
echo "  Output: ${OUTPUT_DIR}"
echo "  Expected wall-clock: ~90 minutes"
echo "==========================================================="

echo "[1/3] Confirming SSH reachability..."
ssh -o StrictHostKeyChecking=accept-new "${USER}@${HOST}" "hostname" \
  || { echo "SSH to ${USER}@${HOST} failed"; exit 1; }

echo "[2/3] Running all four experiments on node0 (this takes ~90 min)..."
ssh "${USER}@${HOST}" "bash /opt/Smaran/KeyTransparencyScripts/run_all.sh"

echo "[3/3] Copying PDFs to ${OUTPUT_DIR}..."
mkdir -p "${OUTPUT_DIR}"
scp "${USER}@${HOST}:~/Smaran/output/*.pdf" "${OUTPUT_DIR}/"

echo ""
echo "==========================================================="
echo "  Done. Open the PDFs at ${OUTPUT_DIR} and compare to paper."
echo "==========================================================="
ls -la "${OUTPUT_DIR}/"
