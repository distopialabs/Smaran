#!/usr/bin/env python3
"""
Analyze account access patterns to understand cache behavior
and identify optimization opportunities.
"""

import re
import sys
from collections import Counter

try:
    import matplotlib.pyplot as plt
    HAS_MATPLOTLIB = True
except ImportError:
    HAS_MATPLOTLIB = False

def analyze_patterns(filename):
    """Analyze account access patterns from log file."""
    account_access = Counter()
    
    # Pattern to match: Account 0x... seen N times, fetched from db M times
    pattern = r'Account (0x[a-fA-F0-9]+) seen (\d+) times'
    
    print(f"Analyzing access patterns in {filename}...")
    
    with open(filename, 'r') as f:
        for line in f:
            match = re.search(pattern, line)
            if match:
                account = match.group(1)
                seen_count = int(match.group(2))
                account_access[account] = seen_count
    
    total_accounts = len(account_access)
    total_accesses = sum(account_access.values())
    
    print(f"\nTotal unique accounts: {total_accounts:,}")
    print(f"Total accesses: {total_accesses:,}")
    print(f"Average accesses per account: {total_accesses/total_accounts:.2f}")
    
    # Analyze hot accounts
    print("\n" + "="*60)
    print("HOT ACCOUNT ANALYSIS")
    print("="*60)
    
    for top_n in [10, 100, 1000, 10000]:
        if top_n > total_accounts:
            break
        hot_accounts = account_access.most_common(top_n)
        hot_accesses = sum(count for _, count in hot_accounts)
        percentage = hot_accesses / total_accesses * 100
        print(f"Top {top_n:5} accounts ({top_n/total_accounts*100:5.2f}% of accounts) "
              f"handle {hot_accesses:7,} accesses ({percentage:5.2f}% of total)")
    
    # Access frequency distribution
    print("\n" + "="*60)
    print("ACCESS FREQUENCY DISTRIBUTION")
    print("="*60)
    freq_dist = Counter(account_access.values())
    
    print(f"{'Times Seen':>10} | {'# Accounts':>10} | {'% of Accounts':>13}")
    print("-"*40)
    
    for seen_count in sorted(freq_dist.keys()):
        num_accounts = freq_dist[seen_count]
        percentage = num_accounts / total_accounts * 100
        print(f"{seen_count:10} | {num_accounts:10,} | {percentage:12.2f}%")
    
    # Calculate cache size recommendations
    print("\n" + "="*60)
    print("CACHE SIZE RECOMMENDATIONS")
    print("="*60)
    
    # Sort accounts by access count
    sorted_accounts = sorted(account_access.values(), reverse=True)
    cumulative_accesses = 0
    
    for coverage in [50, 80, 90, 95, 99]:
        target_accesses = total_accesses * coverage / 100
        cache_size = 0
        cumulative_accesses = 0
        
        for access_count in sorted_accounts:
            cumulative_accesses += access_count
            cache_size += 1
            if cumulative_accesses >= target_accesses:
                break
        
        print(f"To capture {coverage}% of accesses: cache size = {cache_size:,} accounts "
              f"({cache_size/total_accounts*100:.1f}% of unique accounts)")
    
    # Create visualization
    if HAS_MATPLOTLIB:
        try:
            # Plot access frequency distribution
            plt.figure(figsize=(12, 8))
        
            # Subplot 1: Access count distribution
            plt.subplot(2, 2, 1)
            access_counts = list(account_access.values())
            plt.hist(access_counts, bins=50, edgecolor='black')
            plt.xlabel('Number of Accesses')
            plt.ylabel('Number of Accounts')
            plt.title('Distribution of Access Counts')
            plt.yscale('log')
            
            # Subplot 2: Cumulative access percentage
            plt.subplot(2, 2, 2)
            sorted_counts = sorted(access_counts, reverse=True)
            cumulative_pct = [sum(sorted_counts[:i+1])/total_accesses*100 
                             for i in range(len(sorted_counts))]
            account_pct = [(i+1)/total_accounts*100 for i in range(len(sorted_counts))]
            plt.plot(account_pct[:10000], cumulative_pct[:10000])  # Plot first 10k for clarity
            plt.xlabel('Percentage of Accounts (%)')
            plt.ylabel('Cumulative Access Percentage (%)')
            plt.title('Cumulative Access Distribution')
            plt.grid(True)
            
            # Subplot 3: Top accounts bar chart
            plt.subplot(2, 2, 3)
            top_20 = account_access.most_common(20)
            accounts = [f"...{acc[-6:]}" for acc, _ in top_20]
            counts = [count for _, count in top_20]
            plt.bar(range(len(accounts)), counts)
            plt.xticks(range(len(accounts)), accounts, rotation=90)
            plt.xlabel('Account (last 6 chars)')
            plt.ylabel('Access Count')
            plt.title('Top 20 Most Accessed Accounts')
            plt.tight_layout()
            
            # Subplot 4: Cache efficiency curve
            plt.subplot(2, 2, 4)
            cache_sizes = []
            hit_rates = []
            
            for cache_size in range(1000, min(total_accounts, 100000), 1000):
                top_accounts_sum = sum(sorted_counts[:cache_size])
                hit_rate = top_accounts_sum / total_accesses * 100
                cache_sizes.append(cache_size)
                hit_rates.append(hit_rate)
            
            plt.plot(cache_sizes, hit_rates)
            plt.axhline(y=80, color='r', linestyle='--', label='80% target')
            plt.axhline(y=90, color='g', linestyle='--', label='90% target')
            plt.xlabel('Cache Size (number of accounts)')
            plt.ylabel('Expected Hit Rate (%)')
            plt.title('Cache Size vs Expected Hit Rate')
            plt.legend()
            plt.grid(True)
            
            plt.tight_layout()
            output_file = 'access_patterns_analysis.png'
            plt.savefig(output_file, dpi=150)
            print(f"\nVisualization saved to: {output_file}")
            
        except Exception as e:
            print(f"\nCouldn't create visualization: {e}")
            print("Install matplotlib with: pip install matplotlib")
    else:
        print("\nVisualization skipped (matplotlib not installed)")
        print("Install with: pip install matplotlib")
    
    return account_access

def main():
    if len(sys.argv) < 2:
        print("Usage: python analyze_access_patterns.py <log_file>")
        sys.exit(1)
    
    log_file = sys.argv[1]
    
    try:
        analyze_patterns(log_file)
    except FileNotFoundError:
        print(f"Error: File '{log_file}' not found")
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
