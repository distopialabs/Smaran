#!/bin/bash

# Run the ingestion benchmark
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 0
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 1
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 16
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 64
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 256
sleep 10


./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 200000
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 500000
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 1000000
sleep 10
./bin/merkle bench-ingest --accounts-list account_stats_50k.csv --k-users 2000000