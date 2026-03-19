# Count Account Updates

This tool counts the number of updates for each account in the dataset.

## Usage

```bash
go run cmd/tools/count_account_updates/main.go -n <num_blocks> -start <start_block> -o <output_file> -dataset <dataset_dir>
```

## Flags

- `-n`: Number of blocks to process (default: 10000)
- `-start`: Starting block number (default: 18908895)
- `-o`: Output file path for account statistics (default: account_stats.csv)
- `-dataset`: Path to blocks dataset directory (default: ./data/blocks)