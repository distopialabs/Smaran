#!/usr/bin/env bash
# Smaran artifact: single entry point for running experiments.
#
#   ./run.sh                          interactive (menus, then runs detached)
#   ./run.sh start <mode> <scope...>  start a run (detached by default)
#   ./run.sh status                   is it running? which figure? ETA?
#   ./run.sh follow                   live log (Ctrl+C stops watching, not the run)
#   ./run.sh stop                     stop the current run
#   ./run.sh results [dest-dir]       list figures, or copy them to dest-dir
#   ./run.sh setup                    environment prep only (start runs it anyway)
#
#   mode:  smoke   pipeline check (KT ~5 min; DL ~2 min, plots from paper logs)
#          quick   reduced sweeps, same qualitative trends (KT ~2 h; DL ~50 min)
#          full    paper-scale sweeps (KT ~3 h; DL tens of hours)
#   scope: all | kt | dl | one or more figures:
#          fig4a fig4b fig4c fig5 (KT)   fig6a fig6b fig6c fig7a fig7b fig7c (DL)
#
# Everything runs detached: closing your terminal never loses a run.
set -euo pipefail

REPO="$(cd "$(dirname "$0")" && pwd)"
cd "$REPO"
AE_DIR="$REPO/.ae"
STATE="$AE_DIR/state"
LOG="$AE_DIR/run.log"

KT_FIGS="fig4a fig4b fig4c fig5"
DL_FIGS="fig6a fig6b fig6c fig7a fig7b fig7c"

# ---------------------------------------------------------------- helpers ---

is_kt_fig() { case "$1" in fig4*|fig5) return 0;; *) return 1;; esac; }

script_for() { # <mode> <fig>
    local mode=$1 fig=$2
    case "$mode" in
        quick) if is_kt_fig "$fig"; then echo "./QuickTesting-KeyTransparency/run_${fig}_quick.sh"
               else echo "./QuickTesting-DecentralizedLedgerScripts/run_${fig}.sh"; fi ;;
        full)  if is_kt_fig "$fig"; then echo "./KeyTransparencyScripts/run_${fig}.sh"
               else echo "./DecentralizedLedgerScripts/run_${fig}.sh"; fi ;;
    esac
}

eta_minutes() { # <mode> <fig>  -> rough minutes (from measured quick-tier times)
    local mode=$1 fig=$2
    case "$mode:$fig" in
        quick:fig4a|quick:fig4b|quick:fig4c) echo 20 ;;
        quick:fig5) echo 55 ;;
        quick:fig6a) echo 16 ;;   quick:fig6b|quick:fig6c) echo 1 ;;
        quick:fig7a) echo 11 ;;   quick:fig7b) echo 9 ;;   quick:fig7c) echo 12 ;;
        full:fig4a|full:fig4b|full:fig4c) echo 50 ;;
        full:fig5) echo 30 ;;
        full:fig6a) echo 1500 ;;  full:fig6b|full:fig6c) echo 1 ;;
        full:fig7a) echo 1140 ;;  full:fig7b) echo 90 ;;   full:fig7c) echo 300 ;;
        *) echo 5 ;;
    esac
}

expand_scope() { # <scope...> -> figure list (order: KT then DL)
    local out="" s
    for s in "$@"; do
        case "$s" in
            all) out="$out $KT_FIGS $DL_FIGS" ;;
            kt)  out="$out $KT_FIGS" ;;
            dl)  out="$out $DL_FIGS" ;;
            fig4a|fig4b|fig4c|fig5|fig6a|fig6b|fig6c|fig7a|fig7b|fig7c) out="$out $s" ;;
            *) echo "Unknown scope '$s' (use: all, kt, dl, or figure ids)" >&2; return 2 ;;
        esac
    done
    echo "$out"
}

state_get() { grep "^$1=" "$STATE" 2>/dev/null | tail -1 | cut -d= -f2-; }
state_set() { echo "$1=$2" >> "$STATE"; }

running_pid() {
    local pid; pid="$(state_get pid)"
    [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && echo "$pid" || true
}

# ------------------------------------------------------------------ setup ---

do_setup() {
    echo "[setup] repo: $REPO ($(git branch --show-current 2>/dev/null || echo '?') @ $(git log -1 --format=%h 2>/dev/null || echo '?'))"

    # Both usecases run on two nodes: this one (the client) drives the server
    # over SSH. The CloudLab profile provides the hostname node1; own-server
    # setups define SERVER_HOST in /local/cluster.env (see README, Install).
    SERVER_NODE=""
    if [ -f /local/cluster.env ]; then
        SERVER_NODE="$(. /local/cluster.env 2>/dev/null; echo "${SERVER_HOST:-}")"
    fi
    if [ -z "$SERVER_NODE" ] && getent hosts node1 >/dev/null 2>&1; then SERVER_NODE=node1; fi
    if [ -z "$SERVER_NODE" ]; then
        echo "[setup] ERROR: no server node found."
        echo "        The artifact runs on two nodes. Provide a hostname 'node1' (CloudLab"
        echo "        profile does this) or set SERVER_HOST in /local/cluster.env."
        echo "        See README: Install (Paths B/C)."
        exit 1
    fi

    # Inter-node SSH.
    mkdir -p "$HOME/.ssh"; chmod 700 "$HOME/.ssh"; touch "$HOME/.ssh/authorized_keys"

    # Any CloudLab node (our profile OR a manual experiment) can fetch the
    # shared experiment key with geni-get; each node that runs setup
    # self-authorizes it. Our profile does this at boot; on a manual CloudLab
    # experiment, run './run.sh setup' once on each node.
    if [ ! -f "$HOME/.ssh/id_cloudlab" ] && command -v geni-get >/dev/null 2>&1; then
        if geni-get key > "$HOME/.ssh/id_cloudlab" 2>/dev/null && [ -s "$HOME/.ssh/id_cloudlab" ]; then
            chmod 600 "$HOME/.ssh/id_cloudlab"
            ssh-keygen -y -f "$HOME/.ssh/id_cloudlab" > "$HOME/.ssh/id_cloudlab.pub"
        else
            rm -f "$HOME/.ssh/id_cloudlab"
        fi
    fi

    if [ -f "$HOME/.ssh/id_cloudlab" ]; then
        grep -qxF "$(cat "$HOME/.ssh/id_cloudlab.pub")" "$HOME/.ssh/authorized_keys" \
            || cat "$HOME/.ssh/id_cloudlab.pub" >> "$HOME/.ssh/authorized_keys"
        # NB: an IdentityFile in ssh config replaces the default key list, so
        # list the ed25519 key too or a working default would stop working.
        if ! grep -q id_cloudlab "$HOME/.ssh/config" 2>/dev/null; then
            printf 'Host node0 node1 %s\n    IdentityFile %s\n    IdentityFile %s\n    StrictHostKeyChecking accept-new\n' \
                "$SERVER_NODE" "$HOME/.ssh/id_cloudlab" "$HOME/.ssh/id_ed25519" >> "$HOME/.ssh/config"
            chmod 600 "$HOME/.ssh/config"
        fi
    fi
    ssh-keyscan -H node0 "$SERVER_NODE" >> "$HOME/.ssh/known_hosts" 2>/dev/null || true

    # If the CloudLab key didn't get us in (or there is none), fall back to an
    # ed25519 key: self-authorize and install it on the server. The push may
    # prompt for the server's password once; afterwards it's passwordless.
    if ! ssh -o BatchMode=yes -o ConnectTimeout=5 -o LogLevel=ERROR "$SERVER_NODE" true 2>/dev/null; then
        [ -f "$HOME/.ssh/id_ed25519" ] || ssh-keygen -t ed25519 -N '' -f "$HOME/.ssh/id_ed25519" -q
        grep -qxF "$(cat "$HOME/.ssh/id_ed25519.pub")" "$HOME/.ssh/authorized_keys" \
            || cat "$HOME/.ssh/id_ed25519.pub" >> "$HOME/.ssh/authorized_keys"
        cat "$HOME/.ssh/id_ed25519.pub" | ssh -o StrictHostKeyChecking=accept-new "$SERVER_NODE" \
            'mkdir -p ~/.ssh && touch ~/.ssh/authorized_keys && { grep -qxF "$(cat)" ~/.ssh/authorized_keys || cat >> ~/.ssh/authorized_keys; }' \
            || true
    fi

    if ssh -o BatchMode=yes -o ConnectTimeout=5 -o LogLevel=ERROR "$SERVER_NODE" true 2>/dev/null; then
        echo "[setup] inter-node SSH to $SERVER_NODE: OK"
    else
        echo "[setup] ERROR: passwordless SSH to $SERVER_NODE is not working."
        echo "        Manual CloudLab experiment: run './run.sh setup' on $SERVER_NODE too, then rerun here."
        echo "        Own servers: install $HOME/.ssh/id_ed25519.pub into $SERVER_NODE's ~/.ssh/authorized_keys"
        echo "        (e.g. ssh-copy-id), then rerun."
        exit 1
    fi

    # Go on PATH for non-interactive shells.
    [ -e /usr/local/bin/go ] || sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null || true

    # KT node config (defaults match the CloudLab profile).
    [ -f KeyTransparencyScripts/nodes.env ] || cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env

    # Clean stale KT state from any prior run.
    sudo pkill -9 coniksserver ktserver ktbench coniksbench 2>/dev/null || true
    sudo rm -f /tmp/coniks.sock 2>/dev/null || true
    ssh -o StrictHostKeyChecking=no -o LogLevel=ERROR "$SERVER_NODE" \
        'sudo pkill -9 -f "coniks|ktserver|ktbench" 2>/dev/null; sudo rm -f /tmp/coniks.sock' 2>/dev/null || true
    echo "[setup] done"
}

# ----------------------------------------------------------------- worker ---

do_worker() { # <mode> <fig...>   (runs inside the detached process)
    local mode=$1; shift
    local figs=("$@") total=${#figs[@]} i=0 fig script
    for fig in "${figs[@]}"; do
        i=$((i+1))
        if [ "$mode" = smoke ]; then
            if is_kt_fig "$fig"; then script=./KeyTransparencyScripts/smoke_test.sh
            else script=./DecentralizedLedgerScripts/plot_paper_figures.sh; fi
        else
            script="$(script_for "$mode" "$fig")"
        fi
        state_set current "$script ($i/$total)"
        echo
        echo "===== [$i/$total] $(date '+%H:%M:%S')  $script"
        "$script" || { state_set state failed; state_set failed_step "$script"; exit 1; }
    done
    state_set state done
}

do_start() { # <mode> <scope...>
    local mode=$1; shift
    case "$mode" in smoke|quick|full) ;; *) echo "Unknown mode '$mode' (smoke|quick|full)"; exit 2;; esac
    [ $# -ge 1 ] || { echo "Missing scope (all|kt|dl|figure ids)"; exit 2; }

    if [ -n "$(running_pid)" ]; then
        echo "A run is already active: './run.sh status' to inspect, './run.sh stop' to abort."
        exit 1
    fi

    local figs; figs="$(expand_scope "$@")" || exit 2
    # smoke collapses each usecase to one pipeline check
    if [ "$mode" = smoke ]; then
        local has_kt=false has_dl=false f new=""
        for f in $figs; do if is_kt_fig "$f"; then has_kt=true; else has_dl=true; fi; done
        $has_kt && new="fig4a"; $has_dl && new="$new fig6a"
        figs="$new"
    fi

    local eta=0 f
    for f in $figs; do eta=$((eta + $(eta_minutes "$mode" "$f"))); done

    do_setup

    mkdir -p "$AE_DIR"
    : > "$STATE"; : > "$LOG"
    state_set mode "$mode"; state_set scope "$*"; state_set figs "$figs"
    state_set started "$(date +%s)"; state_set eta_min "$eta"; state_set state running

    setsid bash "$REPO/run.sh" _worker "$mode" $figs >> "$LOG" 2>&1 < /dev/null &
    state_set pid $!

    echo
    echo "Started ($mode: $figs), detached; safe to close this terminal."
    if [ "$eta" -ge 120 ]; then printf 'Estimated: ~%.1f hours\n' "$(echo "$eta/60" | bc -l)"; else echo "Estimated: ~$eta minutes"; fi
    echo "  ./run.sh status    progress check"
    echo "  ./run.sh follow    watch the log live"
    echo "  ./run.sh results   where the figures are"
    if [ "${FOLLOW:-0}" = 1 ]; then exec tail -f "$LOG"; fi
}

do_status() {
    [ -f "$STATE" ] || { echo "No run recorded yet. Start one with: ./run.sh start quick all"; exit 0; }
    local st mode figs started eta pid now elapsed
    st="$(state_get state)"; mode="$(state_get mode)"; figs="$(state_get figs)"
    started="$(state_get started)"; eta="$(state_get eta_min)"
    if [ "$st" = running ] && [ -z "$(running_pid)" ]; then st="died (see ./run.sh follow for the log)"; fi
    now=$(date +%s); elapsed=$(( (now - ${started:-$now}) / 60 ))
    echo "state:    $st"
    echo "run:      $mode ($figs)"
    echo "current:  $(state_get current)"
    echo "elapsed:  ${elapsed} min (estimate was ~${eta} min)"
    [ "$st" = failed ] && echo "failed:   $(state_get failed_step)"
    echo "log tail:"
    tail -5 "$LOG" 2>/dev/null | sed 's/^/  | /'
}

do_stop() {
    local pid; pid="$(running_pid)"
    [ -n "$pid" ] || { echo "Nothing running."; exit 0; }
    kill -TERM -- -"$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
    sleep 2
    sudo pkill -9 coniksserver ktserver ktbench coniksbench 2>/dev/null || true
    state_set state stopped
    echo "Stopped. Rerunning is safe: finished figures are cached, unfinished ones redo from clean state."
}

do_results() { # [dest]
    local dest="${1:-}"
    local pdfs
    pdfs="$( { ls output/*.pdf 2>/dev/null; find results -name '*.pdf' 2>/dev/null; } | sort )"
    [ -n "$pdfs" ] || { echo "No figures yet."; exit 0; }
    if [ -z "$dest" ]; then
        echo "$pdfs"
        echo
        echo "(./run.sh results <dir> copies them; from your laptop:"
        echo " bash run_ae.sh <user> <node0-host> fetch <dir>)"
    else
        mkdir -p "$dest"
        echo "$pdfs" | xargs -I{} cp {} "$dest/"
        echo "Copied $(echo "$pdfs" | wc -l) PDFs to $dest/"
    fi
}

# ------------------------------------------------------------- interactive ---

do_menu() {
    echo
    echo '======================================================='
    echo '  Smaran Artifact Evaluation'
    echo '======================================================='
    echo 'What do you want to run?'
    echo '    [1] Everything        (KT + DL)'
    echo '    [2] Key Transparency  (§7.1: Figs 4a, 4b, 4c, 5)'
    echo '    [3] Decentralized Ledger (§7.2: Figs 6a-c, 7a-c)'
    echo '    or type a figure id (e.g. fig6a)'
    read -rp '> Scope [1/2/3/figNN]: ' s
    case "$s" in 1) scope=all;; 2) scope=kt;; 3) scope=dl;; fig*) scope="$s";; *) echo 'Unrecognized.'; exit 2;; esac
    echo
    echo 'At what scale?'
    echo '    [0] SMOKE  pipeline check     (KT ~5 min / DL ~2 min)'
    echo '    [1] QUICK  reduced sweep      (KT ~2 h / DL ~50 min; same trends)'
    echo '    [2] FULL   paper scale        (KT ~3 h / DL tens of hours)'
    read -rp '> Mode [0/1/2]: ' m
    case "$m" in 0) mode=smoke;; 1) mode=quick;; 2) mode=full;; *) echo 'Unrecognized.'; exit 2;; esac
    echo
    echo "Equivalent command (skips these menus): ./run.sh start $mode $scope"
    do_start "$mode" "$scope"
}

# ------------------------------------------------------------------- main ---

cmd="${1:-}"
case "$cmd" in
    "")        if [ -t 0 ]; then do_menu; else sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; fi ;;
    start)     shift
               if [ "${!#}" = "--follow" ]; then FOLLOW=1; set -- "${@:1:$(($#-1))}"; fi
               do_start "$@" ;;
    _worker)   shift; do_worker "$@" ;;
    status)    do_status ;;
    follow)    echo '(Ctrl+C stops watching; the run keeps going)'; exec tail -f "$LOG" ;;
    stop)      do_stop ;;
    results)   shift || true; do_results "${1:-}" ;;
    setup)     do_setup ;;
    *)         sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
