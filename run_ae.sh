#!/usr/bin/env bash
# Smaran AE: drive the artifact from your LAPTOP.
# Thin remote wrapper around ./run.sh on node0; no prior login to node0 is
# needed, provided your SSH key was registered on your CloudLab account
# before the experiment was instantiated (README Quick start, Step 0).
#
#   bash run_ae.sh <user> <node0-host> start <mode> <scope...>   start (detached)
#   bash run_ae.sh <user> <node0-host> status                    check progress
#   bash run_ae.sh <user> <node0-host> follow                    watch log live
#   bash run_ae.sh <user> <node0-host> stop                      abort the run
#   bash run_ae.sh <user> <node0-host> fetch [local-dir]         copy figures back
#
#   mode:  smoke | quick | full        scope: all | kt | dl | fig4a ... fig7c
#
# Example session:
#   bash run_ae.sh alice clnode123.clemson.cloudlab.us start quick all
#   bash run_ae.sh alice clnode123.clemson.cloudlab.us status
#   bash run_ae.sh alice clnode123.clemson.cloudlab.us fetch ~/Desktop/smaran-figs
set -euo pipefail

if [ $# -lt 3 ]; then
  sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi

USER=$1; HOST=$2; shift 2
BRANCH=main
SSH=(ssh -o StrictHostKeyChecking=accept-new "${USER}@${HOST}")

# The profile clones the repo to /local/repository at boot; on a bare machine
# fall back to a home-dir clone.
REPO_EXPR='$( [ -d /local/repository/.git ] && echo /local/repository || echo ~/Smaran )'

ensure_repo() {
  "${SSH[@]}" "BRANCH='$BRANCH' bash -s" <<'REMOTE'
set -euo pipefail
if [ ! -d /local/repository/.git ] && [ ! -d ~/Smaran/.git ]; then
  git clone --branch "$BRANCH" https://github.com/distopialabs/Smaran.git ~/Smaran
fi
REMOTE
}

case "${1:-}" in
  start)
    ensure_repo
    "${SSH[@]}" "cd $REPO_EXPR && ./run.sh $*"
    echo
    echo "Check progress any time with:"
    echo "  bash $0 $USER $HOST status"
    ;;
  status|stop)
    "${SSH[@]}" "cd $REPO_EXPR && ./run.sh $1"
    ;;
  follow)
    echo '(Ctrl+C stops watching; the run keeps going on node0)'
    "${SSH[@]}" "cd $REPO_EXPR && ./run.sh follow"
    ;;
  fetch)
    DEST="${2:-$HOME/Desktop/smaran-ae-output}"
    mkdir -p "$DEST"
    scp "${USER}@${HOST}:$REPO_EXPR/output/*.pdf" "$DEST/" 2>/dev/null || true
    # DL figures live in nested per-figure directories (e.g. fig6/numclients32/)
    scp -r "${USER}@${HOST}:$REPO_EXPR/results/fig*" "$DEST/" 2>/dev/null || true
    scp -r "${USER}@${HOST}:$REPO_EXPR/results/paper-figures" "$DEST/" 2>/dev/null || true
    echo "Figures in $DEST:"
    find "$DEST" -name '*.pdf' | sort
    ;;
  *)
    sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac
