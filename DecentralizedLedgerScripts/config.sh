#!/usr/bin/env bash
# =============================================================================
# Experiment configuration — EDIT DEFAULTS HERE
# =============================================================================
# Every experiment parameter lives in this file. Two ways to change a value:
#   1. Edit the default below (applies to every future run), or
#   2. Override per-run via environment variable, e.g.:
#        RANGES_MERKLE="1 100" ./DecentralizedLedgerScripts/run_fig6a.sh
#
# The QuickTesting-DecentralizedLedgerScripts wrappers set QUICK=1, which
# selects the QUICK_* column; otherwise the FULL_* (paper) column applies.
# Lists are space-separated strings.

# --- Shared ------------------------------------------------------------------
DEFAULT_NUM_CLIENTS=32              # concurrent query clients (paper: 32)
DEFAULT_COOLDOWN=10                 # seconds between benchmark points
DEFAULT_DB_ROOT=/data/local/artifact-dbs   # ingested-DB cache location

# --- Figures 6a/6b/6c + 7b: query benchmarks ---------------------------------
# Blocks ingested into each protocol's query DB (range size must not exceed it)
FULL_N_BLOCKS=2616996               # full dataset window
QUICK_N_BLOCKS=10000

# Wall time per benchmark point (in-flight requests finish after it elapses)
FULL_PROOF_DURATION=2m
QUICK_PROOF_DURATION=30s

# Query range sizes per protocol (paper caps: Verkle proofs are very slow,
# Merkle payloads blow up past 600k)
FULL_RANGES_SMARAN="1 100 500 1000 5000 7000 50000 200000 600000 1200000 2600000"
FULL_RANGES_MERKLE="1 100 500 1000 5000 7000 50000 200000 600000"
FULL_RANGES_VERKLE="1 100 500 1000 5000 7000"
QUICK_RANGES_SMARAN="1 100 1000"
QUICK_RANGES_MERKLE="1 100 1000"
QUICK_RANGES_VERKLE="1 100 1000"

# Figure 7b sweeps Smaran twice (optimus + non_optimus) over these ranges
FULL_RANGES_7B="$FULL_RANGES_SMARAN"
QUICK_RANGES_7B="1 100 500 1000 5000 7000"   # the paper's points < 10k

# --- Figure 7a: ingestion throughput ------------------------------------------
# Block-limited (every point ingests this many blocks from a fresh DB);
# the timeout is only a deadline, as in the paper.
FULL_K_USERS="100000 200000 500000 1000000 2000000"
QUICK_K_USERS="100000 200000 500000"
FULL_INGEST_BENCH_BLOCKS=50000      # paper value
QUICK_INGEST_BENCH_BLOCKS=10000
INGEST_BENCH_TIMEOUT=15m            # deadline per point (both tiers)

# --- Figure 7c: sharding sweep (Smaran only) ----------------------------------
# Duration-limited as in the paper (full N_BLOCKS is never reached).
FULL_SHARDS="1 10 50 100 500 1000"
QUICK_SHARDS="1 10 100"
FULL_SHARDS_BENCH_BLOCKS=2616996    # paper value (duration is the real cap)
QUICK_SHARDS_BENCH_BLOCKS=10000
FULL_SHARDS_BENCH_DURATION=5m       # paper value
QUICK_SHARDS_BENCH_DURATION=60s
