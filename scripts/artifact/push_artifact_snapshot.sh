#!/usr/bin/env bash
# Author-side tool: publish the current branch's tree as a single-commit
# snapshot branch (no history). The CloudLab profile pins that branch —
# a full clone of this repo is >500 MiB of dead history (old committed
# binaries), over CloudLab's repo limit, while the tree itself is a few MB.
#
# Usage: scripts/artifact/push_artifact_snapshot.sh [remote] [branch]
#          remote  git remote or URL to push to   (default: origin)
#          branch  snapshot branch name           (default: artifact)
#
# Re-run after every change to the source branch; the snapshot is
# force-pushed (it has no history to preserve). At submission time, tag the
# final snapshot commit and pin the profile to the tag.
set -euo pipefail

REMOTE="${1:-origin}"
BRANCH="${2:-artifact}"

cd "$(dirname "${BASH_SOURCE[0]}")/../.."

if ! git diff-index --quiet HEAD --; then
    echo "ERROR: uncommitted changes — commit or stash first (the snapshot is taken from HEAD)." >&2
    exit 1
fi

SRC="$(git rev-parse --abbrev-ref HEAD)"
SNAP="$(git commit-tree "HEAD^{tree}" -m "Smaran artifact snapshot of $SRC @ $(git rev-parse --short HEAD)")"
echo "Snapshot commit $SNAP (tree of $SRC @ $(git rev-parse --short HEAD))"
git push -f "$REMOTE" "$SNAP:refs/heads/$BRANCH"
echo "Pushed to $REMOTE $BRANCH — the CloudLab profile should pin this branch."
