#!/usr/bin/env bash
# Rename update_metrics_<n>_<YYYYMMDD>_<HHMMSS>.csv -> update_metrics_<n>.csv
# Usage: ./rename_update_metrics_csvs.sh [DIR]
#   DIR  directory containing the CSVs (default: current working directory)
set -euo pipefail

dir="${1:-.}"
if [[ ! -d "$dir" ]]; then
  echo "usage: $0 [DIR]" >&2
  echo "  DIR must be a directory containing update_metrics_*_*.csv files" >&2
  exit 1
fi

cd "$dir"

shopt -s nullglob
for f in update_metrics_*_*.csv; do
  [[ -f "$f" ]] || continue
  if [[ "$f" =~ ^(update_metrics_[0-9]+)_[0-9]{8}_[0-9]{6}\.csv$ ]]; then
    new="${BASH_REMATCH[1]}.csv"
    if [[ "$f" == "$new" ]]; then
      continue
    fi
    if [[ -e "$new" ]]; then
      echo "skip: $new already exists (would replace $f)" >&2
      continue
    fi
    mv -- "$f" "$new"
    echo "renamed: $f -> $new"
  fi
done
