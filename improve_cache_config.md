# Cache Performance Improvement Recommendations

## 1. Increase Cache Size

Current cache size: 32,768 entries
Total unique accounts: 108,949
Current coverage: ~30%

### Recommended sizes based on different strategies:

```go
// Option A: Cover 50% of working set (~54,474 entries)
// Good balance between memory usage and hit rate
MaxCost: 65_536,  // Next power of 2

// Option B: Cover 75% of working set (~81,711 entries)  
// Better hit rate, more memory usage
MaxCost: 131_072,  // ~128K entries

// Option C: Cover entire working set
// Best hit rate, highest memory usage
MaxCost: 262_144,  // ~256K entries
```

## 2. Optimize Ristretto Configuration

```go
func NewCache(db DB, precomputedData *config.PrecomputedData) (*Cache, error) {
    // Choose your cache size
    maxCost := int64(131_072) // Example: 75% coverage
    
    rc, err := ristretto.NewCache(&ristretto.Config[[]byte, *AccountInfo]{
        NumCounters: maxCost * 10,  // Ristretto recommends 10x MaxCost
        MaxCost:     maxCost,
        BufferItems: 64,
        Metrics:     true,  // Enable metrics to monitor performance
    })
    if err != nil {
        return nil, err
    }
    return &Cache{
        C:               rc,
        db:              db,
        precomputedData: precomputedData,
    }, nil
}
```

## 3. Consider Alternative Cache Implementations

Your code already has **Otter** and **LRU** cache implementations. Consider testing them:

- **Otter Cache**: Uses S3-FIFO eviction policy, often better for real-world workloads
- **LRU Cache**: Simpler, predictable behavior

## 4. Pre-warm Cache

For frequently accessed accounts, consider pre-loading them into cache:

```go
// Example: Pre-load top N most frequently accessed accounts
func PrewarmCache(cache *Cache, db DB, topAccounts []common.Address) {
    for _, account := range topAccounts {
        ai := loadAccountFromDB(account)
        if ai != nil {
            cache.C.Set(account[:], ai, 1)
        }
    }
}
```

## 5. Analyze Access Patterns

Create a script to analyze account access patterns:

```python
# analyze_access_patterns.py
import re
from collections import Counter

def analyze_patterns(filename):
    account_access = Counter()
    
    pattern = r'Account (0x[a-fA-F0-9]+) seen (\d+) times'
    
    with open(filename, 'r') as f:
        for line in f:
            match = re.search(pattern, line)
            if match:
                account = match.group(1)
                seen_count = int(match.group(2))
                account_access[account] = seen_count
    
    # Find hot accounts
    hot_accounts = account_access.most_common(1000)
    total_accesses = sum(account_access.values())
    hot_accesses = sum(count for _, count in hot_accounts)
    
    print(f"Top 1000 accounts handle {hot_accesses/total_accesses*100:.2f}% of accesses")
    
    # Access frequency distribution
    freq_dist = Counter(account_access.values())
    for seen_count, num_accounts in sorted(freq_dist.items()):
        print(f"Accounts seen {seen_count} times: {num_accounts}")
```

## 6. Memory Estimation

Estimate memory usage for different cache sizes:

```go
// Assuming each AccountInfo is ~1KB (rough estimate)
// 32K entries = ~32MB
// 64K entries = ~64MB  
// 128K entries = ~128MB
// 256K entries = ~256MB
```

## 7. Quick Fix - Edit cache.go

Change line 68 in cache.go:
```go
MaxCost:     131_072,  // Increased from 32_768
```

And line 67:
```go
NumCounters: 1_310_720,  // 10x MaxCost as recommended
```
