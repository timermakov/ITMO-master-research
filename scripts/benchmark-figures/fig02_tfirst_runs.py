"""Рис. 2 — разброс T_first по независимым run (filtered box + outliers)."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import SCENARIO_ORDER, load_full_results, us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, new_figure, save_figure, write_caption


def _split_filtered_runs(row: dict) -> tuple[list[float], list[tuple[int, float]]]:
    values = us(row["valuesNs"])
    outlier_indexes = set(row.get("summary", {}).get("outlierRunIndexes", []))
    filtered: list[float] = []
    outliers: list[tuple[int, float]] = []
    for run_index, value in enumerate(values, start=1):
        if run_index in outlier_indexes:
            outliers.append((run_index, value))
        else:
            filtered.append(value)
    return filtered, outliers


def plot(out_dir: Path, full_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_full_results(full_path)
    manifest = payload["manifest"]

    filtered_data: list[list[float]] = []
    outlier_data: list[list[tuple[int, float]]] = []
    labels: list[str] = []
    colors: list[str] = []
    color_map = {"s0-control": COLORS["s0"], "s-ref": COLORS["s_ref"], "s-dw": COLORS["s_dw"]}

    for name in SCENARIO_ORDER:
        row = next(r for r in payload["results"] if r["scenario"] == name)
        filtered, outliers = _split_filtered_runs(row)
        filtered_data.append(filtered)
        outlier_data.append(outliers)
        labels.append(SCENARIO_LABELS[name])
        colors.append(color_map[name])

    fig, ax = new_figure(width=170 / 25.4, height=70 / 25.4, ncols=2)
    positions = np.arange(1, len(filtered_data) + 1)

    bp = ax.boxplot(
        filtered_data,
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
    total_outliers = 0
    for i, (vals, outliers, color) in enumerate(zip(filtered_data, outlier_data, colors, strict=True), start=1):
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
            label="run после фильтрации" if i == 1 else None,
        )
        if not outliers:
            continue
        total_outliers += len(outliers)
        outlier_jitter = rng.uniform(-0.08, 0.08, size=len(outliers))
        outlier_values = [value for _, value in outliers]
        ax.scatter(
            i + outlier_jitter,
            outlier_values,
            s=30,
            marker="x",
            color="black",
            linewidths=0.8,
            zorder=4,
            label="выбросы IQR" if total_outliers == len(outliers) else None,
        )
        for x, (run_index, value) in zip(i + outlier_jitter, outliers, strict=True):
            ax.annotate(
                f"#{run_index}",
                (x, value),
                xytext=(3, 3),
                textcoords="offset points",
                fontsize=6,
                color="black",
            )

    ax.set_xticks(positions)
    ax.set_xticklabels(labels)
    ax.set_ylabel(r"$T_{\mathrm{first}}$, мкс")
    ax.set_title("Разброс измерений по независимым run")
    if total_outliers > 0:
        ax.legend(loc="upper right")

    stem = "fig02_tfirst_runs"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 2 — Распределение $T_{{\\mathrm{{first}}}}$ по {manifest['runs']} "
        f"независимым run для каждого сценария. Ящик, усы и цветные точки построены "
        f"по run после IQR-фильтрации (1,5×IQR), как в сводной статистике; "
        f"чёрные кресты — исключённые выбросы с номером run.",
    )
