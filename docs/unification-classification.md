# KT + DL Unification — Phase 1 Classification

Branch: `unified-artifact` (cut from `timing_debug` @ 6d6c3da).
KT source: `origin/KT-artifact` @ 02de08f (strictly ahead of `origin/kt`).
Common ancestor of both usecases: 8133e1f ("fix: end block one more than actual bug fixed").

## Decision rules (agreed with Asim, 2026-07-12)

- **R1 — import as-is**: KT-only file, or DL deleted it while KT kept/evolved it.
- **R2 — port addition**: KT added something to a shared file DL also evolved; add it to DL's file (additive only, DL behavior unchanged).
- **R3 — mechanical adaptation**: KT call site updated to DL's reshaped API (compile-checked rename/boxing, no logic change).
- **R4 — scoped redundancy**: true semantic divergence; KT keeps its own copy in KT-owned territory, DL's version untouched.
- **R0 — skip**: not needed by the KT artifact flow, or cosmetic churn we don't take.
- **CONV — already converged**: both sides independently made the same change; DL's file already satisfies KT.

## Scope anchor: what the KT artifact actually runs

KT reviewer scripts invoke only `bin/ktserver`, `bin/ktbench`, and the
Coniks/Optiks binaries (`bin/samurai` and `bin/proofc` are built by
`install_smaran.sh` but never invoked). Import closure of
`cmd/ktserver` + `cmd/ktbench`:

```
cmd/ktbench, cmd/ktserver
  -> internal/kt, internal/logging            (KT-only)
  -> internal/proof, internal/tree            (shared, DIVERGED — see R4)
  -> internal/config, internal/crypto/{hash,kzg,polynomial},
     internal/utils, internal/db (via tree)   (shared, compatible)
```

Everything KT-modified outside this closure is R0 unless noted.

## Key evidence driving the split

1. **`internal/tree` forked semantically.** KT: `MaxLayer = 1`, new
   `LeafNodeIdx` / `LXLeafNodes` / `CurrentLXTreeCounts` forest structures,
   `AccountInfo` gained `HistoricalBalances []*HistoricalBalance` +
   `UpdateInMemory`. DL: `MaxLayer = 4`, `AccountInfo` gained
   `Update/UpdateBulk/Save` methods on `db.SamuraiStore`. `MaxLayer` is a
   compile-time constant that fixes array shapes (`[MaxLayer]...`) and DB key
   layout — one binary cannot serve both values.
2. **Persistence off-by-one fork.** KT changed `lastLeafNodeIdx := version - 1`
   to `version` in `Store/GetLXBatchCommitments`; DL kept `version - 1`
   (3 sites). Consistent within each world, incompatible across them.
3. **`internal/proof` is welded to tree geometry** (`tree.MaxLayer`,
   batch-index math), so it forks with `tree` — even though the KZG
   verification algorithm itself is line-for-line convergent (see below).
4. **Convergent evolution elsewhere.** `polynomial.VanishingPolynomial`
   (ω-domain signature) identical on both sides. `crypto/kzg` differs only by
   KT's logging swap + DL's added `SyntheticDivide`. DL's
   `VerifyNewRangeProofs` = KT's FFT rewrite + MPT trust-anchor prologue
   (skipped when the 3 extra args are nil) + error returns instead of panics.
5. **KT's "missing symbols" from Shistuu's MERGE-PLAN.md** were mostly KT-side
   export renames/additions, not DL deletions. With the R4 package split, all
   six MERGE-PLAN API-mismatch items become moot.

## Classification table

### R4 — KT-scoped package copies (the redundancy valve)

| Unified location | Source (verbatim from KT-artifact) | Why |
|---|---|---|
| `internal/kt/tree/` | `internal/tree/` | MaxLayer 1 vs 4; off-by-one; KT-only forest structures |
| `internal/kt/proof/` | `internal/proof/` | welded to KT tree geometry (incl. KT's `rebuild.go`, which DL deleted) |

Only edits allowed: package clause / import paths (`internal/tree` →
`internal/kt/tree` etc.). Function bodies byte-identical to KT-artifact.

### R1 — import as-is

| Path | Note |
|---|---|
| `internal/kt/` (samurai.go, server.go, optiks.go + tests) | imports rewritten to `internal/kt/{tree,proof}` |
| `internal/logging/` | KT-only structured logger |
| `cmd/ktserver/`, `cmd/ktbench/` | imports rewritten to KT-scoped packages |
| `Coniks` submodule + `.gitmodules` | baseline system |
| `Optiks/` | baseline system (plain dir) |
| `KeyTransparencyScripts/`, `QuickTesting-KeyTransparency/` | top-level siblings of the DL script dirs |
| `experiments/` (kt.py, kt_plot.py, kt_put_plot.py, common.py, …) | KT sweep driver; also contains dl_query_plot.py/optimus.py (already known from kt-put-throughput) |
| `reference_pdfs/` | KT expected-output PDFs |
| `run_kt.sh`, `run_ae.sh`, `setup_cloudlab.sh`, `run_dl.sh` | top-level menus — adapted in Phase 5 |
| `KT.md`, `KT_Samurai.md` | design notes → `docs/` |

### R2 — additive ports into shared files

| File | Addition |
|---|---|
| `internal/db/pebble.go` | `NewInMemoryPebbleDB()` (+ vfs import) — purely additive |

### R3 — mechanical adaptations (compile-checked only)

| Where | Adaptation |
|---|---|
| `internal/db/compat.go` | `type SamuraiDB = SamuraiStore` alias (DL renamed the struct; fields identical) — lets KT code compile unchanged |
| `KeyTransparencyScripts/configs/*.toml` | `repo_url` now `@REPO_URL@` (KT_REPO_URL was documented in nodes.env.template but never wired up); remote `build_command` ends in `make build-kt setup-coniks-server setup-coniks-client` instead of bare `make`, which on the unified branch would build the DL set |

The anticipated `VerifyNewRangeProofs` adaptation is moot under the R4 split.

### CONV — already converged, DL file satisfies KT

| File | Evidence |
|---|---|
| `internal/crypto/polynomial/polynomial.go` | identical `VanishingPolynomial(xs, omega)` both sides |
| `internal/crypto/kzg/barycentric4096.go` | logic identical; DL adds `SyntheticDivide`; KT diff is logging-only |

### R0 — skip (with reason)

| Path | Reason |
|---|---|
| `internal/account`, `internal/benchmark`, `internal/fetcher` | outside ktserver/ktbench closure; DL deleted account/benchmark/fetcher deliberately |
| `cmd/stress`, `cmd/tools/mptproofs`, `cmd/tools/debug_version`, `cmd/tools/makedataset` (KT mods) | dev tools outside the artifact flow; DL's versions (where they exist) stay |
| `cmd/samurai/*`, `cmd/proofc` (KT mods) | KT scripts build but never invoke them; DL's binaries build fine and keep the names |
| `internal/server`, `internal/storage` (KT mods) | outside closure |
| `internal/tree/helpers.go`, `internal/crypto/polynomial/barycentric4096.go`, `cmd/tools/count_account_updates` (KT mods) | cosmetic logging swaps only |
| `cloudlab-profile/` | retired — our `profile.py` covers both usecases (KT deps added in Phase 5) |
| KT `README.md` (as root) | superseded by unified landing README (Phase 5); KT content preserved under `KeyTransparencyScripts/README.md` |
| `.DS_Store`, `sandbox.py` | noise |

### Infra merges (Phase 2/5)

| File | Treatment |
|---|---|
| `Makefile` | DL's + KT's `ktserver`/`ktbench`/coniks targets |
| `go.mod` / `go.sum` | dependency union (KT adds pebble/vfs use, toml, etc.), `go mod tidy`; go 1.25 toolchain builds KT's 1.24 code |
| `.gitignore` | union |
| `README.md` | unified landing page; DL guide retained (Phase 5) |

## Verification gates (Phase 6)

1. `go build ./...` + `go vet ./...` clean on unified branch.
2. `go test ./...` (brings KT's `samurai_test.go`, `optiks_test.go`).
3. Guardrail diff: `git diff timing_debug..unified-artifact -- <DL closure>` shows
   additions only (pebble.go) — DL behavior provably untouched.
4. DL quick-tier figure smoke on the live pair.
5. KT `smoke_test.sh` → quick sweep on the live pair; figures vs `reference_pdfs/`.

## Open items for Asim

1. Confirm `MaxLayer = 1` is the intended published KT configuration (the
   KT-artifact reference PDFs were produced with it). If it was a leftover
   dev toggle, the R4 split still works — but worth knowing.
2. Naming: `internal/kt/tree` + `internal/kt/proof` vs `internal/kttree` +
   `internal/ktproof`. Default: the former (keeps KT-owned code under one roof).

Both resolved 2026-07-12: MaxLayer=1 confirmed intended for KT;
`internal/kt/{tree,proof}` naming approved.

## Implementation notes (Phases 2-5)

- Pre-existing breakage, left untouched: `cmd/tools/debug_version` does not
  compile on `timing_debug` itself (passes `*db.PebbleDB` where `*db.DB` is
  expected); `TestSamuraiMPTReusableAfterProve` fails identically on pristine
  KT-artifact and on this branch (verified per-test parity of
  `go test ./internal/kt/`).
- The Coniks submodule is recorded as .gitmodules + gitlink
  (ed679f92, exactly KT-artifact's pin) without fetching; install_coniks.sh
  / kt.py run `git submodule update --init` on the nodes as before.
- `go.mod` additions: github.com/op/go-logging (internal/logging).
- KT's separate `cloudlab-profile/` retired; `profile.py` (REPO_REF ->
  unified-artifact) + `cloudlab/setup-node.sh` (protobuf-compiler,
  `make build-kt`, nodes.env seed) serve both usecases.
