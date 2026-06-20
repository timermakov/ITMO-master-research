"""Рис. 2 — разброс T_first по независимым run (box + точки)."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import SCENARIO_ORDER, load_full_results, us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, new_figure, save_figure, write_caption


def plot(out_dir: Path, full_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_full_results(full_path)
    manifest = payload["manifest"]

    data: list[list[float]] = []
    labels: list[str] = []
    colors: list[str] = []
    color_map = {"s0-control": COLORS["s0"], "s-ref": COLORS["s_ref"], "s-dw": COLORS["s_dw"]}

    for name in SCENARIO_ORDER:
        row = next(r for r in payload["results"] if r["scenario"] == name)
        data.append(us(row["valuesNs"]))
        labels.append(SCENARIO_LABELS[name])
        colors.append(color_map[name])

    fig, ax = new_figure(width=170 / 25.4, height=70 / 25.4, ncols=2)
    positions = np.arange(1, len(data) + 1)

    bp = ax.boxplot(
        data,
        positions=positions,
        widths=0.35,
        patch_artist=True,
        showfliers=False,
        medianprops={"color": "black", "linewidth": 1.2},
        whiskerprops={"linewidth": 0.8},
        capprops={"linewidth": 0.8},
        boxprops={"linewidth": 0.8},
    )
    for patch, color in zip(bp["boxes"], colors, strict=True):
        patch.set_facecolor(color)
        patch.set_alpha(0.55)

    rng = np.random.default_rng(manifest["seed"])
    for i, (vals, color) in enumerate(zip(data, colors, strict=True), start=1):
        jitter = rng.uniform(-0.08, 0.08, size=len(vals))
        ax.scatter(
            i + jitter,
            vals,
            s=22,
            color=color,
            edgecolors="black",
            linewidths=0.4,
            alpha=0.9,
            zorder=3,
        )

    ax.set_xticks(positions)
    ax.set_xticklabels(labels)
    ax.set_ylabel(r"$T_{\mathrm{first}}$, мкс")
    ax.set_title("Разброс измерений по независимым run")

    stem = "fig02_tfirst_runs"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 2 — Распределение $T_{{\\mathrm{{first}}}}$ по {manifest['runs']} "
        f"независимым run для каждого сценария. Ящик — квартили и медиана; "
        f"точки — отдельные run после фильтрации выбросов (если применялась).",
    )
