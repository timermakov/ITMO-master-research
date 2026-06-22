"""Рис. 1 — сравнение P50 метрики T_first по сценариям (лог. шкала)."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import SCENARIO_ORDER, load_full_results, summary_us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, new_figure, save_figure, write_caption


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

    fig, ax = new_figure(ncols=2, height=95 / 25.4)
    x = np.arange(len(labels))
    bars = ax.bar(
        x,
        p50,
        color=colors,
        edgecolor="black",
        linewidth=0.6,
        width=0.55,
        zorder=3,
    )
    ax.errorbar(
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

    ax.set_yscale("log")
    ax.set_xticks(x)
    ax.set_xticklabels(labels)
    ax.set_ylabel(r"$T_{\mathrm{first}}$, P50, мкс")
    ax.set_title("Первый боевой запрос после прогрева")

    for bar, val in zip(bars, p50, strict=True):
        ax.text(
            bar.get_x() + bar.get_width() / 2,
            val * 1.15,
            f"{val:.1f}",
            ha="center",
            va="bottom",
            fontsize=7,
        )
    if len(p50) == 3 and p50[2] > 0:
        speedup = p50[0] / p50[2]
        ax.annotate(
            f"динамический прогрев\nбыстрее холодного старта в {speedup:.0f}x",
            xy=(2, p50[2]),
            xytext=(1.15, p50[0] / 3),
            arrowprops={"arrowstyle": "->", "linewidth": 0.8, "color": COLORS["threshold"]},
            fontsize=7,
            ha="left",
            va="center",
        )

    stem = "fig01_tfirst_p50"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 1 — Медиана (P50) первичной метрики $T_{{\\mathrm{{first}}}}$ "
        f"для трёх сценариев: без прогрева, ручной прогрев и динамический прогрев СДПС "
        f"(N={manifest['runs']}, seed={manifest['seed']}, samples={manifest['samples']}). "
        f"Столбцы — P50; вертикальные отрезки — 95\\% bootstrap ДИ для P50. "
        f"Логарифмическая шкала нужна из-за разницы порядков между холодным стартом и прогретыми сценариями.",
    )
