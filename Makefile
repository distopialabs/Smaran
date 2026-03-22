.PHONY: all build build-samurai build-merkle build-verkle clean proto \
        bench-ingest-samurai bench-ingest-merkle bench-ingest-verkle bench-ingest \
        bench-proof-samurai bench-proof-merkle bench-proof-verkle bench-proof

BUILD_DIR      := bin
BLOCKS_DIR     := data/blocks
ACCOUNTS_LIST  ?= account_stats_all.csv
RANGE_SIZE     ?= 50000
NUM_CLIENTS    ?= 1
BENCH_DURATION ?= 60s

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

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       api/proto/samurai/v1/proof_service.proto \
	       api/proto/merkle/v1/proof_service.proto \
	       api/proto/verkle/v1/proof_service.proto

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

# --- Proof benchmark targets ---
# Output goes to benchmark_output/<protocol>/proof_range<rangeSize>_<timestamp>.txt

bench-proof-samurai: build-samurai
	./$(BUILD_DIR)/proofc bench \
		--server-addr localhost:50051 \
		--range-size $(RANGE_SIZE) \
		--num-clients $(NUM_CLIENTS) \
		--accounts-list $(ACCOUNTS_LIST) \
		--duration $(BENCH_DURATION)

bench-proof-merkle: build-merkle
	./$(BUILD_DIR)/merkle-proofc bench \
		--server-addr localhost:50051 \
		--range-size $(RANGE_SIZE) \
		--num-clients $(NUM_CLIENTS) \
		--accounts-list $(ACCOUNTS_LIST) \
		--duration $(BENCH_DURATION)

bench-proof-verkle: build-verkle
	./$(BUILD_DIR)/verkle-proofc bench \
		--server-addr localhost:50053 \
		--range-size $(RANGE_SIZE) \
		--num-clients $(NUM_CLIENTS) \
		--accounts-list $(ACCOUNTS_LIST) \
		--duration $(BENCH_DURATION)

bench-proof: bench-proof-samurai bench-proof-merkle bench-proof-verkle
