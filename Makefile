.PHONY: all build clean run-bench plot-graphs bench-range bench-concurrency bench-stress bench-proof-all

BUILD_DIR := bin

all: build

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/samurai ./cmd/samurai
	go build -o $(BUILD_DIR)/proofc ./cmd/proofc
	go build -o $(BUILD_DIR)/makedataset ./cmd/tools/makedataset
	@echo "Build artifacts in $(BUILD_DIR)/"

clean:
	rm -rf $(BUILD_DIR)

# Proof server benchmark targets
bench-range: build
	./$(BUILD_DIR)/proofc --benchmark --mode range --output-dir ./benchmark_output

bench-range-verify: build
	./$(BUILD_DIR)/proofc --benchmark --mode range --verify --params-dir ./data/params --output-dir ./benchmark_output

bench-concurrency: build
	./$(BUILD_DIR)/proofc --benchmark --mode concurrency --output-dir ./benchmark_output

bench-stress: build
	./$(BUILD_DIR)/proofc --benchmark --mode stress --stress-duration 5m --stress-clients 50 --output-dir ./benchmark_output

bench-proof-all: bench-range bench-concurrency

# Commit generation benchmark
run-bench: build
	./$(BUILD_DIR)/samurai -bench -benchDuration 300 -benchOutputDir ./benchmark_output -benchDBMetrics

# Plot graphs
plot-graphs:
	python scripts/benchmark/plot_bench.py --updates ./benchmark_output/bench_updates_20260112_195414.csv --blocks ./benchmark_output/bench_blocks_20260112_195414.csv --warmup 0 --cooldown 0 --output ./benchmark_output/plots/