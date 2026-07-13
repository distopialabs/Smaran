# QuickTesting-DecentralizedLedgerScripts

Quick-turnaround variants of the per-figure experiment scripts: same
pipeline and code paths as `../DecentralizedLedgerScripts/run_fig<yy>.sh`,
with reduced parameters so each figure's overall trend is visible without
paper-scale runtimes:

- Figures 6a / 6b / 6c: query range sizes 1, 100, 1000 (over a 10k-block ingest)
- Figure 7b: the paper's range points below 10k (1 … 7000)
- Figure 7a: 100k / 200k / 500k users, 10k blocks per point
- Figure 7c: shard counts 1, 10, 100 with 100k / 200k / 500k users
- Shorter per-point durations (30–60 s instead of 2–15 min)

All defaults live in `../DecentralizedLedgerScripts/config.sh` (edit there, or
override per-run via environment variables).

Absolute numbers will be far below the paper's; compare trends, and use
`../DecentralizedLedgerScripts/plot_paper_figures.sh` for the paper-scale
reference figures. See the repository README for estimated runtimes.
