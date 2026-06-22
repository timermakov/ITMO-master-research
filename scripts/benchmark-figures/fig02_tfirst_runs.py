"""Рис. 2 — разброс T_first по независимым повторам после фильтрации."""

from __future__ import annotations

from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np

from common.data import SCENARIO_ORDER, load_full_results, us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, save_figure, write_caption


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


def _draw_distribution(
    ax,
    data: list[list[float]],
    labels: list[str],
    colors: list[str],
    seed: int,
    *,
    show_legend: bool,
) -> None:
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

    rng = np.random.default_rng(seed)
    for i, (vals, color) in enumerate(zip(data, colors, strict=True), start=1):
        jitter = rng.uniform(-0.08, 0.08, size=len(vals))
        ax.scatter(
            i + jitter,
            vals,
            s=20,
            color=color,
            edgecolors="black",
            linewidths=0.35,
            alpha=0.9,
            zorder=3,
            label="измерения после фильтрации" if show_legend and i == 1 else None,
        )

    ax.set_xticks(positions)
    ax.set_xticklabels(labels)


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

    fig, (ax_all, ax_zoom) = plt.subplots(
        1,
        2,
        figsize=(200 / 25.4, 82 / 25.4),
        width_ratios=[1.25, 1.0],
        constrained_layout=True,
    )
    _draw_distribution(
        ax_all,
        filtered_data,
        labels,
        colors,
        manifest["seed"],
        show_legend=True,
    )
    ax_all.set_yscale("log")
    ax_all.set_ylabel(r"$T_{\mathrm{first}}$, мкс")
    ax_all.set_title("Все сценарии (логарифмическая шкала)")
    ax_all.legend(loc="upper right")

    warm_data = filtered_data[1:]
    warm_labels = labels[1:]
    warm_colors = colors[1:]
    _draw_distribution(
        ax_zoom,
        warm_data,
        warm_labels,
        warm_colors,
        manifest["seed"] + 1,
        show_legend=False,
    )
    ax_zoom.set_ylabel(r"$T_{\mathrm{first}}$, мкс")
    ax_zoom.set_title("Прогретые сценарии (увеличение)")

    stem = "fig02_tfirst_runs"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 2 — Распределение $T_{{\\mathrm{{first}}}}$ по {manifest['runs']} "
        f"независимым повторам для каждого сценария. Левая панель показывает весь диапазон "
        f"на логарифмической шкале, правая — увеличенный вид прогретых сценариев. "
        f"Ящик, усы и цветные точки построены только по значениям после IQR-фильтрации "
        f"(1,5×IQR), как в сводной статистике; исключённые выбросы на графике не показаны.",
    )
