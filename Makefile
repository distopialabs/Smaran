.PHONY: all build clean run-bench plot-graphs bench-range bench-concurrency bench-stress bench-proof-all coniks

BUILD_DIR := $(CURDIR)/bin
CONIKS_SERVER_DIR ?= $(BUILD_DIR)/coniks-server-config
CONIKS_CLIENT_DIR ?= $(BUILD_DIR)/coniks-client-config


SED = $(shell which gsed 2>/dev/null || which sed)

all: build

build: coniks setup-coniks-server setup-coniks-client
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/samurai ./cmd/samurai
	go build -o $(BUILD_DIR)/proofc ./cmd/proofc
	go build -o $(BUILD_DIR)/makedataset ./cmd/tools/makedataset
	go build -o $(BUILD_DIR)/ktserver ./cmd/ktserver
	go build -o $(BUILD_DIR)/ktbench ./cmd/ktbench
	@echo "Build artifacts in $(BUILD_DIR)/"


coniks:
	$(MAKE) -C Coniks
	cp Coniks/build/coniksbench $(BUILD_DIR)/
	cp Coniks/build/coniksserver $(BUILD_DIR)/
	cp Coniks/build/coniksbot $(BUILD_DIR)/
	cp Coniks/build/coniksclient $(BUILD_DIR)/

clean:
	rm -rf $(BUILD_DIR)

commit:
	nohup ./$(BUILD_DIR)/samurai --datadir /data/local/samurai-keccak --n 1 > /data/local/run.log 2>&1 &

commit-clean:
	nohup ./$(BUILD_DIR)/samurai --datadir /data/local/samurai-keccak --n 2616996 --clean > /data/local/run.log 2>&1 &

serve:
	./$(BUILD_DIR)/samurai --datadir /data/local/samurai-keccak --mode serve

# Proof server benchmark targets
bench-range: build
	./$(BUILD_DIR)/proofc --benchmark --mode range --output-dir ./benchmark_output --datadir /data/local/samurai

bench-range-verify: build
	./$(BUILD_DIR)/proofc --benchmark --mode range --verify --params-dir ./data/params --output-dir ./benchmark_output --datadir /data/local/samurai

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


.PHONY: run-coniks-server
run-coniks-server: coniks $(CONIKS_SERVER_DIR)
	rm -rf $(CONIKS_SERVER_DIR)/init.str
	cd $(CONIKS_SERVER_DIR) && $(BUILD_DIR)/coniksserver run -p

.PHONY: stop-coniks-server
stop-coniks-server: $(CONIKS_SERVER_DIR)
	kill -USR2 $(shell cat $(CONIKS_SERVER_DIR)/coniks.pid)


.PHONY: setup-coniks-client
setup-coniks-client: coniks
	mkdir -p $(CONIKS_CLIENT_DIR)
	rm -rf $(CONIKS_CLIENT_DIR)/*
	cd $(CONIKS_CLIENT_DIR) && $(BUILD_DIR)/coniksclient init
	cd $(CONIKS_CLIENT_DIR) && $(SED) -i 's|../keyserver/||g' config.toml
	cd $(CONIKS_CLIENT_DIR) && $(SED) -i 's|../coniksserver|$(CONIKS_SERVER_DIR)|g' config.toml


.PHONY: run-coniks-client
run-coniks-client: coniks $(CONIKS_CLIENT_DIR)
	cd $(CONIKS_CLIENT_DIR) && $(BUILD_DIR)/coniksclient run


.PHONY: setup-coniks-server
setup-coniks-server: coniks
	mkdir -p $(CONIKS_SERVER_DIR)
	rm -rf $(CONIKS_SERVER_DIR)/*
	cd $(CONIKS_SERVER_DIR) && $(BUILD_DIR)/coniksserver init -c
	rm -rf $(CONIKS_SERVER_DIR)/init.str
	echo "  allow_registration = true" >> $(CONIKS_SERVER_DIR)/config.toml
	$(SED) -i 's|epoch_deadline = 0|epoch_deadline = 6000|g' $(CONIKS_SERVER_DIR)/config.toml
