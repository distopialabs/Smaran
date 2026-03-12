# Samurai

## Introduction
...

## Quick Start

```bash
git clone https://github.com/distopialabs/Samurai.git
go mod tidy
make build
```

This produces three binaries in `bin/`:
- `samurai` — main server for committing blocks, generating proofs, and serving gRPC queries
- `proofc` — gRPC proof client for single queries and benchmarks
- `makedataset` — dataset builder (extracts modified accounts from Erigon)

---

## Binaries & Commands

### `samurai`

The main binary. Operates in one of four modes: **commit**, **proof**, **verify**, or **serve**.

```bash
./bin/samurai [flags]
```

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--mode` | string | `"commit"` | Operating mode: `commit`, `proof`, `verify`, `serve` |
| `--datadir` | string | `"samurai-data"` | Data directory for DB, profiles, and benchmarks |
| `--n` | int | `10000` | Number of blocks to process |
| `--clean` | bool | `false` | Wipe the database and start fresh |
| `--port` | int | `50051` | gRPC server port (serve mode) |
| `--p` | bool | `false` | Enable CPU profiling |
| `--profilePath` | string | `<datadir>/profiles` | Profile output path |
| `--queryStartBlock` | int | `18908915` | Start block for proof/verify mode |
| `--queryEndBlock` | int | `18909914` | End block for proof/verify mode |
| `--queryAccount` | string | `"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"` | Account address for proof/verify mode |
| `--bench` | bool | `false` | Enable benchmarking during commit |
| `--benchDuration` | int | `300` | Benchmark duration in seconds |
| `--benchOutputDir` | string | `<datadir>/benchmarks` | Benchmark CSV output directory |
| `--benchDBMetrics` | bool | `false` | Collect Pebble DB metrics during benchmark |
| `--benchPipeline` | bool | `false` | Collect pipeline sizes per shard during benchmark |
| `--benchCacheMetrics` | bool | `true` | Collect Ristretto cache metrics during benchmark |

#### Examples

Commit 100 blocks:
```bash
./bin/samurai --mode commit --n 100
```

Commit all blocks with a clean database:
```bash
./bin/samurai --datadir /data/local/samurai --n 2616996 --clean
```

Start the gRPC server:
```bash
./bin/samurai --mode serve --datadir /data/local/samurai --port 50051
```

Generate a proof:
```bash
./bin/samurai --mode proof --queryAccount 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2 --queryStartBlock 18908915 --queryEndBlock 18909914
```

Run a commit benchmark with DB metrics:
```bash
./bin/samurai --bench --benchDuration 300 --benchOutputDir ./benchmark_output --benchDBMetrics
```

---

### `proofc`

gRPC proof client. Supports single proof queries and three benchmark modes: **range**, **concurrency**, and **stress**.

```bash
./bin/proofc [flags]
```

#### Common Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server` | string | `"localhost:50051"` | gRPC server address |
| `--account` | string | `"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"` | Account address to query |
| `--start-block` | uint64 | `20` | Start block (relative to data start) |
| `--end-block` | uint64 | `119` | End block (relative to data start) |
| `--params-dir` | string | `"./data/params"` | Path to cryptographic parameters |
| `--dump-json` | string | `""` | Dump response to a JSON file |
| `--dump-bin` | string | `""` | Dump response as binary protobuf |

#### Benchmark Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--benchmark` | bool | `false` | Enable benchmark mode |
| `--mode` | string | `"range"` | Benchmark mode: `range`, `concurrency`, `stress` |
| `--output-dir` | string | `"./benchmark_output"` | Benchmark output directory |
| `--accounts-file` | string | `"cmd/proofc/top_1k_accounts_all_blocks.csv"` | CSV of accounts for benchmarks |
| `--verify` | bool | `false` | Include verification time (range mode) |
| `--range-size` | uint64 | `50000` | Block range size (range mode) |
| `--levels` | string | `"1,5,10,20,50,100"` | Comma-separated concurrency levels (concurrency mode) |
| `--stress-duration` | duration | `5m` | Duration of stress test |
| `--stress-clients` | int | `10` | Number of concurrent clients (stress mode) |

#### Examples

Single proof query with verification:
```bash
./bin/proofc --server localhost:50051 --account 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2 --start-block 20 --end-block 119
```

Range benchmark:
```bash
./bin/proofc --benchmark --mode range --range-size 50000 --output-dir ./benchmark_output
```

Range benchmark with verification:
```bash
./bin/proofc --benchmark --mode range --range-size 50000 --verify --params-dir ./data/params
```

Concurrency benchmark:
```bash
./bin/proofc --benchmark --mode concurrency --levels 1,5,10,50,100 --accounts-file ./cmd/proofc/top_1k_accounts_200k_blocks.csv
```

Stress test:
```bash
./bin/proofc --benchmark --mode stress --stress-duration 5m --stress-clients 50
```

---

### `makedataset`

Extracts modified-account data from an Erigon node into a flat dataset.

```bash
./bin/makedataset [flags]
```

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--startBlock` | int | `20600000` | Starting block number |
| `--endBlock` | int | `21600000` | Ending block number |
| `--dataDir` | string | `"/data/local/dataset/modified_accounts"` | Output dataset directory |
| `--testMode` | bool | `false` | Run in test/sanity-check mode |

---

## Auxiliary Tools

Located under `cmd/tools/` and built with `go run`.

### `count_account_updates`

Scans a block dataset and produces a CSV of per-account update counts.

```bash
go run ./cmd/tools/count_account_updates [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--n` | int | `10000` | Number of blocks to process |
| `--start` | int | `18908895` | Starting block number |
| `--o` | string | `"account_stats.csv"` | Output CSV path |
| `--dataset` | string | `"./data/blocks"` | Blocks dataset directory |

### `debug_version`

Inspects account version entries across database shards.

```bash
go run ./cmd/tools/debug_version [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--datadir` | string | `"/data/local/samurai/db/"` | Data directory containing shard DBs |
| `--account` | string | *(required)* | Account address (hex) |
| `--shards` | int | `32` | Number of database shards |

### `stress`

Stress-tests dataset reads.

```bash
go run ./cmd/stress [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--n` | int | `100` | Number of blocks to fetch |
| `--dataDir` | string | `"/data/local/dataset/modified_accounts"` | Dataset path |

### `mptproofs`

Fetches, extracts, and verifies Merkle Patricia Trie proofs from an Alchemy/RPC endpoint.

```bash
go run ./cmd/tools/mptproofs [flags]
```

#### Main Flag

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--mode` | string | `"extract_proofs"` | Mode: `fetch_proofs`, `extract_proofs`, `verify_proofs` |

#### `fetch_proofs` Mode

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--alchemy` | string | *(Alchemy URL)* | Alchemy HTTPS JSON-RPC endpoint |
| `--addr` | string | `"0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045"` | Account address |
| `--start` | uint64 | `18908895` | Start block (inclusive) |
| `--end` | uint64 | `19108895` | End block (inclusive) |
| `--out` | string | `"/mydata/samurai/exp1/"` | Output directory |
| `--rps` | int | `20` | Requests per second (max 25 for Alchemy) |

#### `verify_proofs` Mode

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--account` | string | `"0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045"` | Account address |
| `--start` | uint64 | `18908895` | Start block (inclusive) |
| `--end` | uint64 | `19108894` | End block (inclusive) |
| `--dir` | string | `"/mydata/samurai/exp1/proofs"` | Directory containing `.proof` files |
| `--rpc` | string | `"/mydata/erigon/mainnet/erigon.ipc"` | RPC endpoint (HTTP, WebSocket, or IPC) |
| `--concurrency` | int | `1` | Number of concurrent verifications |

#### `extract_proofs` Mode

No CLI flags. Uses hardcoded paths in `extract_proofs.go`.

---

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build `samurai`, `proofc`, and `makedataset` into `bin/` |
| `make clean` | Remove build artifacts |
| `make commit` | Run samurai commit (1 block, background) |
| `make commit-clean` | Run samurai commit with `--clean` (full dataset, background) |
| `make serve` | Start the gRPC proof server |
| `make run-bench` | Run a commit benchmark (300s, with DB metrics) |
| `make bench-range` | Run a proof-server range benchmark |
| `make bench-range-verify` | Run a proof-server range benchmark with verification |
| `make bench-concurrency` | Run a proof-server concurrency benchmark |
| `make bench-stress` | Run a proof-server stress test (5m, 50 clients) |
| `make bench-proof-all` | Run range + concurrency benchmarks |
| `make plot-graphs` | Plot benchmark results (requires Python + matplotlib) |

---

## Build Protobuf Files

```bash
protoc --go_out=. --go_opt=paths=source_relative internal/tree/pb/segmenttree.proto
```

## Performance Profiling

```bash
go install github.com/google/pprof@latest
sudo apt-get install -y graphviz

./bin/samurai --p --profilePath ./profiles
go tool pprof -http=:8080 ./bin/samurai ./profiles/cpu.prof
```
