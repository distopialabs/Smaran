# Samurai

## Introduction
...

## Quick Start

```bash
git clone https://github.com/distopialabs/Samurai.git
go mod tidy
make build
```

This produces six binaries in `bin/`:

| Binary | Description |
|--------|-------------|
| `samurai` | Samurai (KZG + MPT) — ingest blocks, generate proofs, serve gRPC |
| `merkle` | Baseline MPT — ingest, proof gen, gRPC server |
| `verkle` | Baseline Verkle tree — ingest, proof gen, gRPC server |
| `verklekzg` | Verkle-KZG tree (BLS12-381 + KZG commitments) — ingest, proof gen, gRPC server |
| `proofc` | gRPC proof client for samurai |
| `merkle-proofc` | gRPC proof client for merkle |
| `verkle-proofc` | gRPC proof client for verkle |
| `makedataset` | Extract modified accounts from Erigon into a flat dataset |

---

## Binaries & Commands

### `samurai`

Subcommand-based CLI for the Samurai (KZG commitment + MPT) system.

```
samurai <command> [flags]
```

| Command | Description |
|---------|-------------|
| `ingest` | Ingest blocks into the Samurai+MPT pipeline |
| `build-mpt` | Build MPT from already-processed Samurai data |
| `bench-ingest` | Duration-based ingestion benchmark with optional hot-account filtering |
| `proof` | Generate range proofs for an account |
| `serve` | Start the gRPC proof server |

#### `samurai ingest`

```bash
samurai ingest --db-dir /data/local/tmp/samurai --blocks-dir data/blocks --n 100000
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | `tmp-samurai-db-dir` | Root directory for all databases |
| `--blocks-dir` | `data/blocks` | Path to block dataset directory |
| `--n` | `10000` | Number of blocks to process |
| `--clean` | `false` | Wipe the database and start fresh |
| `--cpuprofile` | | Write CPU profile to file |

#### `samurai bench-ingest`

```bash
# Full pipeline (samurai + MPT)
samurai bench-ingest --n 50000 --duration 15m --k-users 1000 --accounts-list account_stats_all.csv

# Samurai-only (KZG commitments, no MPT bottleneck)
samurai bench-ingest --skip-mpt --n 50000 --duration 15m --k-users 1000 --accounts-list account_stats_all.csv
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | `/data/local/tmp/bench-samurai` | Root directory for databases (wiped on each run) |
| `--blocks-dir` | `data/blocks` | Path to block dataset directory |
| `--n` | `50000` | Number of blocks to ingest |
| `--duration` | `15m` | Deadline/timeout for the benchmark |
| `--k-users` | `0` | Top-K hot accounts (0 = all, no filtering) |
| `--accounts-list` | `account_stats_all.csv` | CSV with accounts sorted by update count desc |
| `--cpuprofile` | | Write CPU profile to file |
| `--skip-mpt` | `false` | Skip MPT and run samurai-only (KZG) benchmark |
| `--output-dir` | `/data/local/benchmark_output` | Root directory for benchmark output |

Output: `{output-dir}/samuraimpt/ingestion_{kUsers}_{timestamp}.csv` (default), or `{output-dir}/samurai/...` with `--skip-mpt`

#### `samurai serve`

```bash
samurai serve --db-dir /data/local/tmp/samurai --port 50051
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | | Database directory |
| `--host` | `0.0.0.0` | gRPC server host |
| `--port` | `50051` | gRPC server port |

---

### `merkle`

Subcommand-based CLI for the baseline Merkle Patricia Trie system.

```
merkle <command> [flags]
```

| Command | Description |
|---------|-------------|
| `ingest` | Ingest block data into the MPT |
| `bench-ingest` | Duration-based ingestion benchmark |
| `getproof` | Generate an eth_getProof-style account proof |
| `verifyproof` | Verify an account proof offline from JSON |
| `serve` | Start the gRPC range proof server |

#### `merkle ingest`

```bash
merkle ingest --db-dir /data/local/merkle --blocks-dir data/blocks --n 100000
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | *(required)* | Path to state database directory |
| `--blocks-dir` | `data/blocks` | Path to blocks data directory |
| `--n` | `1000` | Number of blocks to ingest |
| `--fresh` | `false` | Delete existing DB and start from scratch |

#### `merkle bench-ingest`

```bash
merkle bench-ingest --db-dir /data/local/tmp/bench-merkle --n 50000 --duration 15m --k-users 1000 --fresh
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | *(required)* | Path to state database directory |
| `--blocks-dir` | `data/blocks` | Path to blocks data directory |
| `--n` | `50000` | Number of blocks to ingest |
| `--duration` | `15m` | Deadline/timeout for the benchmark |
| `--k-users` | `0` | Top-K hot accounts (0 = all, no filtering) |
| `--accounts-list` | `account_stats_all.csv` | CSV with accounts sorted by update count desc |
| `--fresh` | `false` | Delete existing DB and start from scratch |
| `--output-dir` | `/data/local/benchmark_output` | Root directory for benchmark output |

Output: `{output-dir}/merkle/ingestion_{kUsers}_{timestamp}.csv`

#### `merkle getproof`

```bash
merkle getproof --db-dir /data/local/merkle --block 18908900 --address 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | *(required)* | Path to state database directory |
| `--db-backend` | `pebble` | Database backend: pebble or leveldb |
| `--block` | *(required)* | Block number to query |
| `--address` | *(required)* | Account address (0x hex) |
| `--verify` | `true` | Verify proof after generation |
| `--cold` | `false` | Reopen DB to simulate cold reads |

#### `merkle serve`

```bash
merkle serve --db-dir /data/local/merkle --port 50051
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | *(required)* | Path to state database directory |
| `--host` | `0.0.0.0` | gRPC server host |
| `--port` | `50051` | gRPC server port |

---

### `verkle`

Subcommand-based CLI for the baseline Verkle tree system.

```
verkle <command> [flags]
```

| Command | Description |
|---------|-------------|
| `ingest` | Ingest block data into the Verkle tree |
| `bench-ingest` | Duration-based ingestion benchmark |
| `getproof` | Generate a Verkle proof for an account |
| `verifyproof` | Verify a Verkle proof |
| `serve` | Start the gRPC range proof server |

#### `verkle ingest`

```bash
verkle ingest --db-dir /data/local/verkle --blocks-dir data/blocks --n 100000
```

#### `verkle bench-ingest`

```bash
verkle bench-ingest --db-dir /data/local/tmp/bench-verkle --n 50000 --duration 15m --k-users 1000
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | `/data/local/tmp/bench-verkle` | Path to persistent DB directory |
| `--blocks-dir` | `data/blocks` | Path to block dataset directory |
| `--db-backend` | `pebble` | DB backend: pebble or leveldb |
| `--flush-every` | `1000` | Reload tree every N blocks for memory management |
| `--n` | `50000` | Number of blocks to ingest |
| `--duration` | `15m` | Deadline/timeout for the benchmark |
| `--k-users` | `0` | Top-K hot accounts (0 = all, no filtering) |
| `--accounts-list` | `account_stats_all.csv` | CSV with accounts sorted by update count desc |
| `--output-dir` | `/data/local/benchmark_output` | Root directory for benchmark output |

Output: `{output-dir}/verkle/ingestion_{kUsers}_{timestamp}.csv`

#### `verkle serve`

```bash
verkle serve --db-dir /data/local/verkle --port 50053
```

---

### `verklekzg`

Subcommand-based CLI for the Verkle-KZG tree system (BLS12-381 + KZG commitments).

```
verklekzg <command> [flags]
```

| Command | Description |
|---------|-------------|
| `ingest` | Ingest block data into a Verkle-KZG trie |
| `bench-ingest` | Ingestion benchmark with block count and deadline |
| `getproof` | Generate a Verkle-KZG proof for an account |
| `verifyproof` | Verify a Verkle-KZG proof |
| `serve` | Start the gRPC range proof server |

#### `verklekzg ingest`

```bash
verklekzg ingest --db-dir /data/local/verklekzg --blocks-dir data/blocks --n 100000
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | `/data/local/tmp/verklekzg` | Path to persistent DB directory |
| `--blocks-dir` | `data/blocks` | Path to dataset block segments |
| `--n` | `1000` | Number of blocks to ingest |
| `--db-backend` | `pebble` | DB backend: pebble or leveldb |
| `--flush-every` | `1000` | Reload tree from DB every N blocks |
| `--fresh` | `false` | Delete existing DB and start from scratch |
| `--params-dir` | `data/params/verklekzg` | Directory for precomputed SRS/barycentric files |

#### `verklekzg bench-ingest`

```bash
verklekzg bench-ingest --db-dir /data/local/tmp/bench-verklekzg --n 50000 --duration 15m --k-users 1000
```

| Flag | Default | Description |
|------|---------|-------------|
| `--db-dir` | `/data/local/tmp/bench-verklekzg` | Path to persistent DB directory |
| `--blocks-dir` | `data/blocks` | Path to dataset block segments |
| `--db-backend` | `pebble` | DB backend: pebble or leveldb |
| `--flush-every` | `1000` | Reload tree every N blocks |
| `--n` | `50000` | Number of blocks to ingest |
| `--duration` | `15m` | Deadline/timeout for the benchmark |
| `--k-users` | `0` | Top-K hot accounts (0 = all) |
| `--accounts-list` | `account_stats_all.csv` | CSV with hot accounts sorted by update count desc |
| `--fresh` | `true` | Delete existing DB and start from scratch |
| `--output-dir` | `/data/local/benchmark_output` | Root directory for benchmark output |
| `--params-dir` | `data/params/verklekzg` | Directory for precomputed SRS/barycentric files |

Output: `{output-dir}/verklekzg/ingestion_{kUsers}_{timestamp}.csv`

---

### Proof Clients (`proofc`, `merkle-proofc`, `verkle-proofc`)

gRPC proof clients for querying and benchmarking range proofs. Each has two subcommands: `query` (single proof) and `bench` (load benchmark).

**Query a single range proof:**

```bash
proofc query --server-addr localhost:50051 --account 0x... --start-block 20 --end-block 119 --verify --params-dir ./data/params
merkle-proofc query --server-addr localhost:50051 --account 0x... --start-block 20 --end-block 119 --verify
verkle-proofc query --server-addr localhost:50053 --account 0x... --start-block 20 --end-block 119 --verify
```

**Run a proof generation benchmark:**

```bash
proofc bench --server-addr localhost:50051 --range-size 50000 --num-clients 4 --accounts-list account_stats_all.csv --duration 60s
merkle-proofc bench --server-addr localhost:50051 --range-size 50000 --num-clients 4 --accounts-list account_stats_all.csv --duration 60s
verkle-proofc bench --server-addr localhost:50053 --range-size 50000 --num-clients 4 --accounts-list account_stats_all.csv --duration 60s
```

| Flag | Default | Description |
|------|---------|-------------|
| `--server-addr` | `localhost:50051` (verkle: `50053`) | gRPC server address |
| `--range-size` | `50000` | Block range size per query |
| `--num-clients` | `1` | Concurrent client goroutines |
| `--accounts-list` | *(required)* | CSV with accounts sorted by update count desc |
| `--duration` | `60s` | Benchmark duration |
| `--verify` | `false` | Verify proofs locally |
| `--params-dir` | `./data/params` | KZG params directory (samurai only) |
| `--output-dir` | `/data/local/benchmark_output` | Root directory for benchmark output |

---

## Benchmark Output

All benchmark output is written under `{output-dir}/<protocol>/` (default `output-dir` is `/data/local/benchmark_output`):

| Protocol | Directory | Description |
|----------|-----------|-------------|
| `samurai` | `{output-dir}/samurai/` | Samurai KZG-only (`--skip-mpt`) |
| `samuraimpt` | `{output-dir}/samuraimpt/` | Samurai + MPT (default) |
| `merkle` | `{output-dir}/merkle/` | Baseline MPT |
| `verkle` | `{output-dir}/verkle/` | Baseline Verkle tree |
| `verklekzg` | `{output-dir}/verklekzg/` | Verkle-KZG tree |

| Type | File pattern | Example |
|------|-------------|---------|
| Ingestion | `ingestion_{kUsers}_{timestamp}.csv` | `samuraimpt/ingestion_1000_20260321_143022.csv` |
| Ingestion (samurai-only) | `ingestion_{kUsers}_{timestamp}.csv` | `samurai/ingestion_1000_20260321_143022.csv` |
| Update metrics | `update_metrics_{kUsers}_{timestamp}.csv` | `samurai/update_metrics_1000_20260321_143022.csv` |
| Proof | `proof_range{rangeSize}_{timestamp}.csv` | `merkle/proof_range50000_20260321_150000.csv` |

### Ingestion CSV columns

All three protocols share a common set of columns:

| Column | Description |
|--------|-------------|
| `block_num` | Block number |
| `num_raw_updates` | Total account updates in the block |
| `num_selected_updates` | Updates after hot-account filtering (equals raw if no filter) |
| `queued_at_ns` | Timestamp when block was queued (ns since epoch) |
| `start_at_ns` | Timestamp when block processing started |
| `completed_at_ns` | Timestamp when block processing completed |

Samurai adds one extra column for its parallel pipeline:

| Column | Description |
|--------|-------------|
| `wait_commitments_ns` | Time spent waiting for KZG commitment workers |

### Update Metrics CSV columns

All protocols produce update-level metrics in a separate CSV using time-windowed atomic counters (1-second windows). This captures true update throughput without the overhead of per-update CSV rows.

| Column | Description |
|--------|-------------|
| `window_end_ns` | Timestamp at end of the 1-second window (ns since epoch) |
| `updates_completed` | Number of updates completed in this window |
| `sum_compute_ns` | Total compute nanoseconds across all updates in this window |

Derived metrics (computed in post-processing):
- `update_throughput = updates_completed / window_seconds`
- `avg_update_latency_ms = (sum_compute_ns / updates_completed) / 1e6`

For Samurai, `sum_compute_ns` captures actual per-update KZG compute time. For Merkle/Verkle, it captures amortized block processing time distributed across the block's updates.

---

## Auxiliary Tools

Located under `cmd/tools/` and built with `go run`.

### `makedataset`

Extracts modified-account data from an Erigon node into a flat dataset.

```bash
./bin/makedataset [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--startBlock` | `20600000` | Starting block number |
| `--endBlock` | `21600000` | Ending block number |
| `--dataDir` | `/data/local/dataset/modified_accounts` | Output dataset directory |
| `--testMode` | `false` | Run in test/sanity-check mode |

### `count_account_updates`

Scans a block dataset and produces a CSV of per-account update counts.

```bash
go run ./cmd/tools/count_account_updates --n 10000 --dataset ./data/blocks --o account_stats.csv
```

### `debug_version`

Inspects account version entries across database shards.

```bash
go run ./cmd/tools/debug_version --datadir /data/local/samurai/db/ --account 0x... --shards 32
```

---

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build all binaries into `bin/` |
| `make build-samurai` | Build `samurai`, `proofc`, `makedataset` |
| `make build-merkle` | Build `merkle`, `merkle-proofc` |
| `make build-verkle` | Build `verkle`, `verkle-proofc` |
| `make clean` | Remove build artifacts |
| `make bench-ingest` | Run ingestion benchmarks for all three protocols |
| `make bench-ingest-samurai` | Run samurai ingestion benchmark |
| `make bench-ingest-merkle` | Run merkle ingestion benchmark |
| `make bench-ingest-verkle` | Run verkle ingestion benchmark |
| `make bench-proof` | Run proof benchmarks for all three protocols |
| `make bench-proof-samurai` | Run samurai proof benchmark (configurable via `RANGE_SIZE`, `NUM_CLIENTS`, `BENCH_DURATION`) |
| `make bench-proof-merkle` | Run merkle proof benchmark |
| `make bench-proof-verkle` | Run verkle proof benchmark |

---

## Build Protobuf Files

```bash
protoc --go_out=. --go_opt=paths=source_relative internal/tree/pb/segmenttree.proto
```

## Performance Profiling

```bash
go install github.com/google/pprof@latest
sudo apt-get install -y graphviz

./bin/samurai ingest --cpuprofile ./profiles/cpu.prof --db-dir /data/local/tmp/samurai
go tool pprof -http=:8080 ./bin/samurai ./profiles/cpu.prof
```
