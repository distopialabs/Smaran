#!/bin/bash

# Run the ingestion benchmark
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 0
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 1
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 16
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 64
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 256
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 1024
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 10000
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 50000
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 100000
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 200000
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 500000
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 1000000
echo "Sleeping for 10 seconds"
sleep 10
./bin/verkle bench-ingest --accounts-list account_stats_50k.csv --k-users 2000000