#!/bin/bash

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 1

sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 100

sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 500

sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 1000

sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 5000


sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 7000


sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 50000


sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 200000

sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 600000

sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 1200000


sleep 5

./bin/proofc bench --server-addr clnode332.clemson.cloudlab.us:50051 --accounts-list account_stats_all.csv --state-root 0x86b81a0941f80ae66edae768f79eeff3f96613518fc73b97795e9214f516aac4 --num-clients 16 --duration 2m --range-size 2600000


