#!/usr/bin/env python3
import os, sys
from pathlib import Path

root = Path(sys.argv[1] if len(sys.argv) > 1 else ".")
count = 0
total = 0
min_size = None
max_size = 0

for base, _, files in os.walk(root, followlinks=False):
    for name in files:
        if name == "fetch.log" or name.endswith("19108895.json") or name.endswith("19108895.proof"):
            print(f"Skipping {name}")
            continue
        p = Path(base) / name
        try:
            sz = p.stat().st_size
        except Exception:
            continue
        count += 1
        total += sz
        if min_size is None or sz < min_size: min_size = sz
        if sz > max_size: max_size = sz

if count == 0:
    print("No files found")
else:
    print(f"files={count}")
    print(f"total_bytes={total}")
    print(f"max_bytes={max_size}")
    print(f"min_bytes={min_size}")
    print(f"avg_bytes={total / count:.2f}")
