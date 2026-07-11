#!/bin/bash
python scripts/benchmark/plot_bench.py \
  ingestion-timeseries \
  --output-dir \
  plots/2layers/ingestion_timeseries/1 \
  --warmup \
  15.0 \
  --cooldown \
  15.0 \
  --format \
  png \
  --window \
  5.0 \
  --input \
  samurai:/data/local/benchmark_output/samurai_2layer/samurai/ingestion_1_20260324_205328.csv \
  --input \
  samuraimpt:/data/local/benchmark_output/samurai_2layer/samuraimpt/ingestion_1_20260324_204759.csv \
  --input \
  verkle:/data/local/benchmark_output/verkle/ingestion_1_20260322_233442.csv