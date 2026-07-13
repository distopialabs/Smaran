.PHONY: all build build-samurai build-merkle build-verkle clean proto \
        bench-ingest-samurai bench-ingest-merkle bench-ingest-verkle bench-ingest \
        bench-proof-samurai bench-proof-merkle bench-proof-verkle bench-proof bench-proof-verklekzg bench-ingest-verklekzg build-verklekzg

export PATH := $(HOME)/go/bin:$(PATH)

BUILD_DIR      := bin
BLOCKS_DIR     := data/blocks
ACCOUNTS_LIST  ?= account_stats_all.csv
RANGE_SIZE     ?= 50000
NUM_CLIENTS    ?= 1
BENCH_DURATION ?= 60s

all: build

# --- Build targets ---

build: build-samurai build-merkle build-verkle build-verklekzg

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

build-verklekzg:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/verklekzg ./cmd/verklekzg
	go build -o $(BUILD_DIR)/verklekzg-proofc ./cmd/verklekzg-proofc
	@echo "Verkle-KZG build artifacts in $(BUILD_DIR)/"

clean:
	rm -rf $(BUILD_DIR)

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       api/proto/samurai/v1/proof_service.proto
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       api/proto/merkle/v1/proof_service.proto
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
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

bench-ingest-verklekzg: build-verklekzg
	./$(BUILD_DIR)/verklekzg bench-ingest \
		--blocks-dir $(BLOCKS_DIR) \
		--duration 5m

bench-ingest: bench-ingest-samurai bench-ingest-merkle bench-ingest-verkle bench-ingest-verklekzg

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

bench-proof-verklekzg: build-verklekzg
	./$(BUILD_DIR)/verklekzg-proofc bench \
		--server-addr localhost:50053 \
		--range-size $(RANGE_SIZE) \
		--num-clients $(NUM_CLIENTS) \
		--accounts-list $(ACCOUNTS_LIST) \
		--duration $(BENCH_DURATION)

bench-proof: bench-proof-samurai bench-proof-merkle bench-proof-verkle bench-proof-verklekzg

# --- Key Transparency (Section 7.1) targets ---
# ktserver/ktbench are the KT experiment binaries; the coniks targets
# build and configure the CONIKS baseline from the Coniks/ submodule.

.PHONY: build-kt coniks setup-coniks-server setup-coniks-client \
        run-coniks-server stop-coniks-server run-coniks-client

# DL's BUILD_DIR is relative; the coniks targets cd into config dirs, so
# they need absolute paths.
KT_BIN := $(CURDIR)/$(BUILD_DIR)
CONIKS_SERVER_DIR ?= $(KT_BIN)/coniks-server-config
CONIKS_CLIENT_DIR ?= $(KT_BIN)/coniks-client-config
SED = $(shell which gsed 2>/dev/null || which sed)

build-kt:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/ktserver ./cmd/ktserver
	go build -o $(BUILD_DIR)/ktbench ./cmd/ktbench
	@echo "KT build artifacts in $(BUILD_DIR)/"

coniks:
	mkdir -p $(BUILD_DIR)
	$(MAKE) -C Coniks
	cp Coniks/build/coniksbench $(BUILD_DIR)/
	cp Coniks/build/coniksserver $(BUILD_DIR)/
	cp Coniks/build/coniksbot $(BUILD_DIR)/
	cp Coniks/build/coniksclient $(BUILD_DIR)/

setup-coniks-server: coniks
	mkdir -p $(CONIKS_SERVER_DIR)
	rm -rf $(CONIKS_SERVER_DIR)/*
	cd $(CONIKS_SERVER_DIR) && $(KT_BIN)/coniksserver init -c
	rm -rf $(CONIKS_SERVER_DIR)/init.str
	echo "  allow_registration = true" >> $(CONIKS_SERVER_DIR)/config.toml
	$(SED) -i 's|epoch_deadline = 0|epoch_deadline = 6000|g' $(CONIKS_SERVER_DIR)/config.toml

setup-coniks-client: coniks
	mkdir -p $(CONIKS_CLIENT_DIR)
	rm -rf $(CONIKS_CLIENT_DIR)/*
	cd $(CONIKS_CLIENT_DIR) && $(KT_BIN)/coniksclient init
	cd $(CONIKS_CLIENT_DIR) && $(SED) -i 's|../keyserver/||g' config.toml
	cd $(CONIKS_CLIENT_DIR) && $(SED) -i 's|../coniksserver|$(CONIKS_SERVER_DIR)|g' config.toml

run-coniks-server: coniks $(CONIKS_SERVER_DIR)
	rm -rf $(CONIKS_SERVER_DIR)/init.str
	cd $(CONIKS_SERVER_DIR) && $(KT_BIN)/coniksserver run -p

stop-coniks-server: $(CONIKS_SERVER_DIR)
	kill -USR2 $(shell cat $(CONIKS_SERVER_DIR)/coniks.pid)

run-coniks-client: coniks $(CONIKS_CLIENT_DIR)
	cd $(CONIKS_CLIENT_DIR) && $(KT_BIN)/coniksclient run
