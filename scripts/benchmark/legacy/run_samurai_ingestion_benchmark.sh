#!/bin/bash

# Run the ingestion benchmark
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 0 --duration 5m
sleep 10
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 1000 --duration 5m
sleep 10
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 10000 --duration 5m
sleep 10
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 100000 --duration 5m
sleep 10
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 1000000 --duration 5m
sleep 10

# Samurai-only
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 1000 --skip-mpt --duration 5m
sleep 10
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 10000 --skip-mpt --duration 5m
sleep 10
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 100000 --skip-mpt --duration 5m
sleep 10
./bin/samurai bench-ingest --accounts-list account_stats_all.csv --k-users 1000000 --skip-mpt --duration 5m
sleep 10


./bin/merkle bench-ingest --accounts-list account_stats_all.csv --k-users 0 --duration 5m
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_all.csv --k-users 1000 --duration 5m
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_all.csv --k-users 10000 --duration 5m
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_all.csv --k-users 100000 --duration 5m
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_all.csv --k-users 1000000 --duration 5m
sleep 10

./bin/verkle bench-ingest --accounts-list account_stats_all.csv --k-users 0 --duration 5m
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_all.csv --k-users 1000 --duration 5m
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_all.csv --k-users 10000 --duration 5m
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_all.csv --k-users 100000 --duration 5m
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_all.csv --k-users 1000000 --duration 5m
sleep 10




