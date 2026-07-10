# plot_bench.py

Generates benchmark graphs from ingestion CSV files and proof summary text files. Output is PDF (vector) or PNG at configurable DPI, sized for a two-column academic paper layout.

**Dependencies:** Python 3, `matplotlib`, `numpy`, `pandas`.

---

## Subcommands

| Subcommand | Graphs | Input |
|---|---|---|
| `ingestion-timeseries` | G1–G6 | One or more ingestion CSVs |
| `ingestion-summary` | G7–G9 | Per-protocol sets of ingestion CSVs across k-user scales |
| `proof-summary` | G10–G13 | Per-protocol sets of proof summary CSV files across range sizes |
| `update-timeseries` | G14–G15 | One or more update metrics CSVs |
| `proof-throughput-timeseries` | G16–G17 | One or more server bench log CSVs (from open-loop benchmarks) |

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
  --protocol LABEL --inputs RANGE:CSV_PATH[,RANGE:CSV_PATH,...] \
  [--protocol LABEL --inputs RANGE:CSV_PATH[,...]] \
  [options]
```

### Expected Summary CSV Format

Generated by the proof benchmark client (`internal/benchutil/proofbench.go`):

```csv
duration_s,clients,range_size,total_requests,client_errors,server_errors,verify_failures,throughput_rps,avg_proofgen_ms,avg_response_ms,avg_verify_ms,avg_payload_kb
60.0,10,100,4523,3,2,0,75.4,8.2,13.1,1.4,2.3
```

All values are numeric. Latency values are in milliseconds. Payload size is in kilobytes.

### Additional Options

| Flag | Default | Description |
|---|---|---|
| `--log-x` | auto | Force log scale on x-axis (auto-detected when range span > 10×) |

### Graphs Produced

| Graph | File | Y-axis | Source Field |
|---|---|---|---|
| **G10** | `G10_proofgen_vs_range` | Latency (ms) | `Avg Proofgen` |
| **G11** | `G11_response_vs_range` | Latency (ms) | `avg_response_ms` |
| **G12** | `G12_verify_vs_range` | Latency (ms) | `Avg Verify` |
| **G13** | `G13_throughput_vs_range` | Throughput (req/s) | `Throughput` |

### Examples

```bash
# Single protocol
python scripts/benchmark/plot_bench.py proof-summary \
  --protocol "samurai" \
  --inputs "100:benchmark_output/samurai/proof_range100.csv,\
500:benchmark_output/samurai/proof_range500.csv,\
1000:benchmark_output/samurai/proof_range1000.csv" \
  --output-dir benchmark_output/plots --format pdf

# Multi-protocol
python scripts/benchmark/plot_bench.py proof-summary \
  --protocol "samurai" --inputs "100:s100.csv,500:s500.csv,1000:s1000.csv" \
  --protocol "merkle"  --inputs "100:m100.csv,500:m500.csv,1000:m1000.csv" \
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

## Subcommand 5: `proof-throughput-timeseries`

Plots proof request throughput and latency time-series from server-side bench log CSVs. These CSVs are produced by the Samurai server when run with `--bench --bench-output <path>`. Multiple inputs are overlaid for comparison.

```
python plot_bench.py proof-throughput-timeseries \
  --input LABEL:CSV_PATH \
  [--input LABEL:CSV_PATH ...] \
  [options]
```

### Additional Options

| Flag | Default | Description |
|---|---|---|
| `--input` | required, repeatable | `label:csv_path` pair — label identifies the run in the legend |
| `--window` | `5.0` | Rolling window size in seconds |
| `--graphs` | `all` | Comma-separated subset to produce, e.g. `G16,G17`, or `all` |

### Expected CSV Format

```
start_at_ns,completed_at_ns
```

Produced by `BenchLogger` in `internal/benchutil/benchlog.go`. Each row represents one completed proof request with nanosecond-precision timestamps.

### Graphs Produced

| Graph | File | Y-axis | Derivation |
|---|---|---|---|
| **G16** | `G16_proof_throughput` | Throughput (req/s) | Count of requests completed per window / window size |
| **G17** | `G17_proof_latency` | Latency (ms) | Mean of `(completed_at_ns − start_at_ns) / 1e6` per window |

### Examples

```bash
# Single run
python scripts/benchmark/plot_bench.py proof-throughput-timeseries \
  --input "samuraimpt:benchmark_output/samuraimpt/openloop_range50000_clients8_20260327_143012.csv" \
  --output-dir benchmark_output/plots --format png

# Compare two runs (different client counts)
python scripts/benchmark/plot_bench.py proof-throughput-timeseries \
  --input "4-clients:openloop_range50000_clients4.csv" \
  --input "8-clients:openloop_range50000_clients8.csv" \
  --warmup 5 --cooldown 5 \
  --output-dir benchmark_output/plots --format pdf
```

### Open-Loop Benchmark Workflow

The open-loop benchmark measures maximum server throughput by firing requests at a fixed rate:

1. **Start the server** with bench logging:
   ```bash
   ./bin/samurai serve --db-dir /data/db --bench --bench-output bench_server.csv
   ```

2. **Run the client** with desired load:
   ```bash
   ./bin/proofc openloop \
     --server-addr localhost:50051 \
     --num-clients 8 \
     --rps-per-client 10 \
     --max-concurrent 100 \
     --range-size 50000 \
     --accounts-list accounts.csv \
     --duration 60s
   ```

3. **Sweep multiple configurations** using the provided script:
   ```bash
   bash scripts/benchmark/run_openloop_bench.sh
   ```
   This iterates over range sizes and client counts, producing separate server CSVs per combination.

4. **Generate plots**:
   ```bash
   python scripts/benchmark/plot_bench.py proof-throughput-timeseries \
     --input "samuraimpt:bench_server.csv" \
     --output-dir plots --warmup 5 --cooldown 5
   ```

The client prints a summary to stdout showing sent/completed/dropped/error counts. Drops indicate server saturation (semaphore full — more than `--max-concurrent` requests in-flight per connection).

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
| G11 | `G11_response_vs_range.{ext}` |
| G12 | `G12_verify_vs_range.{ext}` |
| G13 | `G13_throughput_vs_range.{ext}` |
| G14 | `G14_update_throughput_measured.{ext}` |
| G15 | `G15_update_latency_measured.{ext}` |
| G16 | `G16_proof_throughput.{ext}` |
| G17 | `G17_proof_latency.{ext}` |

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