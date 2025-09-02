import sys
import os
import json
import argparse
from typing import Dict, List, Tuple

import matplotlib.pyplot as plt


def load_data(path):
    with open(path, "r") as f:
        raw = json.load(f)

    data = {}
    for k, v in raw.items():
        ki = str(k)
        data[ki] = int(v)
    return data


def main():
    file_name = "./logs/balance_changes_2m.json"
    data = load_data(file_name)
    items = sorted(data.items(), key=lambda kv: kv[0])
    xs = [k for k, _ in items]
    counts = [v for _, v in items]
    total = sum(counts)
    print(total)


if __name__ == "__main__":
    main()
