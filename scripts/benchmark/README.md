# plot_bench.py

Generates benchmark graphs from ingestion CSV files and proof summary text files. Output is PDF (vector) or PNG at configurable DPI, sized for a two-column academic paper layout.

**Dependencies:** Python 3, `matplotlib`, `numpy`, `pandas`.

---

## Subcommands

| Subcommand | Graphs | Input |
|---|---|---|
| `ingestion-timeseries` | G1–G6 | One or more ingestion CSVs |
| `ingestion-summary` | G7–G9 | Per-protocol sets of ingestion CSVs across k-user scales |
| `proof-summary` | G10–G13 | Per-protocol sets of `proof_bench_summary.txt` files across range sizes |
| `update-timeseries` | G14–G15 | One or more update metrics CSVs |

---

## Global Options

These apply to all three subcommands.

| Flag | Default | Description |
|---|---|---|
| `--output-dir` | `benchmark_output/plots` | Directory to write output files |
| `--format` | `pdf` | Output format: `pdf` or `png` |
| `--dpi` | `300` | DPI for PNG output |
| `--warmup` | `0` | Seconds to discard from the start of each dataset |
| `--cooldown` | `0` | Seconds to discard from the end of each dataset |

Warmup/cooldown trimming is applied to relative time (seconds since the first queued block) before any computation.

---

## Subcommand 1: `ingestion-timeseries`

Plots rolling-window time-series from one or more ingestion CSVs. Multiple inputs are overlaid on the same graph for protocol comparison.

```
python plot_bench.py ingestion-timeseries \
  --input LABEL:CSV_PATH \
  [--input LABEL:CSV_PATH ...] \
  [options]
```

### Additional Options

| Flag | Default | Description |
|---|---|---|
| `--input` | required, repeatable | `label:csv_path` pair — label identifies the protocol/run in the legend |
| `--window` | `5.0` | Rolling window size in seconds |
| `--graphs` | `all` | Comma-separated subset to produce, e.g. `G1,G3,G5`, or `all` |

### Expected CSV Format

```
block_num,num_raw_updates,num_selected_updates,queued_at_ns,start_at_ns,completed_at_ns[,...]
```

Required columns: `block_num`, `num_selected_updates`, `queued_at_ns`, `start_at_ns`, `completed_at_ns`. Extra columns are ignored.

### Graphs Produced

| Graph | File | Y-axis | Derivation |
|---|---|---|---|
| **G1** | `G1_block_latency` | Latency (ms) | `(completed_at_ns − start_at_ns) / 1e6` per block, rolling mean |
| **G2** | `G2_e2e_block_latency` | Latency (ms) | `(completed_at_ns − queued_at_ns) / 1e6` per block, rolling mean. Equals G1 for Merkle/Verkle. |
| **G3** | `G3_e2e_update_latency` | Latency (ms) | `(completed_at_ns − queued_at_ns) / 1e6 / num_selected_updates` per block (skip 0-update blocks), rolling mean |
| **G4** | `G4_update_latency` | Latency (ms) | `(completed_at_ns − start_at_ns) / 1e6 / num_selected_updates` per block (skip 0-update blocks), rolling mean |
| **G5** | `G5_block_throughput` | Throughput (blocks/s) | Block count per window / window size |
| **G6** | `G6_update_throughput` | Throughput (updates/s) | Sum of `num_selected_updates` per window / window size |

### Examples

```bash
# Single protocol
python scripts/benchmark/plot_bench.py ingestion-timeseries \
  --input "samurai:benchmark_output/samurai/ingestion_0_20260322_010608.csv" \
  --output-dir benchmark_output/plots --format png

# Multi-protocol overlay with warmup/cooldown trimming
python scripts/benchmark/plot_bench.py ingestion-timeseries \
  --input "samurai:benchmark_output/samurai/ingestion_0_20260322_010608.csv" \
  --input "merkle:benchmark_output/merkle/ingestion_0_20260322_020805.csv" \
  --warmup 5 --cooldown 5 \
  --output-dir benchmark_output/plots --format pdf

# Only latency graphs, custom window
python scripts/benchmark/plot_bench.py ingestion-timeseries \
  --input "samurai:benchmark_output/samurai/ingestion_0_20260322_010608.csv" \
  --graphs G1,G2,G3,G4 --window 10
```

---

## Subcommand 2: `ingestion-summary`

Computes scalar summary statistics per file and plots them as a function of the number of users (k). Each protocol becomes one line. X-axis is log-scale.

```
python plot_bench.py ingestion-summary \
  --protocol LABEL --inputs K:CSV_PATH[,K:CSV_PATH,...] \
  [--protocol LABEL --inputs K:CSV_PATH[,...]] \
  [options]
```

`--protocol` and `--inputs` are matched by position — the Nth `--protocol` applies to the Nth `--inputs`.

### Graphs Produced

| Graph | File | Y-axis | Derivation |
|---|---|---|---|
| **G7** | `G7_update_latency_vs_kusers` | Avg Update Latency (ms) | Mean of `(completed − start) / 1e6 / num_selected_updates` across all blocks (skip 0-update blocks) |
| **G8** | `G8_e2e_update_latency_vs_kusers` | Avg E2E Update Latency (ms) | Mean of `(completed − queued) / 1e6 / num_selected_updates` across all blocks (skip 0-update blocks) |
| **G9** | `G9_throughput_vs_kusers` | Throughput (updates/s) | `total_selected_updates / ((last_completed_at_ns − first_queued_at_ns) / 1e9)` |

### Examples

```bash
# Single protocol across three scales
python scripts/benchmark/plot_bench.py ingestion-summary \
  --protocol "samurai" \
  --inputs "10000:benchmark_output/samurai/ingestion_10k.csv,\
100000:benchmark_output/samurai/ingestion_100k.csv,\
1000000:benchmark_output/samurai/ingestion_1M.csv" \
  --warmup 5 --cooldown 5 \
  --output-dir benchmark_output/plots --format pdf

# Multi-protocol comparison
python scripts/benchmark/plot_bench.py ingestion-summary \
  --protocol "samurai" --inputs "10000:s_10k.csv,100000:s_100k.csv,1000000:s_1M.csv" \
  --protocol "merkle"  --inputs "10000:m_10k.csv,100000:m_100k.csv,1000000:m_1M.csv" \
  --output-dir benchmark_output/plots --format pdf
```

---

## Subcommand 3: `proof-summary`

Plots proof benchmark metrics as a function of range size. Each protocol becomes one line. X-axis is log-scale when `max(range) / min(range) > 10` (auto-detected), or forced with `--log-x`.

```
python plot_bench.py proof-summary \
  --protocol LABEL --inputs RANGE:TXT_PATH[,RANGE:TXT_PATH,...] \
  [--protocol LABEL --inputs RANGE:TXT_PATH[,...]] \
  [options]
```

### Expected Summary File Format

Generated by the proof benchmark client (`internal/benchutil/proofbench.go`):

```
=== Proof Benchmark Results ===
Duration:            60s
Clients:             10
Range Size:          100
Total Requests:      4523
Client Errors:       3
Server Errors:       2
Verify Failures:     0
Throughput:          75.4 req/s
Avg Proofgen:        8.2ms
Avg E2E Latency:     13.1ms
Avg Verify:          1.4ms
Avg Payload Size:    2.3KB
```

Latency fields use Go's `time.Duration` format. All suffixes are supported: `ns`, `µs`/`us`, `ms`, `s`, `m`, `h`, and combinations like `1m30s`. Values are converted to milliseconds internally.

### Additional Options

| Flag | Default | Description |
|---|---|---|
| `--log-x` | auto | Force log scale on x-axis (auto-detected when range span > 10×) |

### Graphs Produced

| Graph | File | Y-axis | Source Field |
|---|---|---|---|
| **G10** | `G10_proofgen_vs_range` | Latency (ms) | `Avg Proofgen` |
| **G11** | `G11_e2e_vs_range` | Latency (ms) | `Avg E2E Latency` |
| **G12** | `G12_verify_vs_range` | Latency (ms) | `Avg Verify` |
| **G13** | `G13_throughput_vs_range` | Throughput (req/s) | `Throughput` |

### Examples

```bash
# Single protocol
python scripts/benchmark/plot_bench.py proof-summary \
  --protocol "samurai" \
  --inputs "100:benchmark_output/samurai/proof_range100.txt,\
500:benchmark_output/samurai/proof_range500.txt,\
1000:benchmark_output/samurai/proof_range1000.txt" \
  --output-dir benchmark_output/plots --format pdf

# Multi-protocol
python scripts/benchmark/plot_bench.py proof-summary \
  --protocol "samurai" --inputs "100:s100.txt,500:s500.txt,1000:s1000.txt" \
  --protocol "merkle"  --inputs "100:m100.txt,500:m500.txt,1000:m1000.txt" \
  --output-dir benchmark_output/plots --format pdf
```

---

## Subcommand 4: `update-timeseries`

Plots update-level throughput and latency from update metrics CSVs. Multiple inputs are overlaid for protocol comparison. These metrics are recorded using time-windowed atomic counters (1-second windows) and capture true update-level performance.

```
python plot_bench.py update-timeseries \
  --input LABEL:CSV_PATH \
  [--input LABEL:CSV_PATH ...] \
  [options]
```

### Additional Options

| Flag | Default | Description |
|---|---|---|
| `--input` | required, repeatable | `label:csv_path` pair — label identifies the protocol/run in the legend |
| `--window` | `5.0` | Rolling window size in seconds for smoothing (the underlying data has 1-second resolution) |
| `--graphs` | `all` | Comma-separated subset to produce, e.g. `G14,G15`, or `all` |

### Expected CSV Format

```
window_end_ns,updates_completed,sum_compute_ns
```

Produced by `UpdateMetricsCollector` in `internal/benchutil/update_metrics.go`.

### Graphs Produced

| Graph | File | Y-axis | Derivation |
|---|---|---|---|
| **G14** | `G14_update_throughput_measured` | Throughput (updates/s) | `updates_completed / window_seconds` per row |
| **G15** | `G15_update_latency_measured` | Latency (ms) | `(sum_compute_ns / updates_completed) / 1e6` per row |

### Examples

```bash
# Single protocol
python scripts/benchmark/plot_bench.py update-timeseries \
  --input "samurai:benchmark_output/samurai/update_metrics_10000_20260322_010608.csv" \
  --output-dir benchmark_output/plots --format png

# Multi-protocol overlay
python scripts/benchmark/plot_bench.py update-timeseries \
  --input "samurai:benchmark_output/samurai/update_metrics_10000_20260322_010608.csv" \
  --input "merkle:benchmark_output/merkle/update_metrics_10000_20260322_020805.csv" \
  --warmup 5 --cooldown 5 \
  --output-dir benchmark_output/plots --format pdf
```

---

## Output File Names

| Graph | Filename |
|---|---|
| G1 | `G1_block_latency.{ext}` |
| G2 | `G2_e2e_block_latency.{ext}` |
| G3 | `G3_e2e_update_latency.{ext}` |
| G4 | `G4_update_latency.{ext}` |
| G5 | `G5_block_throughput.{ext}` |
| G6 | `G6_update_throughput.{ext}` |
| G7 | `G7_update_latency_vs_kusers.{ext}` |
| G8 | `G8_e2e_update_latency_vs_kusers.{ext}` |
| G9 | `G9_throughput_vs_kusers.{ext}` |
| G10 | `G10_proofgen_vs_range.{ext}` |
| G11 | `G11_e2e_vs_range.{ext}` |
| G12 | `G12_verify_vs_range.{ext}` |
| G13 | `G13_throughput_vs_range.{ext}` |
| G14 | `G14_update_throughput_measured.{ext}` |
| G15 | `G15_update_latency_measured.{ext}` |

---

## Protocol Colors and Markers

Consistent across all graphs.

| Protocol | Color | Marker |
|---|---|---|
| `samurai` | `#2166ac` (blue) | circle `o` |
| `samuraimpt` | `#4393c3` (steel blue) | diamond `D` |
| `merkle` | `#b2182b` (red) | triangle `^` |
| `verkle` | `#1b7837` (green) | square `s` |
| Unknown | auto-assigned | diamond `D` |

Time-series graphs (G1–G6, G14–G15) use lines only (no markers) to keep dense plots readable. Summary graphs (G7–G13) use markers to distinguish data points.

---