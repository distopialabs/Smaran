#!/usr/bin/env python3
"""
plot_bench_all_users.py — Benchmark comparison across protocols AND multiple user counts.

Automatically finds the newest ingestion CSV for each requested protocol
from /data/local/benchmark_output/<protocol>/, for each requested user count.
Produces grouped bar-chart comparisons (one group per user count, one bar per protocol).
"""

import argparse
import glob
import os
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

# ---------------------------------------------------------------------------
# Protocol style constants (same as plot_bench.py)
# ---------------------------------------------------------------------------

# Okabe-Ito colorblind-safe palette
PROTOCOL_STYLE = {
    "samurai":    {"color": "#0072B2", "label": "Smaran (w/o MPT)",      "marker": "o"},
    "samuraimpt": {"color": "#CC79A7", "label": "Smaran",  "marker": "s"},
    "merkle":     {"color": "#D55E00", "label": "MPT",       "marker": "^"},
    "verkle":     {"color": "#009E73", "label": "Verkle",       "marker": "D"},
}
_AUTO_COLORS  = ["#E69F00", "#CC79A7", "#F0E442", "#000000", "#999999"]
_AUTO_MARKERS = ["P", "X", "v", "p", "h"]
_auto_color_idx = 0

BENCH_ROOT = "logs/ingestion_logs"


def _protocol_style(name: str) -> dict:
    global _auto_color_idx
    if name in PROTOCOL_STYLE:
        return PROTOCOL_STYLE[name]
    idx = _auto_color_idx % len(_AUTO_COLORS)
    _auto_color_idx += 1
    return {"color": _AUTO_COLORS[idx], "label": name.capitalize(), "marker": _AUTO_MARKERS[idx]}


# ---------------------------------------------------------------------------
# Matplotlib style
# ---------------------------------------------------------------------------

def apply_paper_style():
    plt.rcParams.update({
        "text.usetex":        True,
        "text.latex.preamble": r"\usepackage{amsmath}\usepackage{times}",
        "font.family":        "serif",
        "font.size":          70,
        "axes.titlesize":     70,
        "axes.labelsize":     70,
        "xtick.labelsize":    65,
        "ytick.labelsize":    70,
        "legend.fontsize":    70,
        "axes.spines.top":    False,
        "axes.spines.right":  False,
        "axes.grid":          False,
        "figure.dpi":         150,
    })


# ---------------------------------------------------------------------------
# Figure helpers
# ---------------------------------------------------------------------------

def save_figure(fig, output_dir: str, name: str, fmt: str, dpi: int):
    os.makedirs(output_dir, exist_ok=True)
    path = os.path.join(output_dir, f"{name}.{fmt}")
    fig.savefig(path, format=fmt, dpi=dpi, bbox_inches="tight")
    print(f"  saved {path}")
    plt.close(fig)


# ---------------------------------------------------------------------------
# CSV discovery
# ---------------------------------------------------------------------------

def discover_user_counts(protocols: list, skip_users: set) -> list:
    """Discover all user counts present across all protocol directories."""
    user_counts = set()
    for protocol in protocols:
        proto_dir = os.path.join(BENCH_ROOT, protocol)
        if not os.path.isdir(proto_dir):
            continue
        for path in glob.glob(os.path.join(proto_dir, "ingestion_*.csv")):
            basename = os.path.basename(path)
            # basename looks like ingestion_<count><rest>.csv
            rest = basename[len("ingestion_"):]
            # extract leading digits as the user count
            digits = ""
            for ch in rest:
                if ch.isdigit():
                    digits += ch
                else:
                    break
            if digits:
                uc = int(digits)
                if uc not in skip_users:
                    user_counts.add(uc)
    return sorted(user_counts)


def find_newest_ingestion_csv(protocol: str, user_count: int) -> str | None:
    """Find the newest ingestion CSV for a protocol/user-count pair, or None if missing."""
    proto_dir = os.path.join(BENCH_ROOT, protocol)
    if not os.path.isdir(proto_dir):
        print(f"  WARNING: protocol directory not found: {proto_dir}")
        return None

    pattern = os.path.join(proto_dir, f"ingestion_{user_count}.csv")
    matches = sorted(glob.glob(pattern), key=os.path.getmtime, reverse=True)
    print(user_count, matches)

    if not matches:
        print(f"  WARNING: no ingestion CSV for protocol={protocol}, users={user_count} — skipping")
        return None

    chosen = matches[0]
    print(f"  {protocol} ({user_count} users): using {chosen}")
    return chosen


# ---------------------------------------------------------------------------
# Ingestion data utilities (same as plot_bench.py)
# ---------------------------------------------------------------------------

INGESTION_REQUIRED_COLS = {
    "block_num", "num_selected_updates",
    "queued_at_ns", "start_at_ns", "completed_at_ns",
}

# Cauchy logs have an extra leading column not present in other protocols.
CAUCHY_EXTRA_COLS = {"Tracked_Accounts"}


def load_ingestion_csv(path: str, protocol: str = "") -> pd.DataFrame:
    df = pd.read_csv(path)
    df.columns = [c.strip() for c in df.columns]

    if protocol == "cauchy":
        df = df.drop(columns=[c for c in CAUCHY_EXTRA_COLS if c in df.columns])

    missing = INGESTION_REQUIRED_COLS - set(df.columns)
    if missing:
        sys.exit(f"ERROR: {path} is missing columns: {missing}")
    return df


def trim_df(df: pd.DataFrame, warmup: float, cooldown: float) -> pd.DataFrame:
    rel = (df["queued_at_ns"] - df["queued_at_ns"].min()) / 1e9
    df = df.copy()
    df["rel_time"] = rel
    max_t = rel.max()
    mask = (rel >= warmup) & (rel <= (max_t - cooldown))
    return df[mask].reset_index(drop=True)


def add_per_update_latency_cols(df: pd.DataFrame) -> pd.DataFrame:
    df = df.copy()
    df["block_lat_ms"]      = (df["completed_at_ns"] - df["start_at_ns"])  / 1e6
    df["e2e_block_lat_ms"]  = (df["completed_at_ns"] - df["queued_at_ns"]) / 1e6
    nu = df["num_selected_updates"].replace(0, np.nan)
    df["update_lat_ms"]     = df["block_lat_ms"]     / nu
    df["e2e_update_lat_ms"] = df["e2e_block_lat_ms"] / nu
    return df


def compute_scalars(path: str, warmup: float, cooldown: float, protocol: str = "") -> dict:
    df = load_ingestion_csv(path, protocol)
    df = trim_df(df, warmup, cooldown)
    df = add_per_update_latency_cols(df)
    valid = df[df["num_selected_updates"] > 0]

    avg_update_lat     = valid["update_lat_ms"].mean()
    avg_e2e_update_lat = valid["e2e_update_lat_ms"].mean()

    total_updates = df["num_selected_updates"].sum()
    wall_time_s   = (df["completed_at_ns"].max() - df["queued_at_ns"].min()) / 1e9
    avg_throughput = total_updates / wall_time_s if wall_time_s > 0 else 0.0

    return {
        "avg_update_lat":     avg_update_lat,
        "avg_e2e_update_lat": avg_e2e_update_lat,
        "avg_throughput":     avg_throughput,
    }


# ---------------------------------------------------------------------------
# Plotting
# ---------------------------------------------------------------------------

# (col, ylabel, fname, title, scale)
# scale is applied to raw values before plotting
GRAPHS = [
    ("avg_update_lat",     "Avg Update Latency (ms)",     "all_users_update_latency",     "Avg Update Latency",     1.0),
    ("avg_e2e_update_lat", "Avg E2E Update Latency (ms)", "all_users_e2e_update_latency", "Avg E2E Update Latency", 1.0),
    ("avg_throughput",     "Throughput (ops/s)",         "all_users_throughput",         "Avg Update Throughput",  1.0),
]


def _user_count_formatter(x, pos):
    """Format x-axis tick labels: integers < 100000 as plain integers, >= 100000 as e.g. '0.1M', '1M'."""
    x = int(x)
    if x >= 100_000:
        m = x / 1_000_000
        if m < 1:
            return f"0.{int(m * 10)}M"
        return f"{m:g}M"
    return str(x)


def _y_formatter_kilo(x, pos):
    """No scientific notation on either panel."""
    if x >= 1000:
        return f"{int(x / 1000)}k"
    if x == 0:
        return "0"
    # Fixed-point for small values — strip trailing zeros
    return f"{x:.10f}".rstrip("0").rstrip(".")


def _y_formatter_normal(x, pos):
    """Scientific notation on either panel."""
    if x == 0:
        return "0"
    # exp = int(np.floor(np.log10(abs(x))))
    # mant = x / 10**exp
    # return rf"$\mathrm{{{mant:.0f} \cdot 10^{{{exp}}}}}$"
    # Fixed-point for small values — strip trailing zeros
    return f"{x:.10f}".rstrip("0").rstrip(".")


def plot_line_charts(
    data: dict,          # data[user_count][protocol] = scalars_dict
    protocols: list,     # ordered list of protocol names
    user_counts: list,   # ordered list of user counts (x-axis)
    proto_styles: dict,  # precomputed styles keyed by protocol name
    args,
):
    """Produces one line chart per metric. X-axis = number of users, one line per protocol."""
    small_ucs   = [uc for uc in user_counts if uc < 100_000]
    large_ucs   = [uc for uc in user_counts if uc >= 100_000]
    use_x_broken = bool(small_ucs and large_ucs)
    Y_BREAK = 10000

    # --- helpers ---

    def _plot_proto_on_ax(ax, ucs, y_normal: bool = False):
        for proto in protocols:
            sty = proto_styles[proto]
            proto_ucs = [uc for uc in ucs if proto in data[uc]]
            if not proto_ucs:
                continue
            values = [data[uc][proto][col] * scale for uc in proto_ucs]
            ax.plot(
                proto_ucs, values,
                color=sty["color"],
                label=sty["label"],
                marker=sty.get("marker", "o"),
                markersize=25,
                linewidth=10,
                markeredgewidth=2,
            )
        ax.set_xscale("log")
        ax.set_xticks(ucs)
        ax.xaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_user_count_formatter))
        if y_normal:
            ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_y_formatter_normal))
        else:
            ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_y_formatter_kilo))
        ax.grid(True, which="both", linestyle="--", linewidth=3, alpha=0.7)

    _X_BRK_MARKER = [(-1, -1), (1, 1)]
    _Y_BRK_MARKER = [(-1, -1), (1, 1)]
    _BRK_KW = dict(markersize=20, color="k", mec="k", mew=3, clip_on=False, linestyle="none")

    def _add_x_break(ax_l, ax_r):
        ax_l.plot([1], [0], marker=_X_BRK_MARKER, transform=ax_l.transAxes, **_BRK_KW)
        ax_r.plot([0], [0], marker=_X_BRK_MARKER, transform=ax_r.transAxes, **_BRK_KW)

    def _add_y_break(ax_top, ax_bot):
        ax_top.plot([0], [0], marker=_Y_BRK_MARKER, transform=ax_top.transAxes, **_BRK_KW)
        ax_bot.plot([0], [1], marker=_Y_BRK_MARKER, transform=ax_bot.transAxes, **_BRK_KW)

    def _collect_handles(*axes_list):
        seen, handles, labels = set(), [], []
        for ax in axes_list:
            for h, l in zip(*ax.get_legend_handles_labels()):
                if l not in seen:
                    handles.append(h); labels.append(l); seen.add(l)
        return handles, labels

    def _add_legend(fig, handles, labels):
        fig.legend(
            handles, labels,
            loc="upper center", bbox_to_anchor=(0.5, 1.04),
            ncol=len(handles), frameon=True, edgecolor="black",
            fontsize=plt.rcParams["legend.fontsize"]*.75,
            columnspacing=0.3,
        )

    # --- per-metric loop ---

    for col, ylabel, fname, title, scale in GRAPHS:
        all_vals = [
            data[uc][proto][col] * scale
            for uc in user_counts for proto in protocols
            if proto in data[uc]
        ]
        use_y_broken = bool(all_vals) and min(all_vals) <= Y_BREAK

        if use_y_broken:
            ymin_d, ymax_d = min(all_vals), max(all_vals)
            pad = (ymax_d - ymin_d) * 0.08 or 1e-5
            # Ensure top panel range is never inverted (data may all be below Y_BREAK)
            top_upper = max(ymax_d + pad, Y_BREAK * 1.5)
            y_top_lim = (Y_BREAK, top_upper)
            y_bot_lim = (max(ymin_d - pad, 0), 0.6)
            h_ratio = [1, 1]

        if use_x_broken and use_y_broken:
            n_s, n_l = max(len(small_ucs), 1), max(len(large_ucs), 1)
            fig, axes = plt.subplots(
                2, 2, figsize=(30, 12),
                sharex="col", sharey="row",
                gridspec_kw={"width_ratios": [n_s, n_l], "height_ratios": h_ratio},
            )
            fig.subplots_adjust(wspace=0.1, hspace=0.08)
            ax_tl, ax_tr = axes[0, 0], axes[0, 1]
            ax_bl, ax_br = axes[1, 0], axes[1, 1]

            for ax in [ax_tl, ax_tr, ax_bl, ax_br]:
                ax.spines["left"].set_linewidth(5)
                ax.spines["bottom"].set_linewidth(5)

            for i, ax in enumerate([ax_tl, ax_bl]): _plot_proto_on_ax(ax, small_ucs, y_normal=(i == 1))
            for i, ax in enumerate([ax_tr, ax_br]): _plot_proto_on_ax(ax, large_ucs, y_normal=(i == 1))

            # x-break spines
            for ax in [ax_tl, ax_bl]: ax.spines["right"].set_visible(False)
            for ax in [ax_tr, ax_br]:
                ax.spines["left"].set_visible(False)
                ax.tick_params(left=False)
            # y-break spines
            for ax in [ax_tl, ax_tr]:
                ax.spines["bottom"].set_visible(False)
                ax.tick_params(bottom=False)
            for ax in [ax_bl, ax_br]: ax.spines["top"].set_visible(False)

            _add_x_break(ax_bl, ax_br)   # bottom row only — top row marks land in the middle
            _add_y_break(ax_tl, ax_bl)   # left column only — right column marks land in the middle

            # Set limits last so nothing above overrides them
            for ax in [ax_tl, ax_tr]: ax.set_ylim(*y_top_lim)
            for ax in [ax_bl, ax_br]: ax.set_ylim(*y_bot_lim)

            for ax in [ax_bl, ax_br]: ax.set_yticks([0, 0.2, 0.4, 0.6])

            fig.supylabel(ylabel, position=(0.04, 0.5))
            fig.text(0.5, -0.04, "Number of users", ha="center",
                     fontsize=plt.rcParams["axes.labelsize"])
            handles, labels = _collect_handles(ax_tl, ax_tr, ax_bl, ax_br)
            _add_legend(fig, handles, labels)

        elif use_x_broken:
            n_s, n_l = max(len(small_ucs), 1), max(len(large_ucs), 1)
            fig, (ax_l, ax_r) = plt.subplots(
                1, 2, sharey=True, figsize=(30, 12),
                gridspec_kw={"width_ratios": [n_s, n_l]},
            )
            fig.subplots_adjust(wspace=0.08)

            _plot_proto_on_ax(ax_l, small_ucs)
            _plot_proto_on_ax(ax_r, large_ucs)

            ax_l.spines["right"].set_visible(False)
            ax_r.spines["left"].set_visible(False)
            ax_r.tick_params(left=False)
            _add_x_break(ax_l, ax_r)

            ax_l.set_ylabel(ylabel)
            fig.text(0.5, -0.04, "Number of users", ha="center",
                     fontsize=plt.rcParams["axes.labelsize"])
            handles, labels = _collect_handles(ax_l, ax_r)
            _add_legend(fig, handles, labels)

        elif use_y_broken:
            fig, (ax_top, ax_bot) = plt.subplots(
                2, 1, sharex=True, figsize=(30, 12),
                gridspec_kw={"height_ratios": h_ratio},
            )
            fig.subplots_adjust(hspace=0.08)

            _plot_proto_on_ax(ax_top, user_counts)
            _plot_proto_on_ax(ax_bot, user_counts)

            ax_top.spines["bottom"].set_visible(False)
            ax_bot.spines["top"].set_visible(False)
            ax_top.tick_params(bottom=False)
            _add_y_break(ax_top, ax_bot)

            # Set limits last so nothing above overrides them
            ax_top.set_ylim(*y_top_lim)
            ax_bot.set_ylim(*y_bot_lim)

            ax_top.set_ylabel(ylabel)
            ax_bot.set_xlabel("Number of users")
            handles, labels = _collect_handles(ax_top, ax_bot)
            _add_legend(fig, handles, labels)

        else:
            fig, ax = plt.subplots(1, 1, figsize=(30, 12))
            _plot_proto_on_ax(ax, user_counts)
            ax.set_xlabel("Number of users")
            ax.set_ylabel(ylabel)
            handles, labels = _collect_handles(ax)
            _add_legend(fig, handles, labels)

        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# TikZ / pgfplots output
# ---------------------------------------------------------------------------

# Maps matplotlib marker codes → pgfplots mark names
_MPL_TO_PGF_MARK = {
    "o": "*",
    "s": "square*",
    "^": "triangle*",
    "D": "diamond*",
    "P": "pentagon*",
    "X": "x",
    "v": "triangle*",   # + rotate=180 below
    "p": "pentagon*",
    "h": "halfdiamond*",
}
_ROTATE_180_MARKS = {"v"}


def generate_tikz(
    data: dict,
    protocols: list,
    user_counts: list,
    proto_styles: dict,  # precomputed, avoids re-running auto-color logic
    args,
):
    """Generate one .tex file per metric with a pgfplots tikzpicture and embedded data."""

    small_ucs = [uc for uc in user_counts if uc < 100_000]
    large_ucs = [uc for uc in user_counts if uc >= 100_000]
    use_broken = bool(small_ucs and large_ucs)

    def fmt_label(uc):
        if uc >= 100_000:
            m = uc / 1_000_000
            return f"{m:g}M"
        return str(uc)

    def axis_range(ucs):
        """Return (xmin, xmax) with half-interval padding on each side."""
        if not ucs:
            return 0, 1
        if len(ucs) == 1:
            return ucs[0] * 0.5, ucs[0] * 1.5
        return (ucs[0]  - (ucs[1]  - ucs[0])  * 0.5,
                ucs[-1] + (ucs[-1] - ucs[-2]) * 0.5)

    def color_name(proto):
        return f"clr{proto.replace('+', 'p').replace('-', 'm')}"

    def addplot_block(proto, ucs, col, scale, add_legend):
        sty     = proto_styles[proto]
        mpl_mk  = sty.get("marker", "o")
        pgf_mk  = _MPL_TO_PGF_MARK.get(mpl_mk, "*")
        mk_opts = ", mark options={rotate=180}" if mpl_mk in _ROTATE_180_MARKS else ""
        cname   = color_name(proto)

        rows = [(uc, data[uc][proto][col] * scale)
                for uc in ucs if proto in data[uc]]
        if not rows:
            return []

        lines = [
            f"\\addplot[color={cname}, mark={pgf_mk}{mk_opts},"
            f" line width=1.5pt, mark size=2pt]",
            f"  table[x=u, y=v, col sep=comma] {{",
            "  u,v",
        ]
        lines += [f"  {uc},{v:.6f}" for uc, v in rows]
        lines += ["  };"]
        if add_legend:
            lines.append(f"\\addlegendentry{{{sty['label']}}}")
        return lines

    os.makedirs(args.output_dir, exist_ok=True)

    for col, ylabel, fname, title, scale in GRAPHS:
        out = [
            "% Auto-generated by plot_bench_all_users.py",
            "% Required packages: pgfplots, pgfplotscompat, xcolor",
            "%   \\usepackage{pgfplots}",
            "%   \\usepgfplotslibrary{groupplot}",
            "\\pgfplotsset{compat=1.18}",
            "",
        ]

        # Color definitions
        for proto in protocols:
            hex_col = proto_styles[proto]["color"].lstrip("#")
            out.append(f"\\definecolor{{{color_name(proto)}}}{{HTML}}{{{hex_col}}}")
        out.append("")

        # Compute shared y range so both broken-axis panels have identical y scale
        all_vals = [
            data[uc][proto][col] * scale
            for uc in user_counts
            for proto in protocols
            if proto in data[uc]
        ]
        if all_vals:
            span   = max(all_vals) - min(all_vals) or 1.0
            ymin_p = min(all_vals) - span * 0.12
            ymax_p = max(all_vals) + span * 0.12
            yrange_opts = f"  ymin={ymin_p:.6f}, ymax={ymax_p:.6f},"
        else:
            yrange_opts = ""

        out.append("\\begin{tikzpicture}")

        if use_broken:
            n_s = max(len(small_ucs), 1)
            n_l = max(len(large_ucs), 1)
            tot = n_s + n_l
            lw  = f"{n_s / tot * 0.68:.2f}\\linewidth"
            rw  = f"{n_l / tot * 0.68:.2f}\\linewidth"

            xmns, xmxs = axis_range(small_ucs)
            xmnl, xmxl = axis_range(large_ucs)

            s_ticks  = ",".join(str(u) for u in small_ucs)
            l_ticks  = ",".join(str(u) for u in large_ucs)
            s_labels = ",".join(fmt_label(u) for u in small_ucs)
            l_labels = ",".join(fmt_label(u) for u in large_ucs)

            out += [
                "\\begin{groupplot}[",
                "  group style={group size=2 by 1, horizontal sep=2pt},",
                "  height=5cm,",
                "  axis x line*=bottom,",
                "]",
                "",
                "% --- Left panel: small user counts ---",
                "\\nextgroupplot[",
                f"  width={lw},",
                f"  xmin={xmns:.2f}, xmax={xmxs:.2f},",
                f"  xtick={{{s_ticks}}},",
                f"  xticklabels={{{s_labels}}},",
                f"  xlabel={{Number of users}},",
                f"  ylabel={{{ylabel}}},",
                f"  title={{{title}}},",
                "  axis y line*=left,",
                yrange_opts,
                "  legend style={at={(0.05,0.95)}, anchor=north west, draw=none, font=\\small},",
                "]",
            ]
            for proto in protocols:
                out += addplot_block(proto, small_ucs, col, scale, add_legend=True)

            out += [
                "",
                "% --- Right panel: large user counts ---",
                "\\nextgroupplot[",
                f"  width={rw},",
                f"  xmin={xmnl:.2f}, xmax={xmxl:.2f},",
                f"  xtick={{{l_ticks}}},",
                f"  xticklabels={{{l_labels}}},",
                f"  xlabel={{Number of users}},",
                "  ytick=\\empty,",
                "  yticklabels=\\empty,",
                "  axis y line=none,",
                yrange_opts,
                "]",
            ]
            for proto in protocols:
                out += addplot_block(proto, large_ucs, col, scale, add_legend=False)

            out += [
                "",
                "\\end{groupplot}",
                "",
                "% Axis-break diagonal markers (/ slashes at the panel boundary)",
                "\\draw ([xshift=-3pt,yshift=-3pt]group c1r1.south east) --",
                "      ([xshift= 3pt,yshift= 3pt]group c1r1.south east);",
                "\\draw ([xshift=-3pt,yshift=-3pt]group c1r1.north east) --",
                "      ([xshift= 3pt,yshift= 3pt]group c1r1.north east);",
                "\\draw ([xshift=-3pt,yshift=-3pt]group c2r1.south west) --",
                "      ([xshift= 3pt,yshift= 3pt]group c2r1.south west);",
                "\\draw ([xshift=-3pt,yshift=-3pt]group c2r1.north west) --",
                "      ([xshift= 3pt,yshift= 3pt]group c2r1.north west);",
            ]
        else:
            xmn, xmx = axis_range(user_counts)
            all_ticks  = ",".join(str(u) for u in user_counts)
            all_labels = ",".join(fmt_label(u) for u in user_counts)

            out += [
                "\\begin{axis}[",
                "  width=0.75\\linewidth, height=5cm,",
                f"  xmin={xmn:.2f}, xmax={xmx:.2f},",
                f"  xtick={{{all_ticks}}},",
                f"  xticklabels={{{all_labels}}},",
                f"  xlabel={{Number of users}},",
                f"  ylabel={{{ylabel}}},",
                f"  title={{{title}}},",
                "  axis x line*=bottom,",
                "  axis y line*=left,",
                yrange_opts,
                "  legend style={draw=none, font=\\small},",
                "]",
            ]
            for proto in protocols:
                out += addplot_block(proto, user_counts, col, scale, add_legend=True)
            out.append("\\end{axis}")

        out += ["", "\\end{tikzpicture}", ""]

        path = os.path.join(args.output_dir, f"{fname}.tex")
        with open(path, "w") as fh:
            fh.write("\n".join(out))
        print(f"  saved {path}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    apply_paper_style()

    parser = argparse.ArgumentParser(
        description="Compare latest ingestion benchmarks across protocols and multiple user counts.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--protocol", action="append", required=True,
                        metavar="NAME",
                        help="Protocol name (repeatable). Must match a "
                             "subdirectory under /data/local/benchmark_output/.")
    parser.add_argument("--skip-users", action="append", type=int, default=[],
                        metavar="N",
                        help="User count to exclude from auto-discovery (repeatable).")
    parser.add_argument("--output-dir", default="benchmark_output/plots",
                        help="Directory for output files (default: benchmark_output/plots)")
    parser.add_argument("--format", choices=["pdf", "png"], default="pdf",
                        help="Output format (default: pdf)")
    parser.add_argument("--dpi", type=int, default=300,
                        help="DPI for PNG output (default: 300)")
    parser.add_argument("--warmup", type=float, default=0.0,
                        help="Seconds to trim from start of data (default: 0)")
    parser.add_argument("--cooldown", type=float, default=0.0,
                        help="Seconds to trim from end of data (default: 0)")

    args = parser.parse_args()

    protocols   = args.protocol
    skip_users  = set(args.skip_users)

    user_counts = discover_user_counts(protocols, skip_users)
    if not user_counts:
        sys.exit("ERROR: no user counts discovered from benchmark directories.")

    if skip_users:
        print(f"Skipping user counts: {sorted(skip_users)}")
    print(f"Discovered user counts: {user_counts}")
    print(f"Finding newest ingestion CSVs for users={user_counts}, protocols={protocols}:")

    # data[user_count][protocol] = scalars_dict  (only populated when data exists)
    data: dict = {}
    for uc in user_counts:
        data[uc] = {}
        for proto in protocols:
            csv_path = find_newest_ingestion_csv(proto, uc)
            if csv_path is None:
                continue
            data[uc][proto] = compute_scalars(csv_path, args.warmup, args.cooldown, proto)

    # Print summary table
    col_w = [8, 15, 18, 18, 18]
    headers = ["Users", "Protocol", "Avg Update Lat (ms)", "Avg E2E Lat (ms)", "Throughput (upd/s)"]
    header_line = "  ".join(f"{h:<{w}}" for h, w in zip(headers, col_w))
    print(f"\n{header_line}")
    print("-" * len(header_line))
    for uc in user_counts:
        for proto in protocols:
            if proto not in data[uc]:
                continue
            s = data[uc][proto]
            row = [
                f"{uc:<{col_w[0]}}",
                f"{proto:<{col_w[1]}}",
                f"{s['avg_update_lat']:<{col_w[2]}.4f}",
                f"{s['avg_e2e_update_lat']:<{col_w[3]}.4f}",
                f"{s['avg_throughput']:<{col_w[4]}.4f}",
            ]
            print("  ".join(row))
        print()

    # Save summary table as CSV
    rows = []
    for uc in user_counts:
        for proto in protocols:
            if proto not in data[uc]:
                continue
            s = data[uc][proto]
            rows.append({
                "Users": uc,
                "Protocol": proto,
                "Avg Update Lat (ms)": round(s["avg_update_lat"], 4),
                "Avg E2E Lat (ms)": round(s["avg_e2e_update_lat"], 4),
                "Throughput (upd/s)": round(s["avg_throughput"], 4),
            })
    os.makedirs(args.output_dir, exist_ok=True)
    csv_path = os.path.join(args.output_dir, "summary.csv")
    pd.DataFrame(rows).to_csv(csv_path, index=False)
    print(f"  saved {csv_path}")

    # Precompute styles once so both the matplotlib and TikZ outputs are consistent
    global _auto_color_idx
    _auto_color_idx = 0
    proto_styles = {p: _protocol_style(p) for p in protocols}

    print("Generating comparison charts:")
    plot_line_charts(data, protocols, user_counts, proto_styles, args)
    print("Generating TikZ/pgfplots charts:")
    generate_tikz(data, protocols, user_counts, proto_styles, args)


if __name__ == "__main__":
    main()
