"""Рис. 4 — коэффициент вариации по сценариям."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import SCENARIO_ORDER, load_full_results, summary_us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, new_figure, save_figure, write_caption

_THRESHOLD_KEYS = {
    "s0-control": "cvThresholdS0",
    "s-ref": "cvThresholdSRef",
    "s-dw": "cvThresholdSDw",
}


def plot(out_dir: Path, full_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_full_results(full_path)
    manifest = payload["manifest"]

    labels: list[str] = []
    cvs: list[float] = []
    thresholds: list[float] = []
    colors: list[str] = []
    color_map = {"s0-control": COLORS["s0"], "s-ref": COLORS["s_ref"], "s-dw": COLORS["s_dw"]}

    for name in SCENARIO_ORDER:
        row = next(r for r in payload["results"] if r["scenario"] == name)
        s = summary_us(row["summary"])
        labels.append(SCENARIO_LABELS[name])
        cvs.append(s["cv"])
        thresholds.append(float(manifest[_THRESHOLD_KEYS[name]]))
        colors.append(color_map[name])

    fig, ax = new_figure(ncols=2, height=95 / 25.4)
    x = np.arange(len(labels))
    ax.bar(x, cvs, color=colors, edgecolor="black", linewidth=0.6, width=0.55, zorder=3)

    for i, thr in enumerate(thresholds):
        ax.hlines(
            thr,
            i - 0.32,
            i + 0.32,
            colors=COLORS["threshold"],
            linestyles="--",
            linewidth=1.0,
            zorder=4,
        )
        ax.text(
            i + 0.34,
            thr,
            f"{thr:g}%",
            fontsize=6,
            va="center",
            color=COLORS["threshold"],
        )

    ax.set_xticks(x)
    ax.set_xticklabels(labels)
    ax.set_ylabel("CV, %")
    ax.set_title("Воспроизводимость измерений\n(CV на filtered runs)")
    ax.plot([], [], color=COLORS["threshold"], linestyle="--", label="Порог CV по сценарию")
    ax.legend(loc="upper center", bbox_to_anchor=(0.5, -0.12))

    for i, cv in enumerate(cvs):
        ax.text(i, cv + 0.35, f"{cv:.1f}", ha="center", va="bottom", fontsize=7)

    thr_summary = ", ".join(f"{labels[i]} {thresholds[i]:g}%" for i in range(len(labels)))
    stem = "fig04_cv_scenarios"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 4 — Коэффициент вариации (CV) первичной метрики по сценариям "
        f"(пороги: {thr_summary}). CV вычислен по отфильтрованным run (1,5×IQR).",
    )
