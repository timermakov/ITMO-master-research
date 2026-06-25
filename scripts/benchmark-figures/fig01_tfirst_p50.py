"""Рис. 1 — сравнение P50 метрики T_first по сценариям."""

from __future__ import annotations

from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np

from common.data import SCENARIO_ORDER, load_full_results, summary_us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, save_figure, write_caption


def plot(out_dir: Path, full_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_full_results(full_path)
    manifest = payload["manifest"]

    labels: list[str] = []
    p50: list[float] = []
    err_lo: list[float] = []
    err_hi: list[float] = []
    colors: list[str] = []

    color_map = {"s0-control": COLORS["s0"], "s-ref": COLORS["s_ref"], "s-dw": COLORS["s_dw"]}
    for name in SCENARIO_ORDER:
        row = next(r for r in payload["results"] if r["scenario"] == name)
        s = summary_us(row["summary"])
        labels.append(SCENARIO_LABELS[name])
        p50.append(s["p50"])
        err_lo.append(s["p50"] - s["p50_lo"])
        err_hi.append(s["p50_hi"] - s["p50"])
        colors.append(color_map[name])

    fig, (ax_all, ax_warm) = plt.subplots(
        1,
        2,
        figsize=(200 / 25.4, 92 / 25.4),
        width_ratios=[1.25, 1.0],
        constrained_layout=True,
    )
    x = np.arange(len(labels))

    bars = ax_all.bar(
        x,
        p50,
        color=colors,
        edgecolor="black",
        linewidth=0.6,
        width=0.55,
        zorder=3,
    )
    ax_all.errorbar(
        x,
        p50,
        yerr=[err_lo, err_hi],
        fmt="none",
        ecolor="black",
        elinewidth=0.9,
        capsize=4,
        capthick=0.9,
        zorder=4,
    )

    ax_all.set_xticks(x)
    ax_all.set_xticklabels(labels)
    ax_all.set_ylabel(r"$T_{\mathrm{first}}$, P50, мкс")
    ax_all.set_title("Задержка построения hashmap при первом запросе")

    for bar, val in zip(bars, p50, strict=True):
        ax_all.text(
            bar.get_x() + bar.get_width() / 2,
            val + max(p50) * 0.03,
            f"{val:.1f}",
            ha="center",
            va="bottom",
            fontsize=7,
        )
    if len(p50) == 3 and p50[2] > 0:
        speedup = p50[0] / p50[2]
        ax_all.annotate(
            f"динамический прогрев\nбыстрее запуска без прогрева в {speedup:.0f}x",
            xy=(2, p50[2]),
            xytext=(1.05, max(p50) * 0.55),
            arrowprops={"arrowstyle": "->", "linewidth": 0.8, "color": COLORS["threshold"]},
            fontsize=7,
            ha="left",
            va="center",
        )

    warm_idx = np.arange(2)
    warm_values = p50[1:]
    warm_err_lo = err_lo[1:]
    warm_err_hi = err_hi[1:]
    warm_bars = ax_warm.bar(
        warm_idx,
        warm_values,
        color=colors[1:],
        edgecolor="black",
        linewidth=0.6,
        width=0.55,
        zorder=3,
    )
    ax_warm.errorbar(
        warm_idx,
        warm_values,
        yerr=[warm_err_lo, warm_err_hi],
        fmt="none",
        ecolor="black",
        elinewidth=0.9,
        capsize=4,
        capthick=0.9,
        zorder=4,
    )
    ax_warm.set_xticks(warm_idx)
    ax_warm.set_xticklabels(labels[1:])
    ax_warm.set_ylabel(r"$T_{\mathrm{first}}$, P50, мкс")
    ax_warm.set_title("Сравнение прогретых сценариев")
    for bar, val in zip(warm_bars, warm_values, strict=True):
        ax_warm.text(
            bar.get_x() + bar.get_width() / 2,
            val + max(warm_values) * 0.06,
            f"{val:.1f}",
            ha="center",
            va="bottom",
            fontsize=7,
        )

    stem = "fig01_tfirst_p50"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 1 — Медиана (P50) метрики $T_{{\\mathrm{{first}}}}$: время построения "
        f"in-memory hashmap и lookup при первом GET /work после reset "
        f"для трёх сценариев: без прогрева, ручной прогрев и динамический прогрев СДПС "
        f"(N={manifest['runs']}, seed={manifest['seed']}, samples={manifest['samples']}). "
        f"Столбцы — P50; вертикальные отрезки — 95\\% доверительный интервал для P50. "
        f"Правая панель отдельно сравнивает ручной и динамический прогрев на обычной шкале.",
    )
