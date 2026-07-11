#!/usr/bin/env python3
"""verify.py — sanity-check that a reviewer's AE run matches the paper's shape.

Reads:
  ~/Smaran/logs/<latest-sweep>/output/kt_query_summary.csv  (for Figures 4)
  ~/Smaran/logs/<latest-sweep>/output/kt_put_summary.csv    (for Figure 5)

Emits a pass/fail report on each qualitative claim from §7.1 of the paper.
Exits 0 if all pass, 1 if any fail. Absolute numbers depend on hardware,
so all checks are shape-based (ordering, monotonicity, ratios).

Usage:
  python3 verify.py                          # auto-detect latest sweep
  python3 verify.py --logs-dir ~/Smaran/logs # explicit
"""
import argparse
import csv
import sys
from pathlib import Path


def load_csv(path):
    if not path.exists():
        return None
    rows = []
    with path.open() as f:
        for row in csv.DictReader(f):
            rows.append(row)
    return rows


def check(label, ok, detail=""):
    icon = '✓' if ok else '✗'
    print(f'  {icon} {label}' + (f' — {detail}' if detail else ''))
    return ok


def find_latest_sweep(logs_dir):
    candidates = sorted([d for d in logs_dir.glob('2026-*') if d.is_dir()], reverse=True)
    return candidates[0] if candidates else None


def verify_fig4(csv_path):
    rows = load_csv(csv_path)
    if not rows:
        print(f'  (skipped — {csv_path} not found)')
        return True

    print(f'\nFig 4 (from {csv_path.parent.name})')
    by_proto = {}
    for r in rows:
        proto = r['protocol_label']
        by_proto.setdefault(proto, []).append({
            'v': int(r['num_versions']),
            'lat': float(r['avg_latency_ms']),
            'thr': float(r['throughput_qps']),
            'pay': float(r['avg_payload_kib']),
        })
    for proto in by_proto:
        by_proto[proto].sort(key=lambda p: p['v'])

    ok = True
    if 'Coniks' in by_proto and 'Smaran' in by_proto:
        smaran_max = by_proto['Smaran'][-1]
        coniks_max = by_proto['Coniks'][-1]
        ok &= check(
            'Coniks 2047-version latency > Smaran 2047-version latency',
            coniks_max['lat'] > smaran_max['lat'],
            f'{coniks_max["lat"]:.1f}ms vs {smaran_max["lat"]:.1f}ms',
        )
        ok &= check(
            'Smaran 2047-version throughput > Coniks 2047-version throughput',
            smaran_max['thr'] > coniks_max['thr'],
            f'{smaran_max["thr"]:.1f} vs {coniks_max["thr"]:.1f} qps',
        )
    if 'Optiks' in by_proto:
        optiks_first = by_proto['Optiks'][0]
        optiks_last  = by_proto['Optiks'][-1]
        ok &= check(
            'Optiks latency grows with versions',
            optiks_last['lat'] > optiks_first['lat'],
            f'{optiks_first["lat"]:.1f}ms → {optiks_last["lat"]:.1f}ms',
        )
    if 'Smaran' in by_proto and 'Optiks' in by_proto:
        smaran_end = by_proto['Smaran'][-1]
        optiks_end = by_proto['Optiks'][-1]
        ok &= check(
            'Smaran payload smaller than Optiks payload at max versions',
            smaran_end['pay'] < optiks_end['pay'],
            f'{smaran_end["pay"]:.1f} < {optiks_end["pay"]:.1f} KiB',
        )
    return ok


def verify_fig5(csv_path):
    rows = load_csv(csv_path)
    if not rows:
        print(f'  (skipped — {csv_path} not found)')
        return True

    print(f'\nFig 5 (from {csv_path.parent.name})')
    by_proto = {}
    for r in rows:
        proto = r['protocol_label']
        by_proto.setdefault(proto, []).append({
            'u': int(r['num_users']),
            'thr': float(r['throughput_qps']),
        })
    for proto in by_proto:
        by_proto[proto].sort(key=lambda p: p['u'])

    ok = True
    if 'Smaran' in by_proto and 'Coniks' in by_proto:
        smaran_10k = next(p for p in by_proto['Smaran'] if p['u'] == 10000)
        coniks_10k = next(p for p in by_proto['Coniks'] if p['u'] == 10000)
        ratio = smaran_10k['thr'] / coniks_10k['thr']
        ok &= check(
            'Smaran throughput ≫ Coniks at 10k users (paper: ~130×)',
            ratio > 50,
            f'{ratio:.1f}× higher',
        )
    if 'Optiks' in by_proto:
        optiks_thr = [p['thr'] for p in by_proto['Optiks']]
        ok &= check(
            'Optiks reaches at least 50k ops/s at some point',
            max(optiks_thr) > 50000,
            f'max = {max(optiks_thr):.0f} qps',
        )
    return ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--logs-dir', default=str(Path.home() / 'Smaran' / 'logs'))
    args = ap.parse_args()

    logs = Path(args.logs_dir)
    if not logs.exists():
        print(f'{logs} does not exist — run the experiments first', file=sys.stderr)
        return 2

    sweeps = sorted([d for d in logs.glob('2026-*') if d.is_dir()], reverse=True)
    if not sweeps:
        print(f'no sweep directories under {logs}', file=sys.stderr)
        return 2

    query_csv = next((s / 'output' / 'kt_query_summary.csv' for s in sweeps
                      if (s / 'output' / 'kt_query_summary.csv').exists()), None)
    put_csv   = next((s / 'output' / 'kt_put_summary.csv' for s in sweeps
                      if (s / 'output' / 'kt_put_summary.csv').exists()), None)

    if not query_csv and not put_csv:
        print(f'no CSVs found under {logs}', file=sys.stderr)
        return 2

    ok = True
    if query_csv: ok &= verify_fig4(query_csv)
    else: print('Fig 4: no kt_query_summary.csv yet (run Fig 4 experiments first)')
    if put_csv:   ok &= verify_fig5(put_csv)
    else: print('Fig 5: no kt_put_summary.csv yet (run Fig 5 experiments first)')

    print()
    if ok:
        print('All shape checks passed. Compare PDFs to reference_pdfs/ for visual confirmation.')
        return 0
    print('One or more shape checks failed. See detail above.')
    return 1


if __name__ == '__main__':
    sys.exit(main())
