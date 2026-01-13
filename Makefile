# plot grapg

plot-graphs:
	python scripts/benchmark/plot_bench.py --updates ./benchmark_output/bench_updates_20260112_195414.csv --blocks ./benchmark_output/bench_blocks_20260112_195414.csv --warmup 0 --cooldown 0 --output ./benchmark_output/plots/


	go run ./cmd/samurai/ -bench -benchDuration 300 -benchOutputDir ./benchmark_output -benchDBMetrics