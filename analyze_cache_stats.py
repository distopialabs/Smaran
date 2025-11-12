#!/usr/bin/env python3
"""
Analyze cache hit/miss ratio from account_seen_count.log

Logic:
- First encounter (seen = 1): initialize, no cache operation
- Subsequent encounters (seen > 1): cache lookup
  - Cache miss: fetch from db (increments db_fetch_count)
  - Cache hit: no db fetch
- Total cache lookups = seen - 1
- Cache misses = fetched from db
- Cache hits = (seen - 1) - (fetched from db)
"""

import re
import sys

def parse_log_file(filename):
    """Parse the log file and extract account statistics."""
    # Pattern to match: Account 0x... seen N times, fetched from db M times, total time ...
    pattern = r'Account 0x[a-fA-F0-9]+ seen (\d+) times, fetched from db (\d+) times'
    
    total_cache_lookups = 0
    total_cache_misses = 0
    total_cache_hits = 0
    account_count = 0
    
    print(f"Analyzing {filename}...")
    
    with open(filename, 'r') as f:
        for line_num, line in enumerate(f, 1):
            match = re.search(pattern, line)
            if match:
                seen_count = int(match.group(1))
                db_fetch_count = int(match.group(2))
                
                # Only accounts with seen > 1 have cache operations
                if seen_count > 1:
                    cache_lookups = seen_count - 1  # All encounters after first
                    cache_misses = db_fetch_count
                    cache_hits = cache_lookups - cache_misses
                    
                    total_cache_lookups += cache_lookups
                    total_cache_misses += cache_misses
                    total_cache_hits += cache_hits
                
                account_count += 1
    
    return {
        'account_count': account_count,
        'total_cache_lookups': total_cache_lookups,
        'total_cache_misses': total_cache_misses,
        'total_cache_hits': total_cache_hits
    }

def print_statistics(stats):
    """Print cache statistics with hit/miss ratios."""
    print("\n" + "=" * 60)
    print("CACHE STATISTICS")
    print("=" * 60)
    print(f"Total accounts logged: {stats['account_count']:,}")
    print(f"Total cache lookups: {stats['total_cache_lookups']:,}")
    print(f"Total cache hits: {stats['total_cache_hits']:,}")
    print(f"Total cache misses: {stats['total_cache_misses']:,}")
    print("-" * 60)
    
    if stats['total_cache_lookups'] > 0:
        hit_ratio = (stats['total_cache_hits'] / stats['total_cache_lookups']) * 100
        miss_ratio = (stats['total_cache_misses'] / stats['total_cache_lookups']) * 100
        
        print(f"Cache Hit Ratio: {hit_ratio:.2f}%")
        print(f"Cache Miss Ratio: {miss_ratio:.2f}%")
    else:
        print("No cache operations found (all accounts seen only once)")
    
    print("=" * 60)

def main():
    if len(sys.argv) < 2:
        print("Usage: python analyze_cache_stats.py <log_file>")
        print("Example: python analyze_cache_stats.py account_seen_count.log")
        sys.exit(1)
    
    log_file = sys.argv[1]
    
    try:
        stats = parse_log_file(log_file)
        print_statistics(stats)
    except FileNotFoundError:
        print(f"Error: File '{log_file}' not found")
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()

