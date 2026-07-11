# CloudLab Profile for Smaran AE

Two variants of the profile, selectable at instantiate time:

1. **Pre-built image** — recommended for evaluators. Everything (Smaran,
   OPTIKS, CONIKS) is already installed and the source tree lives at
   `~/Smaran`.
2. **Clean image** — Ubuntu 22.04. The evaluator runs the installers from
   [`../KeyTransparencyScripts/`](../KeyTransparencyScripts/).

The pre-built image URN is recorded in [`image_urn.txt`](image_urn.txt).

## Instantiate the pre-built image (Path A in `../README-AE.md`)

1. Log in to <https://www.cloudlab.us>.
2. Go to **Experiments → Create Experiment Profile** and upload
   [`profile.py`](profile.py). Save as e.g. `smaran-ae`.
3. Click **Instantiate**. On the parameter screen:
   - `Boot from pre-built image` — **checked**
   - `Disk image URN` — paste the value from
     [`image_urn.txt`](image_urn.txt) (if it still says PLACEHOLDER,
     the maintainer has not yet snapshotted a working install; use the
     source-install variant instead).
   - `Hardware types` — leave defaults (`r6615` for node0, `c6420` for node1).
4. Wait for the experiment status to become **Ready**.
5. SSH into node0. Then:
   ```
   cd ~/Smaran/KeyTransparencyScripts
   ./run_fig4a.sh
   ```

## Build the pre-built image (maintainer only)

Do this once from a fresh instantiation of the source-install variant:

```
# On node0:
cd ~/Smaran
./KeyTransparencyScripts/install_coniks.sh
./KeyTransparencyScripts/install_optiks.sh
./KeyTransparencyScripts/install_smaran.sh
# On node1: same three commands.
```

Then, in the CloudLab web UI:

1. Open the experiment → **List View** → node0.
2. Click **Create Disk Image**.
3. Name it e.g. `smaran-ae-v1`.
4. Wait for the snapshot to complete (~10-15 min).
5. Copy the resulting URN and paste it into
   [`image_urn.txt`](image_urn.txt). Commit + push.
6. Repeat for node1 (or use the same image for both — the binaries are
   identical, only the invocation differs).

## Instantiate the clean-image variant (Path B in `../README-AE.md`)

Uncheck `Boot from pre-built image` on the parameter screen. The nodes will
boot Ubuntu 22.04 and the profile's bootstrap script clones the repo into
`~/Smaran` on both. Continue with the installers under
`~/Smaran/KeyTransparencyScripts/`.
