# Smaran artifact disk image — build recipe (optional, currently not used)

**Status: skipped.** Measured on the stock `UBUNTU22-64-STD` image, full
node setup takes ~4 min (client; server ~1.5 min) — a baked image would
save only ~2 min per instantiation, not worth the per-cluster snapshot
maintenance. The profile therefore boots stock images. This recipe is kept
in case setup ever grows slow again (e.g., heavier dependencies).

The profile ([profile.py](../profile.py) — maintained in the repo, pasted
into the CloudLab profile editor) would boot each node from a per-cluster
disk image with the slow, stable parts of the environment baked in. The
**code is never baked**: the startup command shallow-clones the repo fresh
at instantiation and builds it (~1 min with the warm module cache), so a
code fix never requires re-imaging.

What the image adds over stock `UBUNTU22-64-STD`:

- Go toolchain (the version in `lib/common.sh` `GO_TARBALL_VERSION`) at
  `/usr/local/go`
- LaTeX plotting stack: `texlive-latex-extra texlive-fonts-recommended
  cm-super dvipng ghostscript`
- Python: `matplotlib pandas numpy` (pip)
- A warm Go module cache for this repo (`~/go/pkg/mod`)

Without the image everything still works — the startup script installs all
of the above — but instantiation takes ~5–10 min of apt/pip instead of ~2 min.

## Building (once per cluster: Clemson + Utah)

1. Instantiate this profile (any node pair on the target cluster) with the
   stock image, or take any Ubuntu 22.04 node and clone the repo.
2. Let the startup finish (Startup column = Finished), or run
   `./DecentralizedLedgerScripts/install_smaran.sh` by hand.
3. Warm the module cache if it wasn't already: `cd /local/repository && make build`.
4. Clean anything instance-specific — the repo clone itself is fine to leave
   (CloudLab re-clones over it), but remove experiment state:
   `rm -rf /local/cluster.env /local/setup.status /local/setup.log
   /data/local/* results/` and any `~/.ssh/id_cloudlab*` (regenerated per
   experiment).
5. On the experiment page: node context menu → **Create Image**. Name it
   `smaran-artifact` under project `distopialabs-PG0`; take it from the
   **client-type** node (images are hardware-portable within a cluster).
6. Note the produced image URN and put it in `IMAGE` in
   [profile.py](../profile.py) for that cluster (replacing the `UBUNTU22-64-STD`
   entry and the `TODO(image)` note).
7. Repeat on the other cluster.

## Updating

Only needed when a *dependency* changes (Go version bump, new python/LaTeX
package): repeat the steps above. Code-only changes ship automatically via
the repo clone.
