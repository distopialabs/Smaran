#!/usr/bin/env python3
import sys, re

# Read from a file path if given, else from stdin
data = (
    sys.stdin.read()
    if len(sys.argv) == 1
    else open(sys.argv[1], "r", encoding="utf-8").read()
)

# Regex to capture the three timings (value + unit)
pat = re.compile(
    r"Time taken to (?P<kind>get balances for block|update account trees for block|process block) \d+:\s*(?P<val>[\d.]+)(?P<unit>ms|s|µs|us|ns)"
)


def to_ms(v: float, unit: str) -> float:
    if unit == "ms":
        return v
    if unit == "s":
        return v * 1000.0
    if unit in ("µs", "us"):
        return v / 1000.0
    if unit == "ns":
        return v / 1_000_000.0
    raise ValueError(f"unknown unit: {unit}")


vals = {
    "get balances for block": [],
    "update account trees for block": [],
    "process block": [],
}

for m in pat.finditer(data):
    vals[m["kind"]].append(to_ms(float(m["val"]), m["unit"]))


def avg(xs):
    return sum(xs) / len(xs) if xs else 0.0


n_blocks = min(
    len(vals["get balances for block"]),
    len(vals["update account trees for block"]),
    len(vals["process block"]),
)

print(f"Blocks parsed (with all three timings present): {n_blocks}")
print(
    f"Average time to get balances:       {avg(vals['get balances for block']):.6f} ms"
)
print(
    f"Average time to update account tree: {avg(vals['update account trees for block']):.6f} ms"
)
print(f"Average time to process block:       {avg(vals['process block']):.6f} ms")
