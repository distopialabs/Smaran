# Unified Branch — Merge Plan

Goal: one branch (`main`) that builds and runs both KT (§7.1) and DL (§7.2) experiments. Currently split across `kt` and `timing_debug`.

## What actually conflicts

**Not conflicting (bring over as-is):**
- KT-only `internal/` packages: `account`, `benchmark`, `fetcher`, `kt`, `logging`
- DL-only `internal/` packages: `benchutil`, `ingest`, `merkle`, `verkle`, `verklekzg`
- KT-only `cmd/`: `ktbench`, `ktserver`, `stress`, `tools/mptproofs`, `tools/makedataset`, `tools/debug_version`, plus `samurai/commands/{commit_bench,serve,verify}.go`
- DL-only `cmd/`: `merkle`, `merkle-proofc`, `samuraiold`, `api/proto/samurai/`
- All AE infrastructure (KeyTransparencyScripts, QuickTesting-KeyTransparency, run_ae.sh, setup_cloudlab.sh, run_kt.sh, README-AE.md, reference_pdfs, cloudlab-profile) — kt only, no conflict

**Truly conflicting** (timing_debug's refactor of shared packages broke KT's API assumptions):

## API mismatches to reconcile

Each row below is a decision Asim needs to make (or explain to Shistata). Format: what KT expects → what timing_debug provides → resolution options.

### 1. `internal/db` — concrete type became interface

| File | KT expects | timing_debug has | Resolution |
|---|---|---|---|
| `cmd/tools/debug_version/main.go:58,69` | Pass `*db.PebbleDB` to `tree.GetCurrentBalanceInfo` / `tree.GetHistoricalBalance` | `db.DB` is now an interface; `tree.Get*` accepts `*db.DB` | (a) At call site: dereference `*db.PebbleDB` and box into `db.DB` interface. (b) Change `tree.Get*` back to accepting `*db.PebbleDB` directly. |

### 2. `internal/proof` — function signature widened

| File | KT call | timing_debug signature | Resolution |
|---|---|---|---|
| `cmd/ktbench/main.go:696` | `VerifyNewRangeProofs(addr, number, endBlock, proofs, balances, precomputed)` — 6 args | `VerifyNewRangeProofs(addr, uint64, uint64, proofs, balances, precomputed, [][]byte, common.Hash, *tree.CurrentBalance)` — 9 args | Need to know what the 3 new args (`[][]byte`, `common.Hash`, `*tree.CurrentBalance`) semantically mean and whether KT can supply reasonable defaults. |

### 3. `internal/proof` — functions deleted

| Function | Used by | Was it renamed or removed entirely? |
|---|---|---|
| `proof.FindCommitmentsCoveringRange` | `internal/kt/samurai.go:220` | ? |
| `proof.FindNodesToInterpolate` | `internal/kt/samurai.go:242` | ? |

If renamed, patch the KT calls. If removed, either restore or reimplement in KT.

### 4. `internal/tree.AccountInfo` — field and method deleted

| Symbol | Used by | Resolution |
|---|---|---|
| `AccountInfo.HistoricalBalances` (field) | `internal/kt/samurai.go:190,192` | Was it moved elsewhere or replaced by a method? |
| `AccountInfo.UpdateInMemory` (method) | `internal/kt/samurai.go:384,401` | Was in-memory update semantics changed? |

### 5. `internal/db.SamuraiDB` — type deleted or renamed

| Used by | Resolution |
|---|---|
| `cmd/samurai/commands/serve.go:12` | What replaces `db.SamuraiDB` on timing_debug? |

### 6. `cmd/samurai/commands/` — helper symbols missing

| File | Undefined | Note |
|---|---|---|
| `cmd/samurai/commands/commit_bench.go` | `log`, `BlockInfo`, `UpdateTask` | These are package-level helpers/types that existed alongside `commit_bench.go` on `kt`. They may have been `cmd/samurai/flags.go` or similar sibling files on kt that timing_debug flattened away. Need to check what side each helper should live on. |

## Suggested plan of the call (30–45 min)

1. Walk through the API mismatches above one at a time.
2. For each, Asim states: was this a rename (easy) or a semantic change (need to think)?
3. Shistata writes down the resolution.
4. After the call, Shistata (or Asim) implements each fix and re-runs `go build ./...` until it's clean.
5. Then `./KeyTransparencyScripts/smoke_test.sh` to verify KT still works, and Asim's equivalent to verify DL still works.
6. Push to a new branch `main` (or overwrite existing `main`).

## What the merge looks like once decisions are made

```bash
git checkout timing_debug
git checkout -b main-candidate

# Bring KT-only additions verbatim (no conflict possible)
git checkout origin/kt -- \
  internal/account internal/benchmark internal/fetcher internal/kt internal/logging \
  cmd/ktbench cmd/ktserver cmd/stress \
  cmd/samurai/commands/commit_bench.go cmd/samurai/commands/serve.go cmd/samurai/commands/verify.go \
  cmd/tools/mptproofs cmd/tools/makedataset cmd/tools/debug_version \
  KeyTransparencyScripts QuickTesting-KeyTransparency cloudlab-profile reference_pdfs \
  setup_cloudlab.sh run_kt.sh run_ae.sh README-AE.md \
  experiments/kt.py experiments/kt_plot.py experiments/kt_put_plot.py

# Apply the API-mismatch fixes decided in the call (edit files)
# ... edits per section 1-6 above ...

go mod tidy
go build ./...              # must succeed
./KeyTransparencyScripts/smoke_test.sh    # KT still works
# (Asim's DL smoke test)                  # DL still works

git add .
git commit -m 'Unify KT and DL onto one branch'
git push origin main-candidate
# then rename main-candidate → main on GitHub after PR review
```
