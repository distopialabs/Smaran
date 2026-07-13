# Key-Transparency Artifact-Evaluation Scripts

These scripts reproduce **Figures 4a, 4b, 4c, and 5** of the Smaran paper.
Each script is self-contained: it prepares the cluster, runs one
sweep across `{Smaran, Optiks, Coniks}`, and produces a single PDF.

## Two ways to run

1. **Use the CloudLab profile** (fastest): instantiate
   [the unified profile](https://www.cloudlab.us/p/distopialabs-PG0/smaran-artifact)
   (see the [root README](../README.md)). Smaran is
   built at boot; Coniks/Optiks install on first use. Skip to *Running the
   experiments* below.
2. **Install from source** on any two Ubuntu 22.04 machines that can SSH each
   other: run the three installers first (see *Installation* below).

## Cluster prerequisites

Two Linux nodes reachable over SSH:

| Role   | Recommended CloudLab hardware | Purpose               |
|--------|------------------------------|-----------------------|
| server | `r6615` (Clemson)            | runs `ktserver`       |
| client | `c6420` (Clemson)            | runs `ktbench`        |

Both must be able to SSH into each other (or, at minimum, the machine
that invokes the scripts must be able to SSH into both).

Copy `nodes.env.template` to `nodes.env` and edit the values:

```
cp nodes.env.template nodes.env
$EDITOR nodes.env
```

## Installation (source path)

Run these on **both** nodes:

```
./install_coniks.sh    # prints "Installing Coniks"
./install_optiks.sh    # prints "Installing Optiks"
./install_smaran.sh    # prints "Installing Smaran"
```

All three installers are idempotent: they detect existing Go/apt packages and
skip the reinstall.

## Running the experiments

From either node (or a laptop that can SSH to both):

```
./run_fig4a.sh       # Figure 4a: latency vs versions
./run_fig4b.sh       # Figure 4b: throughput vs versions
./run_fig4c.sh       # Figure 4c: payload vs versions
./run_fig5.sh        # Figure 5:  put throughput vs users
```

Each script prints:

- `Running experiment Figure <yy>`: banner on first line.
- `Running <Protocol> with <x> versions` (or `users` for Fig 5): per data point.
- `Plotting`: followed by the resulting PDF.

Output PDFs land in `<repo>/output/` by default. Override with
`KT_OUTPUT_DIR=/some/dir ./run_fig4a.sh`.

### Sweep sharing across 4a/4b/4c

Figures 4a, 4b, and 4c come from the same experimental sweep. If you run
`./run_fig4a.sh` first, `./run_fig4b.sh` and `./run_fig4c.sh` reuse the sweep
and only re-plot (30 seconds). Set `KT_FORCE_RERUN=1` to force a fresh sweep.

## Quick-turnaround variants

See [`../QuickTesting-KeyTransparency/`](../QuickTesting-KeyTransparency/) for
scripts that run a reduced sweep: enough to see the trend without waiting
for the full sweep to complete.

## Time estimates

See the Key Transparency section of the [root README](../README.md) for a
table of human + compute time per experiment.
