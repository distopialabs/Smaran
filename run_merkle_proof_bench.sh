#!/bin/bash

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 1

sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 100

sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 500

sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 1000

sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 5000


sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 7000


sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 50000


sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 200000

sleep 5

./bin/merkle-proofc bench --verify --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --num-clients 16 --duration 2m --range-size 600000
