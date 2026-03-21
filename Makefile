.PHONY: all build build-samurai build-merkle build-verkle clean \
        run-bench plot-graphs \
        bench-range bench-concurrency bench-stress bench-proof-all \
        bench-ingest-merkle bench-ingest-verkle bench-ingest \
        bench-proof-merkle bench-proof-verkle bench-proof

BUILD_DIR := bin

all: build

# --- Build targets ---

build: build-samurai build-merkle build-verkle

build-samurai:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/samurai ./cmd/samurai
	go build -o $(BUILD_DIR)/samuraimpt ./cmd/samuraimpt
	go build -o $(BUILD_DIR)/proofc ./cmd/proofc
	go build -o $(BUILD_DIR)/makedataset ./cmd/tools/makedataset
	@echo "Samurai build artifacts in $(BUILD_DIR)/"

build-merkle:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/merkle ./cmd/merkle
	go build -o $(BUILD_DIR)/merkle-proofc ./cmd/merkle-proofc
	@echo "Merkle build artifacts in $(BUILD_DIR)/"

build-verkle:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/verkle ./cmd/verkle
	go build -o $(BUILD_DIR)/verkle-proofc ./cmd/verkle-proofc
	@echo "Verkle build artifacts in $(BUILD_DIR)/"

clean:
	rm -rf $(BUILD_DIR)

commit:
	nohup ./$(BUILD_DIR)/samurai --datadir /data/local/samurai-keccak --n 1 > /data/local/run.log 2>&1 &

commit-clean:
	nohup ./$(BUILD_DIR)/samurai --datadir /data/local/samurai-keccak --n 2616996 --clean > /data/local/run.log 2>&1 &

serve:
	./$(BUILD_DIR)/samurai --datadir /data/local/samurai-keccak --mode serve

# --- Samurai benchmark targets ---

bench-range: build-samurai
	./$(BUILD_DIR)/proofc --benchmark --mode range --output-dir ./benchmark_output/samurai --datadir /data/local/samurai

bench-range-verify: build-samurai
	./$(BUILD_DIR)/proofc --benchmark --mode range --verify --params-dir ./data/params --output-dir ./benchmark_output/samurai --datadir /data/local/samurai

bench-concurrency: build-samurai
	./$(BUILD_DIR)/proofc --benchmark --mode concurrency --output-dir ./benchmark_output/samurai

bench-stress: build-samurai
	./$(BUILD_DIR)/proofc --benchmark --mode stress --stress-duration 5m --stress-clients 50 --output-dir ./benchmark_output/samurai

bench-proof-all: bench-range bench-concurrency

# Commit generation benchmark
run-bench: build-samurai
	./$(BUILD_DIR)/samurai -bench -benchDuration 300 -benchOutputDir ./benchmark_output/samurai -benchDBMetrics

# --- Merkle benchmark targets ---

bench-ingest-merkle: build-merkle
	./$(BUILD_DIR)/merkle bench-ingest --db-dir /data/local/merkle --blocks-dir /data/local/blocks --output benchmark_output/merkle/bench_ingest.csv

bench-proof-merkle: build-merkle
	./$(BUILD_DIR)/merkle-proofc --benchmark --mode range --output-dir ./benchmark_output/merkle
	./$(BUILD_DIR)/merkle-proofc --benchmark --mode concurrency --output-dir ./benchmark_output/merkle

# --- Verkle benchmark targets ---

bench-ingest-verkle: build-verkle
	./$(BUILD_DIR)/verkle bench-ingest --db-dir /data/local/verkle --blocks-dir /data/local/blocks --output benchmark_output/verkle/bench_ingest.csv

bench-proof-verkle: build-verkle
	./$(BUILD_DIR)/verkle-proofc --benchmark --mode range --output-dir ./benchmark_output/verkle
	./$(BUILD_DIR)/verkle-proofc --benchmark --mode concurrency --output-dir ./benchmark_output/verkle

# --- Cross-system benchmarks ---

bench-ingest: bench-ingest-merkle bench-ingest-verkle run-bench

bench-proof: bench-proof-merkle bench-proof-verkle bench-proof-all

# --- Visualization ---

plot-graphs:
	python scripts/benchmark/plot_bench.py --updates ./benchmark_output/samurai/bench_updates_*.csv --blocks ./benchmark_output/samurai/bench_blocks_*.csv --warmup 0 --cooldown 0 --output ./benchmark_output/samurai/plots/