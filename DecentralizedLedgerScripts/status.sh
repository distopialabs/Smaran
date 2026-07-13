#!/usr/bin/env bash
# Progress of detached experiment runs (./run_figXX.sh --detach): one line
# per experiment — state, elapsed vs estimate, last progress line; finished
# runs show the figure path. Inline runs don't appear here (their console is
# their status).
source "$(dirname "$0")/lib/common.sh"

# fmt_dur <seconds> -> "3h 12m" / "7m 02s" / "45s"
fmt_dur() {
    local s="$1"
    if [ "$s" -ge 3600 ]; then
        printf '%dh %02dm' $((s / 3600)) $((s % 3600 / 60))
    elif [ "$s" -ge 60 ]; then
        printf '%dm %02ds' $((s / 60)) $((s % 60))
    else
        printf '%ds' "$s"
    fi
}

shopt -s nullglob
states=("$RESULTS_DIR"/logs/*.state)
if [ ${#states[@]} -eq 0 ]; then
    echo "No detached runs recorded yet — start one with: ./run_figXX.sh --detach"
    exit 0
fi

# state_get <file> <key>: value of a key=value line (values may contain
# spaces and shell metacharacters, so the file is parsed, not sourced).
state_get() { sed -n "s/^$2=//p" "$1" | tail -1; }

for f in "${states[@]}"; do
    FIGURE="$(state_get "$f" FIGURE)"
    TIER="$(state_get "$f" TIER)"
    STATE="$(state_get "$f" STATE)"
    PID="$(state_get "$f" PID)"
    STARTED_AT="$(state_get "$f" STARTED_AT)"
    ESTIMATE="$(state_get "$f" ESTIMATE)"
    CONSOLE_LOG="$(state_get "$f" CONSOLE_LOG)"
    FINISHED_AT="$(state_get "$f" FINISHED_AT)"
    label="Figure ${FIGURE#fig} ($TIER)"
    console_rel="${CONSOLE_LOG#"$REPO_ROOT"/}"
    case "$STATE" in
        running)
            if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
                elapsed=$(($(date +%s) - STARTED_AT))
                last="$(grep -v '^[[:space:]]*$' "$CONSOLE_LOG" 2>/dev/null | tail -1 | cut -c1-110)"
                printf '%-18s running   %s elapsed — estimated %s%s\n' \
                    "$label" "$(fmt_dur "$elapsed")" "$ESTIMATE" "${last:+ — last: $last}"
            else
                printf '%-18s DIED      process gone without finishing (interrupted?) — see %s, then rerun the script\n' \
                    "$label" "$console_rel"
            fi ;;
        done)
            took=$((FINISHED_AT - STARTED_AT))
            figline="$(grep -E '^Figure [^:]+: ' "$CONSOLE_LOG" 2>/dev/null | tail -1)"
            printf '%-18s done      took %s — %s\n' \
                "$label" "$(fmt_dur "$took")" "${figline:-see $console_rel}" ;;
        failed)
            took=$((FINISHED_AT - STARTED_AT))
            printf '%-18s FAILED    after %s — see %s\n' \
                "$label" "$(fmt_dur "$took")" "$console_rel" ;;
    esac
done
