#!/usr/bin/env bash
# Figure 6c: query response payload size vs. range size (Merkle, Verkle,
# Smaran; Cauchy series from prebaked logs).
#
# Figures 6a/6b/6c share one benchmark sweep — see run_fig6a.sh header.
source "$(dirname "$0")/lib/experiments.sh"
echo "Running experiment Figure 6c"
run_fig6_pipeline
echo "Figure 6c: $RESULTS_DIR/fig6/numclients${NUM_CLIENTS}/fig6c_payload_size.pdf"
