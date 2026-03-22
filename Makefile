.PHONY: all build build-samurai build-merkle build-verkle clean \
        bench-ingest-samurai bench-ingest-merkle bench-ingest-verkle bench-ingest \
        bench-proof-samurai bench-proof-merkle bench-proof-verkle bench-proof

BUILD_DIR  := bin
BLOCKS_DIR := data/blocks

all: build

# --- Build targets ---

build: build-samurai build-merkle build-verkle

build-samurai:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/samurai ./cmd/samurai
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

# --- Ingestion benchmark targets ---
# Output goes to benchmark_output/<protocol>/ingestion_<kUsers>_<timestamp>.csv

bench-ingest-samurai: build-samurai
	./$(BUILD_DIR)/samurai bench-ingest \
		--blocks-dir $(BLOCKS_DIR) \
		--duration 5m

bench-ingest-merkle: build-merkle
	./$(BUILD_DIR)/merkle bench-ingest \
		--db-dir /data/local/tmp/bench-merkle \
		--blocks-dir $(BLOCKS_DIR) \
		--duration 5m --fresh

bench-ingest-verkle: build-verkle
	./$(BUILD_DIR)/verkle bench-ingest \
		--blocks-dir $(BLOCKS_DIR) \
		--duration 5m

bench-ingest: bench-ingest-samurai bench-ingest-merkle bench-ingest-verkle

# --- Proof benchmark targets (proof clients, pre-refactor) ---

bench-proof-samurai: build-samurai
	./$(BUILD_DIR)/proofc --benchmark --mode range --output-dir ./benchmark_output/samurai

bench-proof-merkle: build-merkle
	./$(BUILD_DIR)/merkle-proofc --benchmark --mode range --output-dir ./benchmark_output/merkle

bench-proof-verkle: build-verkle
	./$(BUILD_DIR)/verkle-proofc --benchmark --mode range --output-dir ./benchmark_output/verkle

bench-proof: bench-proof-samurai bench-proof-merkle bench-proof-verkle
