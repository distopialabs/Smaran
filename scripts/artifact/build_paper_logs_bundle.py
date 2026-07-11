#!/usr/bin/env python3
"""Author-side tool: assemble the curated paper-logs bundle for artifact evaluation.

Selects exactly the log files the four paper plot scripts consume from
asim-dev-notes/FinalPaperBenchLogs, stages them in the plot-ready layout, gzips
the large per-block ingestion CSVs, and produces a tarball to upload to the
SmaranEthereumDataset CloudLab dataset (Zenodo hosts only the Ethereum block
dataset, not these logs).

File contents are never modified (gzip -n round-trips byte-identically); only
the directory layout is adapted to what scripts/paper-figures/*.py expect:

    paper-logs/
      fig7a/{smaran,merkle,verkle,cauchy}/ingestion_<users>.csv.gz
      fig7c/shard<S>/smaran/ingestion_<users>_<ts>.csv.gz
      fig6/numclients32/{merkle,verkle,samuraimpt}/proof_range<R>.csv
      fig6/numclients32/cauchy/{Query.csv, proof_range<R>.txt}
      fig7b/optimus/proof_range<R>.csv
      fig7b/non_optimus/proof_range<R>_<ts>.csv
      MANIFEST.txt

(fig7a/fig7c protocol dirs are named at stage time by plot_paper_figures.sh;
the bundle keeps the benchmark's own protocol dir names — see notes below.)

Usage: python3 scripts/artifact/build_paper_logs_bundle.py [--output-dir DIR]
"""

import argparse
import glob
import gzip
import hashlib
import os
import re
import shutil
import subprocess
import sys

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
SRC = os.path.join(REPO, "asim-dev-notes", "FinalPaperBenchLogs")

SKIP_USERS = {0, 1}  # ingestion_0 (all users) and ingestion_1 are not in the paper


def newest_per_users(proto_dir: str) -> dict:
    """Newest ingestion CSV per user count (mirrors fig7c's find_newest_ingestion_csv)."""
    best = {}
    for path in glob.glob(os.path.join(proto_dir, "ingestion_*.csv")):
        m = re.match(r"ingestion_(\d+)(?:_|\.)", os.path.basename(path))
        if not m or int(m.group(1)) in SKIP_USERS:
            continue
        users = int(m.group(1))
        if users not in best or os.path.getmtime(path) > os.path.getmtime(best[users]):
            best[users] = path
    return best


def collect() -> list[tuple[str, str, bool]]:
    """Return (source_path, bundle_relpath, gzip?) triples for the curated set."""
    files: list[tuple[str, str, bool]] = []

    # fig7a: ingestion logs per protocol (samurai/merkle/verkle from the sweep, cauchy prebaked)
    for proto in ("samurai", "merkle", "verkle"):
        for users, path in sorted(newest_per_users(os.path.join(SRC, "ingestion_logs", proto)).items()):
            files.append((path, f"fig7a/{proto}/{os.path.basename(path)}", True))
    for users, path in sorted(newest_per_users(os.path.join(SRC, "cauchy_unzipped", "cauchy")).items()):
        files.append((path, f"fig7a/cauchy/{os.path.basename(path)}", True))

    # fig7c: newest ingestion CSV per (shard, users), samurai only
    for shard_dir in sorted(glob.glob(os.path.join(SRC, "microbench_shards_output", "shard*"))):
        shard = os.path.basename(shard_dir)
        for users, path in sorted(newest_per_users(os.path.join(shard_dir, "samurai")).items()):
            files.append((path, f"fig7c/{shard}/samurai/{os.path.basename(path)}", True))

    # fig6: numclients32 proof CSVs (already clean-named) + cauchy query logs
    for proto in ("merkle", "verkle", "samuraimpt"):
        for path in sorted(glob.glob(os.path.join(SRC, "proof_latency_logs", "numclients32", proto, "proof_range*.csv"))):
            files.append((path, f"fig6/numclients32/{proto}/{os.path.basename(path)}", False))
    cauchy = os.path.join(SRC, "cauchy_unzipped", "cauchy")
    for path in [os.path.join(cauchy, "Query.csv")] + sorted(glob.glob(os.path.join(cauchy, "proof_range*.txt"))):
        files.append((path, f"fig6/numclients32/cauchy/{os.path.basename(path)}", False))

    # fig7b: optimus = the fig6 samuraimpt CSVs; non_optimus = the flat timestamped dir
    for path in sorted(glob.glob(os.path.join(SRC, "proof_latency_logs", "numclients32", "samuraimpt", "proof_range*.csv"))):
        files.append((path, f"fig7b/optimus/{os.path.basename(path)}", False))
    for path in sorted(glob.glob(os.path.join(SRC, "samuraimpt_non_optimus_proof_bench_logs", "proof_range*.csv"))):
        files.append((path, f"fig7b/non_optimus/{os.path.basename(path)}", False))

    return files


def sha256(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--output-dir", default="/data/local/artifact-staging",
                        help="Where to write paper-logs/ and the tarball (default: %(default)s)")
    args = parser.parse_args()

    if not os.path.isdir(SRC):
        sys.exit(f"ERROR: source logs not found at {SRC}")

    bundle_root = os.path.join(args.output_dir, "paper-logs")
    if os.path.isdir(bundle_root):
        shutil.rmtree(bundle_root)

    files = collect()
    manifest_lines = []
    raw_bytes = staged_bytes = 0
    for src, rel, do_gzip in files:
        dst = os.path.join(bundle_root, rel + (".gz" if do_gzip else ""))
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        if do_gzip:
            with open(src, "rb") as fin, gzip.GzipFile(dst, "wb", compresslevel=6, mtime=0) as fout:
                shutil.copyfileobj(fin, fout)
        else:
            shutil.copyfile(src, dst)
        raw_bytes += os.path.getsize(src)
        staged_bytes += os.path.getsize(dst)
        manifest_lines.append(f"{sha256(src)}  {rel}  (source: {os.path.relpath(src, REPO)})")
        print(f"  staged {rel}{'.gz' if do_gzip else ''}")

    with open(os.path.join(bundle_root, "MANIFEST.txt"), "w") as f:
        f.write("Curated benchmark logs used for the figures in the Smaran paper.\n"
                "SHA-256 checksums are of the original (uncompressed) files; .gz files\n"
                "decompress byte-identically to the archived originals.\n\n")
        f.write("\n".join(manifest_lines) + "\n")

    tarball = os.path.join(args.output_dir, "smaran-paper-logs.tar.gz")
    subprocess.run(["tar", "-czf", tarball, "-C", args.output_dir, "paper-logs"], check=True)

    print(f"\n{len(files)} files: {raw_bytes / 2**20:.1f} MB raw -> {staged_bytes / 2**20:.1f} MB staged")
    print(f"Bundle dir: {bundle_root}")
    print(f"Tarball:    {tarball} ({os.path.getsize(tarball) / 2**20:.1f} MB)")
    print("Upload the tarball to the SmaranEthereumDataset CloudLab dataset (logs are NOT hosted on Zenodo).")


if __name__ == "__main__":
    main()
