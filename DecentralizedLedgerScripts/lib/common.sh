#!/usr/bin/env bash
# Shared helpers for the Smaran artifact-evaluation scripts.
# Sourced by the install scripts, plot_paper_figures.sh, and the per-figure
# experiment scripts (both full-scale and QuickTesting variants).

set -euo pipefail

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$LIB_DIR/../.." && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$REPO_ROOT/results}"

# Two-node (CloudLab profile) mode: the profile's startup script writes
# /local/cluster.env on both nodes with SERVER_HOST=<server node> and
# SMARAN_DATASET_DIR=/smaran-dataset. Without it, everything runs on this
# machine (Path B — own server), exactly as before. SMARAN_NO_CLUSTER_ENV=1
# ignores the file (debugging escape hatch).
if [ -f /local/cluster.env ] && [ -z "${SMARAN_NO_CLUSTER_ENV:-}" ]; then
    # shellcheck source=/dev/null
    . /local/cluster.env
fi

is_remote() {
    [ -n "${SERVER_HOST:-}" ] && [ "$SERVER_HOST" != "localhost" ] \
        && [ "$SERVER_HOST" != "127.0.0.1" ]
}

# Host the protocol servers run on, as seen from this node.
server_host() { if is_remote; then echo "$SERVER_HOST"; else echo localhost; fi; }

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10)

# server_ctl <cmd...>: short control command on the server node (remote mode).
server_ctl() { ssh "${SSH_OPTS[@]}" "$SERVER_HOST" "$@"; }

# server_run <cmd...>: long-running foreground command on the server node.
# -tt ties the remote process's lifetime to this script: a client-side Ctrl+C
# (or dying SSH session) takes the remote process down with it, and its
# stdout streams live to this console.
server_run() { ssh -tt "${SSH_OPTS[@]}" "$SERVER_HOST" "$@"; }

# Server-local scratch for anything a remote server-side process writes
# (logs, ingestion output). Never a repo path: under the CloudLab profile
# each node has its own repo clone, so the client fetches these with rsync
# at end of run instead of expecting shared storage.
SERVER_RUN_DIR=/data/local/artifact-run

# Required Go toolchain (go.mod says go 1.25).
GO_REQUIRED_MINOR=25
GO_TARBALL_VERSION=1.25.6

# Where the SmaranEthereumDataset CloudLab dataset may be mounted. The Phase-4
# CloudLab profile mounts it at the first entry; the rest are fallbacks for
# manual setups. Override with SMARAN_DATASET_DIR / SMARAN_PAPER_LOGS.
DATASET_MOUNT_CANDIDATES=(
    "/smaran-dataset"
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
# --detach: background experiment runs that survive SSH disconnects
# ---------------------------------------------------------------------------
# Shared by every run_fig*.sh via lib/experiments.sh (the QuickTesting
# wrappers inherit it). Inline runs (the default) write no state files; only
# --detach records <figure>.state + <figure>.console.log under results/logs/,
# which status.sh reads.

DETACH=0
RUN_ARGS=()

# parse_run_flags "$@": split artifact-level flags from pass-through args.
parse_run_flags() {
    local a
    for a in "$@"; do
        case "$a" in
            --detach) DETACH=1 ;;
            *) RUN_ARGS+=("$a") ;;
        esac
    done
}

run_state_file()  { echo "$RESULTS_DIR/logs/$1.state"; }
run_console_log() { echo "$RESULTS_DIR/logs/$1.console.log"; }

# figure_estimate <fig-id>: wall-clock estimate for the current tier, from
# the EST_* table in config.sh (informational only).
figure_estimate() {
    local v="FULL_EST_${1^^}"
    [ "${QUICK:-0}" = "1" ] && v="QUICK_EST_${1^^}"
    echo "${!v:-unknown}"
}

# maybe_detach: in the parent invocation of a --detach run, relaunch this
# script nohup'd in the background, record its state file, and exit. The
# relaunched child (SMARAN_DETACHED=1) takes the normal inline path with its
# output captured to the console log.
maybe_detach() {
    [ "$DETACH" = "1" ] || return 0
    [ -z "${SMARAN_DETACHED:-}" ] || return 0
    local fig="$FIGURE_ID"
    local console state est pid
    console="$(run_console_log "$fig")"
    state="$(run_state_file "$fig")"
    est="$(figure_estimate "$fig")"

    # Refuse to double-launch: same figure = same ports and cache dirs.
    if [ -f "$state" ]; then
        pid="$(sed -n 's/^PID=//p' "$state")"
        if grep -q '^STATE=running$' "$state" && [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            echo "Figure ${fig#fig} is already running detached — check it with ./status.sh" >&2
            exit 1
        fi
    fi

    mkdir -p "$RESULTS_DIR/logs"
    echo "Running experiment Figure ${fig#fig} detached — estimated ${est}. Console log: ${console#"$REPO_ROOT"/} — check progress with ./status.sh"
    : >"$console"
    local tier=full
    [ "${QUICK:-0}" = "1" ] && tier=quick
    {
        echo "FIGURE=$fig"
        echo "TIER=$tier"
        echo "STATE=running"
        echo "STARTED_AT=$(date +%s)"
        echo "ESTIMATE=$est"
        echo "CONSOLE_LOG=$console"
    } >"$state"
    SMARAN_DETACHED=1 SMARAN_STATE_FILE="$state" nohup "$0" "${RUN_ARGS[@]}" >>"$console" 2>&1 &
    echo "PID=$!" >>"$state"
    exit 0
}

# _finalize_run_state <exit-code>: EXIT-trap hook — mark a detached run's
# state file done/failed. No-op for inline runs (no SMARAN_STATE_FILE).
_finalize_run_state() {
    local rc="$1" st=done
    [ -n "${SMARAN_STATE_FILE:-}" ] && [ -f "$SMARAN_STATE_FILE" ] || return 0
    [ "$rc" -eq 0 ] || st=failed
    sed -i "s/^STATE=.*/STATE=$st/" "$SMARAN_STATE_FILE"
    echo "FINISHED_AT=$(date +%s)" >>"$SMARAN_STATE_FILE"
}

# ---------------------------------------------------------------------------
# Server lifecycle (used by the query-benchmark experiment scripts)
# ---------------------------------------------------------------------------
# In remote mode the server runs on $SERVER_HOST under nohup with a pidfile;
# locally it is a plain shell child. Either way SERVER_PID/SERVER_LOG describe
# the running server, and shutdown on exit/interrupt is handled by the
# experiment-level cleanup trap in lib/experiments.sh.

SERVER_PID=""
SERVER_LOG=""
SERVER_PIDFILE=""

# Clears any process left holding the port (or recorded in the pidfile) by an
# interrupted earlier run — double Ctrl+C or a dropped SSH session can orphan
# a server that still holds the port and DB locks. Runs on the node given by
# server_host, so every run starts from a clean process slate.
_STALE_KILL_SCRIPT='
pids="$(cat "$PIDFILE" 2>/dev/null || true) $(lsof -ti "tcp:$PORT" -sTCP:LISTEN 2>/dev/null || true)"
live=""
for p in $pids; do kill -0 "$p" 2>/dev/null && live="$live $p"; done
live="$(echo "$live" | tr " " "\n" | sort -u | tr "\n" " " | sed "s/^ *//;s/ *$//")"
if [ -n "$live" ]; then
    echo "Clearing stale server (pid $live) left by an interrupted run"
    kill $live 2>/dev/null || true
    for i in $(seq 1 12); do
        alive=0
        for p in $live; do kill -0 "$p" 2>/dev/null && alive=1; done
        [ "$alive" = 0 ] && break
        sleep 5
    done
    for p in $live; do kill -9 "$p" 2>/dev/null || true; done
fi
[ -f "$PIDFILE" ] && rm -f "$PIDFILE" || true
'

# kill_stale_server <port> [pidfile]
kill_stale_server() {
    local port="$1" pidfile="${2:-}"
    if is_remote; then
        server_ctl "PORT=$port PIDFILE='$pidfile' bash -s" <<<"$_STALE_KILL_SCRIPT"
    else
        PORT="$port" PIDFILE="$pidfile" bash -s <<<"$_STALE_KILL_SCRIPT"
    fi
}

# start_server <binary> <db-dir> <port> [extra args...]
start_server() {
    local bin="$1" db="$2" port="$3"; shift 3
    local name; name="$(basename "$bin")"
    if is_remote; then
        local dir="$SERVER_RUN_DIR/server-logs"
        SERVER_LOG="$dir/${name}_port${port}.log"
        SERVER_PIDFILE="$SERVER_RUN_DIR/${name}_port${port}.pid"
        kill_stale_server "$port" "$SERVER_PIDFILE"
        say "Starting $name server on $SERVER_HOST port $port (log: $SERVER_LOG, fetched to $RESULTS_DIR/server-logs/ at end of run)"
        local extra=""
        [ $# -gt 0 ] && extra="$(printf ' %q' "$@")"
        SERVER_PID="$(server_ctl "mkdir -p '$dir' && nohup '$bin' serve --db-dir '$db' --port $port$extra >'$SERVER_LOG' 2>&1 & echo \$! | tee '$SERVER_PIDFILE'")"
    else
        mkdir -p "$RESULTS_DIR/server-logs"
        SERVER_LOG="$RESULTS_DIR/server-logs/${name}_port${port}.log"
        kill_stale_server "$port"
        say "Starting $name server on port $port (log: $SERVER_LOG)"
        "$bin" serve --db-dir "$db" --port "$port" "$@" >"$SERVER_LOG" 2>&1 &
        SERVER_PID=$!
    fi
}

# True while the started server process is alive (either mode).
server_alive() {
    [ -n "$SERVER_PID" ] || return 1
    if is_remote; then
        server_ctl "kill -0 $SERVER_PID 2>/dev/null"
    else
        kill -0 "$SERVER_PID" 2>/dev/null
    fi
}

# wait_for_server <port> <timeout-seconds> [message]
wait_for_server() {
    local port="$1" timeout="$2" msg="${3:-server}"
    local host waited=0
    host="$(server_host)"
    say "Waiting for $msg to become ready on $host:$port (up to $((timeout / 60)) min)..."
    while ! nc -z "$host" "$port" 2>/dev/null; do
        # Liveness check every 30 s (it is an ssh round trip in remote mode).
        if [ $((waited % 30)) -eq 0 ] && ! server_alive; then
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
    [ -n "$SERVER_PID" ] || return 0
    if server_alive; then
        if is_remote; then
            say "Stopping server on $SERVER_HOST (pid $SERVER_PID; Smaran closes ~1000 shard DBs, can take minutes)"
            server_ctl "kill $SERVER_PID 2>/dev/null" || true
            local waited=0
            while server_alive; do
                sleep 5
                waited=$((waited + 5))
                [ $((waited % 120)) -eq 0 ] && say "  still shutting down (${waited}s elapsed)"
                if [ "$waited" -ge 1800 ]; then
                    say "  server still up after 30 min — force-killing"
                    server_ctl "kill -9 $SERVER_PID 2>/dev/null" || true
                    break
                fi
            done
        else
            say "Stopping server (pid $SERVER_PID; Smaran closes ~1000 shard DBs, can take minutes)"
            kill "$SERVER_PID"
            wait "$SERVER_PID" 2>/dev/null || true
        fi
    fi
    if is_remote && [ -n "$SERVER_PIDFILE" ]; then
        server_ctl "rm -f '$SERVER_PIDFILE'" || true
    fi
    SERVER_PID=""
    SERVER_PIDFILE=""
}

# server_state_root <server-log-file>: extract the state root logged at startup
# (needed by the Smaran proof client for verification). The log lives on the
# server node in remote mode.
server_state_root() {
    if is_remote; then
        server_ctl "grep -oE 'state root: (0x[0-9a-fA-F]+)' '$1' | tail -1 | awk '{print \$3}'"
    else
        grep -oE 'state root: (0x[0-9a-fA-F]+)' "$1" | tail -1 | awk '{print $3}'
    fi
}

# End of every run: copy server-side logs to the client's results tree so the
# reviewer never logs into the server node. No-op in single-node mode.
fetch_server_logs() {
    is_remote || return 0
    mkdir -p "$RESULTS_DIR/server-logs"
    rsync -a "$SERVER_HOST:$SERVER_RUN_DIR/server-logs/" "$RESULTS_DIR/server-logs/" 2>/dev/null || true
}
