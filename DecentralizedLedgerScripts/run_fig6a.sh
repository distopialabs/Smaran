#!/usr/bin/env bash
# Figure 6a: closed-loop query latency vs. range size (Merkle, Verkle, Smaran;
# Cauchy series from prebaked logs).
#
# Figures 6a/6b/6c share one benchmark sweep: whichever run_fig6*.sh runs
# first executes it (ingest -> serve -> bench per protocol) and caches the
# logs; the other two reuse them and only re-plot. FORCE_RERUN=1 to redo.
#
# Parameters and per-tier defaults: ../config.sh. Time estimates: README —
# dominated by the one-time ingestion per protocol plus Smaran's shard-DB
# setup/teardown around every Smaran run.
source "$(dirname "$0")/lib/experiments.sh"
echo "Running experiment Figure 6a"
run_fig6_pipeline
echo "Figure 6a: $RESULTS_DIR/fig6/numclients${NUM_CLIENTS}/fig6a_query_latency.pdf"
