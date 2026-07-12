#!/usr/bin/env bash
# Shared helpers for the Smaran artifact-evaluation scripts.
# Sourced by the install scripts, plot_paper_figures.sh, and the per-figure
# experiment scripts (both full-scale and QuickTesting variants).

set -euo pipefail

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$LIB_DIR/../.." && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$REPO_ROOT/results}"

# Required Go toolchain (go.mod says go 1.25).
GO_REQUIRED_MINOR=25
GO_TARBALL_VERSION=1.25.6

# Where the SmaranEthereumDataset CloudLab dataset may be mounted. The Phase-4
# CloudLab profile mounts it at the first entry; the rest are fallbacks for
# manual setups. Override with SMARAN_DATASET_DIR / SMARAN_PAPER_LOGS.
DATASET_MOUNT_CANDIDATES=(
    "/mydata"
    "/data/dataset"
    "/proj/distopialabs-PG0/asim/dataset"
)

say() { echo "[$(date +%H:%M:%S)] $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Installation steps
# ---------------------------------------------------------------------------

ensure_data_local() {
    if [ ! -d /data/local ] || [ ! -w /data/local ]; then
        say "Making /data/local user-writable (needs sudo)"
        sudo mkdir -p /data/local
        sudo chown "$USER" /data/local
    fi
}

ensure_go() {
    if command -v go >/dev/null 2>&1; then
        local minor
        minor="$(go version | sed -E 's/.*go1\.([0-9]+).*/\1/')"
        if [ "$minor" -ge "$GO_REQUIRED_MINOR" ]; then
            say "Go toolchain OK: $(go version)"
            return
        fi
    fi
    say "Installing Go ${GO_TARBALL_VERSION} to /usr/local/go (needs sudo + network)"
    local tarball="go${GO_TARBALL_VERSION}.linux-amd64.tar.gz"
    curl -fsSL -o "/tmp/${tarball}" "https://go.dev/dl/${tarball}"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
    export PATH="/usr/local/go/bin:$PATH"
    say "Installed: $(go version)"
    say "NOTE: add /usr/local/go/bin to your PATH for future shells."
}

ensure_plot_deps() {
    # LaTeX + python plotting stack (plot scripts use matplotlib usetex).
    if command -v latex >/dev/null 2>&1 && command -v dvipng >/dev/null 2>&1 \
        && command -v gs >/dev/null 2>&1 \
        && python3 -c 'import matplotlib, pandas, numpy' >/dev/null 2>&1; then
        say "Plot dependencies already installed"
        return
    fi
    say "Installing plot dependencies (apt + pip, needs sudo + network; a few minutes)"
    sudo DEBIAN_FRONTEND=noninteractive apt-get update
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        python3-pip texlive-latex-extra texlive-fonts-recommended cm-super dvipng ghostscript
    pip3 install --user matplotlib pandas numpy
}

build_binaries() {
    say "Building all protocol binaries (make build; first build downloads Go modules)"
    (cd "$REPO_ROOT" && make build)
}

ensure_block_dataset() {
    # The ~23 GB Ethereum block dataset must be reachable at data/blocks.
    local link="$REPO_ROOT/data/blocks"
    if compgen -G "$link/blk_*.dat" >/dev/null 2>&1; then
        say "Block dataset present at data/blocks"
        return
    fi
    local candidates=()
    [ -n "${SMARAN_DATASET_DIR:-}" ] && candidates+=("$SMARAN_DATASET_DIR")
    for m in "${DATASET_MOUNT_CANDIDATES[@]}"; do
        candidates+=("$m/modified_accounts" "$m")
    done
    for dir in "${candidates[@]}"; do
        if compgen -G "$dir/blk_*.dat" >/dev/null 2>&1; then
            say "Linking data/blocks -> $dir"
            rm -f "$link"
            ln -s "$dir" "$link"
            return
        fi
    done
    say "WARNING: block dataset not found. Experiment scripts need it; plot-only mode does not."
    say "  Mount the SmaranEthereumDataset CloudLab dataset, or download it from Zenodo"
    say "  (see README), then set SMARAN_DATASET_DIR=<dir containing blk_*.dat> and re-run."
}

# The account-stats CSVs drive account selection in every benchmark. They are
# too large for git (account_stats_all.csv is 2.7 GB) and ship in the
# SmaranEthereumDataset CloudLab dataset next to the block data.
ensure_account_stats() {
    local f dir found
    for f in account_stats_50k.csv account_stats_all.csv; do
        [ -s "$REPO_ROOT/$f" ] && continue
        found=""
        for dir in "${SMARAN_DATASET_DIR:-}" "${DATASET_MOUNT_CANDIDATES[@]}"; do
            [ -n "$dir" ] && [ -f "$dir/$f" ] || continue
            say "Linking $f -> $dir/$f"
            ln -sf "$dir/$f" "$REPO_ROOT/$f"
            found=1
            break
        done
        [ -n "$found" ] || say "WARNING: $f not found — benchmarks need it. It ships in the SmaranEthereumDataset CloudLab dataset (see README)."
    done
}

# install_protocol <Merkle|Verkle|Smaran>
# The three protocols share one Go module, so the steps are identical; each
# install script exists so reviewers can follow the per-protocol instructions.
install_protocol() {
    echo "Installing $1"
    ensure_data_local
    ensure_go
    ensure_plot_deps
    build_binaries
    ensure_block_dataset
    ensure_account_stats
    say "$1 installed. Binaries in $REPO_ROOT/bin/"
}

# ---------------------------------------------------------------------------
# Setup checks — one source of truth for "is this machine ready"
# ---------------------------------------------------------------------------
# Every experiment/plot script starts with a quiet require_setup call listing
# only the items it actually needs (<1 s, silent on success); check_setup.sh
# runs every item verbosely. Both use the same _setup_check function so the
# quiet guard and the troubleshooting tool can never drift apart.

SETUP_CHECK_ITEMS=(binaries go plot-deps data-local blocks
    account-stats-50k account-stats params paper-logs server)

_INSTALL_HINT="run ./DecentralizedLedgerScripts/install_smaran.sh (on a fresh CloudLab node, setup may still be running — the SSH login banner shows its status)"

_setup_desc() {
    case "$1" in
        binaries)          echo "protocol binaries built (bin/)" ;;
        go)                echo "Go toolchain (go >= 1.$GO_REQUIRED_MINOR)" ;;
        plot-deps)         echo "plot dependencies (LaTeX + matplotlib/pandas/numpy)" ;;
        data-local)        echo "/data/local exists and is writable" ;;
        blocks)            echo "block dataset reachable at data/blocks" ;;
        account-stats-50k) echo "account_stats_50k.csv present" ;;
        account-stats)     echo "benchmark accounts list available" ;;
        params)            echo "Smaran verification params (data/params)" ;;
        paper-logs)        echo "paper-logs bundle reachable" ;;
        server)            echo "server node reachable (two-node mode only)" ;;
    esac
}

# _setup_check <item>: silent; returns 0 if satisfied, else sets
# SETUP_FAIL (what is wrong) and SETUP_HINT (one-line remedy).
_setup_check() {
    local item="$1"
    SETUP_FAIL=""
    SETUP_HINT="$_INSTALL_HINT"
    case "$item" in
        binaries)
            local b
            for b in samurai proofc merkle merkle-proofc verkle verkle-proofc; do
                if [ ! -x "$REPO_ROOT/bin/$b" ]; then
                    SETUP_FAIL="protocol binaries not built (bin/$b missing)"
                    return 1
                fi
            done ;;
        go)
            if ! command -v go >/dev/null 2>&1 \
                || [ "$(go version | sed -E 's/.*go1\.([0-9]+).*/\1/')" -lt "$GO_REQUIRED_MINOR" ]; then
                SETUP_FAIL="Go >= 1.$GO_REQUIRED_MINOR not on PATH"
                return 1
            fi ;;
        plot-deps)
            if ! command -v latex >/dev/null 2>&1 || ! command -v dvipng >/dev/null 2>&1 \
                || ! command -v gs >/dev/null 2>&1 \
                || ! python3 -c 'import importlib.util as u, sys; sys.exit(0 if all(u.find_spec(m) for m in ("matplotlib", "pandas", "numpy")) else 1)' 2>/dev/null; then
                SETUP_FAIL="plot dependencies missing (LaTeX/dvipng/ghostscript or python matplotlib/pandas/numpy)"
                return 1
            fi ;;
        data-local)
            if [ ! -d /data/local ] || [ ! -w /data/local ]; then
                SETUP_FAIL="/data/local missing or not writable"
                return 1
            fi ;;
        blocks)
            if ! compgen -G "$REPO_ROOT/data/blocks/blk_*.dat" >/dev/null 2>&1; then
                SETUP_FAIL="block dataset not reachable at data/blocks"
                SETUP_HINT="mount the SmaranEthereumDataset CloudLab dataset (or set SMARAN_DATASET_DIR) and $_INSTALL_HINT"
                return 1
            fi ;;
        account-stats-50k)
            if [ ! -s "$REPO_ROOT/account_stats_50k.csv" ]; then
                SETUP_FAIL="account_stats_50k.csv missing"
                SETUP_HINT="it ships in the SmaranEthereumDataset CloudLab dataset (or regenerate: scripts/artifact/generate_account_stats.sh); then $_INSTALL_HINT"
                return 1
            fi ;;
        account-stats)
            # Full-scale runs read account_stats_all.csv; quick runs generate a
            # window-matched list on the fly, which needs the Go toolchain.
            if [ "${QUICK:-0}" = "1" ]; then
                _setup_check go || SETUP_FAIL="Go toolchain missing (quick runs generate their accounts list with 'go run')"
            else
                if [ ! -s "$REPO_ROOT/account_stats_all.csv" ]; then
                    SETUP_FAIL="account_stats_all.csv missing"
                    SETUP_HINT="it ships in the SmaranEthereumDataset CloudLab dataset (or regenerate: scripts/artifact/generate_account_stats.sh); then $_INSTALL_HINT"
                    return 1
                fi
            fi ;;
        params)
            if [ ! -d "$REPO_ROOT/data/params" ] || ! ls -A "$REPO_ROOT/data/params" >/dev/null 2>&1; then
                SETUP_FAIL="data/params missing (Smaran proof verification needs it; it is committed in the repo)"
                SETUP_HINT="restore it with: git -C $REPO_ROOT checkout -- data/params"
                return 1
            fi ;;
        paper-logs)
            if ! _paper_logs_findable; then
                SETUP_FAIL="paper-logs bundle not found (smaran-paper-logs.tar.gz)"
                SETUP_HINT="mount the SmaranEthereumDataset CloudLab dataset, or set SMARAN_PAPER_LOGS=<extracted paper-logs dir>"
                return 1
            fi ;;
        server)
            # Only meaningful in two-node mode (SERVER_HOST set by
            # /local/cluster.env on CloudLab); single-node mode always passes.
            if [ -n "${SERVER_HOST:-}" ] && [ "$SERVER_HOST" != "localhost" ]; then
                if ! ssh -o BatchMode=yes -o ConnectTimeout=5 "$SERVER_HOST" "test -d /data/local -a -w /data/local" 2>/dev/null; then
                    SETUP_FAIL="server node $SERVER_HOST not reachable over ssh (or its /data/local is not writable)"
                    SETUP_HINT="server node setup may still be running — retry in a minute; see /local/cluster.env for the configured host"
                    return 1
                fi
            fi ;;
        *) die "unknown setup-check item: $item" ;;
    esac
}

# require_setup <item>... — quiet per-run guard: stops before any work with a
# one-line remedy if this script's prerequisites are not in place.
require_setup() {
    local item
    for item in "$@"; do
        if ! _setup_check "$item"; then
            echo "Setup incomplete: $SETUP_FAIL" >&2
            echo "  Fix: $SETUP_HINT" >&2
            echo "  (Full diagnosis: ./DecentralizedLedgerScripts/check_setup.sh)" >&2
            exit 1
        fi
    done
}

# ---------------------------------------------------------------------------
# Paper-logs bundle (prebaked logs, hosted in the CloudLab dataset — not git)
# ---------------------------------------------------------------------------

# True iff resolve_paper_logs below would succeed — same lookup order, but
# never extracts anything (used by the setup checks).
_paper_logs_findable() {
    local candidates=() dir m
    [ -n "${SMARAN_PAPER_LOGS:-}" ] && candidates+=("$SMARAN_PAPER_LOGS")
    candidates+=("$REPO_ROOT/paper-logs" "/data/local/artifact-staging/paper-logs")
    for m in "${DATASET_MOUNT_CANDIDATES[@]}"; do
        candidates+=("$m/paper-logs")
    done
    for dir in "${candidates[@]}"; do
        [ -f "$dir/MANIFEST.txt" ] && return 0
    done
    for m in "${DATASET_MOUNT_CANDIDATES[@]}"; do
        [ -f "$m/smaran-paper-logs.tar.gz" ] && return 0
    done
    return 1
}

# Prints the path of the paper-logs directory, extracting the tarball from the
# dataset mount if needed. Fails with instructions if not found.
resolve_paper_logs() {
    local candidates=()
    [ -n "${SMARAN_PAPER_LOGS:-}" ] && candidates+=("$SMARAN_PAPER_LOGS")
    candidates+=("$REPO_ROOT/paper-logs" "/data/local/artifact-staging/paper-logs")
    for m in "${DATASET_MOUNT_CANDIDATES[@]}"; do
        candidates+=("$m/paper-logs")
    done
    for dir in "${candidates[@]}"; do
        if [ -f "$dir/MANIFEST.txt" ]; then
            echo "$dir"
            return
        fi
    done
    # Tarball in the dataset mount → extract once into results/.
    for m in "${DATASET_MOUNT_CANDIDATES[@]}"; do
        if [ -f "$m/smaran-paper-logs.tar.gz" ]; then
            say "Extracting paper logs from $m/smaran-paper-logs.tar.gz" >&2
            mkdir -p "$RESULTS_DIR"
            tar -xzf "$m/smaran-paper-logs.tar.gz" -C "$RESULTS_DIR"
            echo "$RESULTS_DIR/paper-logs"
            return
        fi
    done
    die "paper logs not found. Mount the SmaranEthereumDataset CloudLab dataset (contains
smaran-paper-logs.tar.gz), or set SMARAN_PAPER_LOGS=<extracted paper-logs dir>."
}

# stage_paper_logs <bundle-dir> <work-dir>: copy the bundle into a writable
# work dir and decompress the .gz CSVs (plot scripts read plain .csv).
stage_paper_logs() {
    local src="$1" work="$2"
    rm -rf "$work"
    mkdir -p "$work"
    cp -r "$src"/. "$work"/
    find "$work" -name '*.gz' -exec gunzip -f {} +
}

# ---------------------------------------------------------------------------
# Server lifecycle (used by the query-benchmark experiment scripts)
# ---------------------------------------------------------------------------

SERVER_PID=""

# start_server <binary> <db-dir> <port> [extra args...]
start_server() {
    local bin="$1" db="$2" port="$3"; shift 3
    mkdir -p "$RESULTS_DIR/server-logs"
    local log="$RESULTS_DIR/server-logs/$(basename "$bin")_port${port}.log"
    say "Starting $(basename "$bin") server on port $port (log: $log)"
    "$bin" serve --db-dir "$db" --port "$port" "$@" >"$log" 2>&1 &
    SERVER_PID=$!
    trap 'stop_server' EXIT
}

# wait_for_server <port> <timeout-seconds> [message]
wait_for_server() {
    local port="$1" timeout="$2" msg="${3:-server}"
    local waited=0
    say "Waiting for $msg to become ready on port $port (up to $((timeout / 60)) min)..."
    while ! nc -z localhost "$port" 2>/dev/null; do
        if [ -n "$SERVER_PID" ] && ! kill -0 "$SERVER_PID" 2>/dev/null; then
            die "$msg exited before becoming ready; see $RESULTS_DIR/server-logs/"
        fi
        sleep 5
        waited=$((waited + 5))
        if [ $((waited % 60)) -eq 0 ]; then
            say "  still waiting (${waited}s elapsed) — Smaran opens ~1000 shard DBs at startup; this is normal"
        fi
        [ "$waited" -ge "$timeout" ] && die "$msg not ready after ${timeout}s"
    done
    say "$msg is ready (took ${waited}s)"
}

stop_server() {
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        say "Stopping server (pid $SERVER_PID; Smaran closes ~1000 shard DBs, can take minutes)"
        kill "$SERVER_PID"
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    SERVER_PID=""
}

# server_state_root <server-log-file>: extract the state root logged at startup
# (needed by the Smaran proof client for verification).
server_state_root() {
    grep -oE 'state root: (0x[0-9a-fA-F]+)' "$1" | tail -1 | awk '{print $3}'
}
