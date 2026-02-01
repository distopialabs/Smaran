.PHONY: all build clean run-bench plot-graphs

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

# plot graphs
plot-graphs:
	python scripts/benchmark/plot_bench.py --updates ./benchmark_output/bench_updates_20260112_195414.csv --blocks ./benchmark_output/bench_blocks_20260112_195414.csv --warmup 0 --cooldown 0 --output ./benchmark_output/plots/

run-bench: build
	./$(BUILD_DIR)/samurai -bench -benchDuration 300 -benchOutputDir ./benchmark_output -benchDBMetrics