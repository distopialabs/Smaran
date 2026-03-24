#!/bin/bash

# Run the samurai-only (KZG) ingestion benchmark (no MPT bottleneck)
./bin/samurai bench-ingest --skip-mpt --accounts-list account_stats_50k.csv --k-users 10000
sleep 10
./bin/samurai bench-ingest --skip-mpt --accounts-list account_stats_50k.csv --k-users 50000
sleep 10
./bin/samurai bench-ingest --skip-mpt --accounts-list account_stats_50k.csv --k-users 100000
sleep 10
./bin/samurai bench-ingest --skip-mpt --accounts-list account_stats_50k.csv --k-users 200000
sleep 10
./bin/samurai bench-ingest --skip-mpt --accounts-list account_stats_50k.csv --k-users 500000
sleep 10
./bin/samurai bench-ingest --skip-mpt --accounts-list account_stats_50k.csv --k-users 1000000
sleep 10
./bin/samurai bench-ingest --skip-mpt --accounts-list account_stats_50k.csv --k-users 2000000
