#!/usr/bin/env bash
# Progress of detached runs — quick and full runs share the same state files.
exec "$(dirname "$0")/../DecentralizedLedgerScripts/status.sh" "$@"
