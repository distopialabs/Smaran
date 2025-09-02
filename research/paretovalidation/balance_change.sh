#!/bin/bash

# Ensure directories exist
mkdir -p ./logs
mkdir -p ./logs/console

go run ./cmd/balance-changes -numBlocks 1000  -out ./logs/balance_changes_1k.json
go run .  -out ./logs/balance_changes_1k.json

go run ./cmd/balance-changes -numBlocks 200000  -out ./logs/balance_changes_200k.json

go run ./cmd/balance-changes -numBlocks 2000000  -out ./logs/balance_changes_2m.json
