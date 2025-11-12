#!/usr/bin/env python3
"""
Analyze account_seen_count.log to extract:
1. Count of update tasks sent to each worker
2. For each account, count how many times it was seen and which workers it was sent to
"""

import re
from collections import defaultdict
import sys

def parse_log_line(line):
    """
    Parse a log line and extract account address and worker ID.
    Expected format: "Sending update task for account <address> to worker <id>"
    Returns: (account_address, worker_id) or (None, None) if parsing fails
    """
    # Pattern to match: "Sending update task for account <address> to worker <id>"
    pattern = r'Sending update task for account (0x[a-fA-F0-9]+) to worker (\d+)'
    match = re.search(pattern, line)
    
    if match:
        account = match.group(1)
        worker_id = int(match.group(2))
        return account, worker_id
    
    return None, None

def analyze_log(log_file_path):
    """
    Analyze the log file and return statistics.
    """
    worker_task_counts = defaultdict(int)  # worker_id -> count of tasks
    account_info = defaultdict(lambda: {'count': 0, 'workers': set()})  # account -> {count, workers set}
    
    total_lines = 0
    parsed_lines = 0
    
    print(f"Analyzing log file: {log_file_path}")
    print("Processing...")
    
    with open(log_file_path, 'r') as f:
        for line_num, line in enumerate(f, 1):
            total_lines += 1
            
            # Progress indicator every 100k lines
            if line_num % 100000 == 0:
                print(f"  Processed {line_num:,} lines...", file=sys.stderr)
            
            account, worker_id = parse_log_line(line)
            
            if account and worker_id is not None:
                parsed_lines += 1
                
                # Count tasks per worker
                worker_task_counts[worker_id] += 1
                
                # Track account occurrences and workers
                account_info[account]['count'] += 1
                account_info[account]['workers'].add(worker_id)
    
    print(f"\nProcessing complete!")
    print(f"Total lines: {total_lines:,}")
    print(f"Successfully parsed lines: {parsed_lines:,}")
    print(f"Unique accounts: {len(account_info):,}")
    print(f"Workers used: {len(worker_task_counts)}")
    print()
    
    return worker_task_counts, account_info

def print_worker_statistics(worker_task_counts):
    """
    Print statistics about tasks sent to each worker.
    """
    print("=" * 80)
    print("WORKER TASK STATISTICS")
    print("=" * 80)
    
    # Sort by worker ID
    sorted_workers = sorted(worker_task_counts.items())
    
    print(f"\n{'Worker ID':<15} {'Task Count':<15}")
    print("-" * 30)
    
    for worker_id, count in sorted_workers:
        print(f"{worker_id:<15} {count:<15,}")
    
    # Summary statistics
    total_tasks = sum(worker_task_counts.values())
    avg_tasks = total_tasks / len(worker_task_counts) if worker_task_counts else 0
    max_tasks = max(worker_task_counts.values()) if worker_task_counts else 0
    min_tasks = min(worker_task_counts.values()) if worker_task_counts else 0
    
    print("\n" + "-" * 30)
    print(f"Total tasks: {total_tasks:,}")
    print(f"Average tasks per worker: {avg_tasks:,.2f}")
    print(f"Max tasks (single worker): {max_tasks:,}")
    print(f"Min tasks (single worker): {min_tasks:,}")
    print()

def print_account_statistics(account_info, top_n=50):
    """
    Print statistics about accounts.
    """
    print("=" * 80)
    print("ACCOUNT STATISTICS")
    print("=" * 80)
    
    # Sort by count (most frequent first)
    sorted_accounts = sorted(account_info.items(), key=lambda x: x[1]['count'], reverse=True)
    
    print(f"\nTotal unique accounts: {len(account_info):,}")
    print(f"\nTop {top_n} most frequently seen accounts:")
    print(f"\n{'Account Address':<45} {'Count':<10} {'Workers':<30}")
    print("-" * 90)
    
    for i, (account, info) in enumerate(sorted_accounts[:top_n], 1):
        workers_str = ', '.join(map(str, sorted(list(info['workers']))[:10]))
        if len(info['workers']) > 10:
            workers_str += f", ... ({len(info['workers'])} total)"
        print(f"{account:<45} {info['count']:<10,} {workers_str}")
    
    # Statistics about account distribution
    print("\n" + "-" * 90)
    
    # Accounts seen only once
    single_occurrence = sum(1 for info in account_info.values() if info['count'] == 1)
    
    # Accounts sent to single worker
    single_worker = sum(1 for info in account_info.values() if len(info['workers']) == 1)
    
    # Accounts sent to multiple workers
    multiple_workers = sum(1 for info in account_info.values() if len(info['workers']) > 1)
    
    print(f"Accounts seen only once: {single_occurrence:,} ({100*single_occurrence/len(account_info):.2f}%)")
    print(f"Accounts sent to single worker: {single_worker:,} ({100*single_worker/len(account_info):.2f}%)")
    print(f"Accounts sent to multiple workers: {multiple_workers:,} ({100*multiple_workers/len(account_info):.2f}%)")
    
    # Max workers for any account
    max_workers = max(len(info['workers']) for info in account_info.values())
    max_count = max(info['count'] for info in account_info.values())
    
    print(f"Max workers assigned to single account: {max_workers}")
    print(f"Max occurrences of single account: {max_count:,}")
    print()

def save_detailed_results(worker_task_counts, account_info, output_prefix):
    """
    Save detailed results to files.
    """
    # Save worker statistics
    worker_file = f"{output_prefix}_workers.txt"
    with open(worker_file, 'w') as f:
        f.write("Worker ID\tTask Count\n")
        for worker_id, count in sorted(worker_task_counts.items()):
            f.write(f"{worker_id}\t{count}\n")
    print(f"Worker statistics saved to: {worker_file}")
    
    # Save account statistics
    account_file = f"{output_prefix}_accounts.txt"
    with open(account_file, 'w') as f:
        f.write("Account Address\tOccurrence Count\tWorker Count\tWorker IDs\n")
        sorted_accounts = sorted(account_info.items(), key=lambda x: x[1]['count'], reverse=True)
        for account, info in sorted_accounts:
            workers_str = ','.join(map(str, sorted(info['workers'])))
            f.write(f"{account}\t{info['count']}\t{len(info['workers'])}\t{workers_str}\n")
    print(f"Account statistics saved to: {account_file}")
    print()

def main():
    log_file = "account_seen_count.log"
    
    # Analyze the log
    worker_task_counts, account_info = analyze_log(log_file)
    
    # Print statistics to console
    print_worker_statistics(worker_task_counts)
    print_account_statistics(account_info, top_n=50)
    
    # Save detailed results to files
    save_detailed_results(worker_task_counts, account_info, "log_analysis")
    
    print("Analysis complete!")

if __name__ == "__main__":
    main()


