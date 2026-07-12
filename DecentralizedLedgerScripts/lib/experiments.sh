#!/usr/bin/env bash
# Shared experiment machinery for the per-figure scripts (run_fig*.sh).
#
# Full-scale defaults reproduce the paper's parameters. The QuickTesting
# variants set QUICK=1 before exec'ing the same scripts, which switches to
# the advisor's quick-turnaround cutoffs (fig6/7b ranges < 10k, fig7a users
# < 0.5M, fig7c users < 0.5M and shards <= 100) with shorter durations.
# Individual knobs can also be overridden via environment variables.

_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$_LIB_DIR/common.sh"
source "$_LIB_DIR/../config.sh"
export PATH="$PATH:/usr/local/go/bin"

QUICK="${QUICK:-0}"

# --- Effective parameters ----------------------------------------------------
# Defaults come from ../config.sh (FULL_* or QUICK_* per the QUICK flag);
# any of these names can be overridden per-run via the environment.
NUM_CLIENTS="${NUM_CLIENTS:-$DEFAULT_NUM_CLIENTS}"
DB_ROOT="${DB_ROOT:-$DEFAULT_DB_ROOT}"
COOLDOWN="${COOLDOWN:-$DEFAULT_COOLDOWN}"

# Distinct ports so a stray server never collides with another protocol's run.
PORT_MERKLE=50061
PORT_VERKLE=50062
PORT_SMARAN=50063

# _pick <name>: value of QUICK_<name> or FULL_<name> depending on tier
_pick() { local v="FULL_$1"; [ "$QUICK" = "1" ] && v="QUICK_$1"; echo "${!v}"; }

N_BLOCKS="${N_BLOCKS:-$(_pick N_BLOCKS)}"
PROOF_DURATION="${PROOF_DURATION:-$(_pick PROOF_DURATION)}"
RANGES_SMARAN=(${RANGES_SMARAN:-$(_pick RANGES_SMARAN)})
RANGES_MERKLE=(${RANGES_MERKLE:-$(_pick RANGES_MERKLE)})
RANGES_VERKLE=(${RANGES_VERKLE:-$(_pick RANGES_VERKLE)})
RANGES_7B=(${RANGES_7B:-$(_pick RANGES_7B)})
K_USERS_LIST=(${K_USERS_LIST:-$(_pick K_USERS)})
SHARDS_LIST=(${SHARDS_LIST:-$(_pick SHARDS)})
INGEST_BENCH_BLOCKS="${INGEST_BENCH_BLOCKS:-$(_pick INGEST_BENCH_BLOCKS)}"
SHARDS_BENCH_BLOCKS="${SHARDS_BENCH_BLOCKS:-$(_pick SHARDS_BENCH_BLOCKS)}"
SHARDS_BENCH_DURATION="${SHARDS_BENCH_DURATION:-$(_pick SHARDS_BENCH_DURATION)}"

# --- Per-run setup guard ------------------------------------------------------
# Quiet check of exactly what this figure's script needs; fails fast with a
# remedy before any benchmark work (require_setup in common.sh). FIGURE_ID is
# derived from the calling script's name (run_fig6a.sh -> fig6a).
FIGURE_ID="$(basename "$0" .sh)"
FIGURE_ID="${FIGURE_ID#run_}"
case "$FIGURE_ID" in
    fig6a | fig6b | fig6c | fig7b)
        require_setup binaries data-local blocks account-stats params plot-deps server ;;
    fig7a | fig7c)
        require_setup binaries data-local blocks account-stats-50k plot-deps server ;;
esac

# --- Run mode: inline (default) or --detach -----------------------------------
parse_run_flags "$@"
maybe_detach   # exits here in the parent of a --detach run

# From this point on we are the real run (inline, or the detached child).
# One cleanup trap for the whole experiment: stop any server we started and,
# for detached runs, finalize the state file. INT/TERM route through EXIT so
# Ctrl+C also stops servers.
_experiment_cleanup() {
    local rc=$?
    stop_server
    _finalize_run_state "$rc"
}
trap _experiment_cleanup EXIT
trap 'exit 130' INT TERM

# Full-scale runs are hours long; recommend --detach when run inline.
if [ "$QUICK" != "1" ] && [ -z "${SMARAN_DETACHED:-}" ] && [[ "$FIGURE_ID" == fig* ]]; then
    echo "NOTE: this is a full-scale run, estimated $(figure_estimate "$FIGURE_ID") — recommended: $0 --detach (survives SSH disconnects; check progress with ./status.sh)"
fi

# --- Protocol helpers -------------------------------------------------------
# Reviewer-facing name is "Smaran"; binaries/dirs use the old name "samurai",
# and the query benchmark logs use "samuraimpt".

proto_bin()   { case "$1" in smaran) echo "$REPO_ROOT/bin/samurai";; *) echo "$REPO_ROOT/bin/$1";; esac; }
proto_label() { case "$1" in smaran) echo "Smaran";; merkle) echo "Merkle";; verkle) echo "Verkle";; esac; }
proto_port()  { case "$1" in smaran) echo "$PORT_SMARAN";; merkle) echo "$PORT_MERKLE";; verkle) echo "$PORT_VERKLE";; esac; }
proto_client(){ case "$1" in smaran) echo "$REPO_ROOT/bin/proofc";; merkle) echo "$REPO_ROOT/bin/merkle-proofc";; verkle) echo "$REPO_ROOT/bin/verkle-proofc";; esac; }
proto_ranges(){ case "$1" in smaran) echo "${RANGES_SMARAN[@]}";; merkle) echo "${RANGES_MERKLE[@]}";; verkle) echo "${RANGES_VERKLE[@]}";; esac; }

# --- Accounts list ----------------------------------------------------------
# Query benchmarks pick accounts from a stats CSV; the accounts must exist in
# the ingested block window or the client reports NotFound errors. Full-scale
# runs use the repo's account_stats_all.csv; quick runs generate a list
# matched to the exact ingested window (cached).
accounts_list_for_window() {
    if [ "$QUICK" != "1" ]; then
        echo "$REPO_ROOT/account_stats_all.csv"
        return
    fi
    local out="$RESULTS_DIR/accounts/account_stats_first${N_BLOCKS}.csv"
    if [ ! -f "$out" ]; then
        say "Generating accounts list for the first $N_BLOCKS blocks (one-time, cached)" >&2
        mkdir -p "$(dirname "$out")"
        (cd "$REPO_ROOT" && go run ./cmd/tools/count_account_updates -n "$N_BLOCKS" -o "$out") >&2
    fi
    echo "$out"
}

# --- Ingested-DB cache ------------------------------------------------------
# ensure_ingested <protocol>: ingest N_BLOCKS once per (protocol, N_BLOCKS);
# later runs reuse the DB. Prints the DB dir.
ensure_ingested() {
    local proto="$1"
    local db="$DB_ROOT/${proto}_n${N_BLOCKS}"
    local marker="$db/.ingest-complete"
    if [ -f "$marker" ]; then
        say "Reusing ingested $(proto_label "$proto") database at $db" >&2
        echo "$db"
        return
    fi
    say "Ingesting $N_BLOCKS blocks into a fresh $(proto_label "$proto") database (one-time; cached at $db)" >&2
    if [ "$proto" = "smaran" ]; then
        say "NOTE: Smaran creates ~1000 shard databases before ingesting — expect several minutes of setup and teardown around the ingest itself." >&2
    fi
    rm -rf "$db"
    (cd "$REPO_ROOT" && "$(proto_bin "$proto")" ingest --db-dir "$db" -n "$N_BLOCKS" --fresh) >&2
    touch "$marker"
    echo "$db"
}

# --- Query benchmark sweep --------------------------------------------------
# run_proof_sweep <protocol> <output-dir> <mode> [ranges...]
#   mode: default | optimus | non_optimus   (optimus/non_optimus: smaran only;
#   default for smaran = optimus, i.e. stored roots / no --old)
# Starts the protocol's server against the cached DB, waits for readiness,
# sweeps the protocol's range sizes (or the explicit ranges argument), then
# stops the server.
run_proof_sweep() {
    local proto="$1" outdir="$2" mode="${3:-default}"
    shift $(( $# < 3 ? $# : 3 ))
    local ranges=("$@")
    [ ${#ranges[@]} -gt 0 ] || ranges=($(proto_ranges "$proto"))
    local db port client accounts label
    db="$(ensure_ingested "$proto")"
    port="$(proto_port "$proto")"
    client="$(proto_client "$proto")"
    accounts="$(accounts_list_for_window)"
    label="$(proto_label "$proto")"

    start_server "$(proto_bin "$proto")" "$db" "$port"
    local ready_timeout=900
    [ "$proto" = "smaran" ] && ready_timeout=2400   # opens ~1000 shard DBs on startup
    wait_for_server "$port" "$ready_timeout" "$label server"

    local extra=()
    if [ "$proto" = "smaran" ]; then
        local server_log="$RESULTS_DIR/server-logs/samurai_port${port}.log"
        local root
        root="$(server_state_root "$server_log")"
        [ -n "$root" ] || die "could not read state root from $server_log"
        extra=(--params-dir "$REPO_ROOT/data/params" --state-root "$root")
        [ "$mode" = "non_optimus" ] && extra+=(--old)
    fi

    mkdir -p "$outdir"
    for range in "${ranges[@]}"; do
        if [ "$mode" = "default" ]; then
            echo "Running $label with query range size $range"
        else
            echo "Running $label ($mode) with query range size $range"
        fi
        (cd "$REPO_ROOT" && "$client" bench --verify \
            "${extra[@]}" \
            --server-addr "localhost:$port" \
            --accounts-list "$accounts" \
            --num-clients "$NUM_CLIENTS" \
            --duration "$PROOF_DURATION" \
            --range-size "$range" \
            --output-dir "$outdir")
        sleep "$COOLDOWN"
    done
    stop_server
}

# --- Ingestion benchmark point ----------------------------------------------
# run_ingest_bench <protocol> <k-users> <output-dir> <blocks> <duration> [shards]
# blocks is the primary limit for fig7a (duration is a deadline); fig7c is
# duration-limited as in the paper.
run_ingest_bench() {
    local proto="$1" k="$2" outdir="$3" blocks="$4" duration="$5" shards="${6:-}"
    local extra=()
    [ "$proto" = "smaran" ] && extra+=(--skip-mpt)   # paper's ingestion numbers are samurai-only (KZG)
    [ -n "$shards" ] && extra+=(--shards "$shards")
    mkdir -p "$outdir"
    (cd "$REPO_ROOT" && "$(proto_bin "$proto")" bench-ingest \
        "${extra[@]}" \
        --db-dir "/data/local/tmp/bench-${proto}$$" \
        -n "$blocks" \
        --duration "$duration" \
        --k-users "$k" \
        --accounts-list "$REPO_ROOT/account_stats_50k.csv" \
        --output-dir "$outdir")
    rm -rf "/data/local/tmp/bench-${proto}$$"
}

# --- Cauchy (prebaked only) --------------------------------------------------
# Cauchy lives in a separate Rust codebase and is far too slow to rerun; the
# paper's cauchy logs ship in the prebaked bundle. If the bundle is reachable,
# stage its cauchy logs so the figure includes the Cauchy series; otherwise
# plot without it.
stage_cauchy() {
    local what="$1" dest="$2"      # what: fig7a | fig6 (source subdir in the bundle)
    local bundle src
    if ! bundle="$(resolve_paper_logs 2>/dev/null)"; then
        say "Paper-logs bundle not found — plotting without the Cauchy series (see README)"
        return 1
    fi
    case "$what" in
        fig7a) src="$bundle/fig7a/cauchy" ;;
        fig6)  src="$bundle/fig6/numclients32/cauchy" ;;
    esac
    [ -d "$src" ] || return 1
    mkdir -p "$dest"
    cp -r "$src"/. "$dest"/
    find "$dest" -name '*.gz' -exec gunzip -f {} +
    say "Staged prebaked Cauchy logs into $dest"
}

# --- Figure 6 shared pipeline -------------------------------------------------
# Figures 6a, 6b and 6c come from ONE query-benchmark sweep. The first
# run_fig6*.sh executes the sweep and caches the logs; the other two reuse
# them and only re-plot. Set FORCE_RERUN=1 to redo the benchmarks.
run_fig6_pipeline() {
    local logs="$RESULTS_DIR/fig6/logs"
    local out="$RESULTS_DIR/fig6"
    local clients_dir="$logs/numclients${NUM_CLIENTS}"
    local marker="$logs/.complete-n${N_BLOCKS}-c${NUM_CLIENTS}-d${PROOF_DURATION}"

    if [ -f "$marker" ] && [ "${FORCE_RERUN:-0}" != "1" ]; then
        say "Reusing benchmark logs from a previous Figure 6 run ($logs; FORCE_RERUN=1 to redo)"
    else
        # A fresh sweep must not inherit CSVs from an earlier or aborted run:
        # the plot reads every proof_range*.csv in these dirs, and the rename
        # step keeps whichever file got a clean name first.
        rm -rf "$clients_dir/merkle" "$clients_dir/verkle" "$clients_dir/samuraimpt"
        rm -f "$logs"/.complete-*
        for proto in merkle verkle smaran; do
            run_proof_sweep "$proto" "$clients_dir"
        done
        # fig6_query.py reads clean proof_range<R>.csv names
        for d in "$clients_dir"/merkle "$clients_dir"/verkle "$clients_dir"/samuraimpt; do
            [ -d "$d" ] && "$REPO_ROOT/scripts/benchmark/rename_proof_csvs.sh" "$d"
        done
        touch "$marker"
    fi

    stage_cauchy fig6 "$clients_dir/cauchy" || true

    echo "Plotting"
    (cd "$REPO_ROOT" && python3 scripts/paper-figures/fig6_query.py "$logs" --output "$out")
    say "Figures written to $out/numclients${NUM_CLIENTS}/"
}
