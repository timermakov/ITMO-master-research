"""Рис. 5 — H4: влияние зеркалирования на задержку ответа."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import load_h4_results, summary_us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, new_figure, save_figure, write_caption


def _h4_metric(row: dict) -> tuple[float, float, float, str]:
    summary = summary_us(row["summary"])
    runs = row.get("runs") or []
    if len(runs) >= 5:
        return summary["p50"], summary["p50_lo"], summary["p50_hi"], "медиана P95 по повторам"
    return summary["p95"], summary["p95"], summary["p95"], "P95 медиан по 1-секундным окнам"


def plot(out_dir: Path, h4_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_h4_results(h4_path)

    off_row = next(r for r in payload["results"] if r["scenario"] == "h4-mirror-off")
    on_row = next(r for r in payload["results"] if r["scenario"] == "h4-mirror-on")
    off_val, off_lo, off_hi, metric_label = _h4_metric(off_row)
    on_val, on_lo, on_hi, _ = _h4_metric(on_row)
    bound = off_val * 1.05

    labels = [SCENARIO_LABELS["h4-mirror-off"], SCENARIO_LABELS["h4-mirror-on"]]
    values = [off_val, on_val]
    err_lo = [off_val - off_lo, on_val - on_lo]
    err_hi = [off_hi - off_val, on_hi - on_val]
    colors = [COLORS["mirror_off"], COLORS["mirror_on"]]

    fig, ax = new_figure(height=72 / 25.4)
    x = np.arange(2)
    ax.bar(x, values, color=colors, edgecolor="black", linewidth=0.6, width=0.45, zorder=3)
    if any(err_hi):
        ax.errorbar(x, values, yerr=[err_lo, err_hi], fmt="none", ecolor="black", capsize=4, zorder=4)
    ax.axhline(bound, color=COLORS["threshold"], linestyle="--", linewidth=1.0,
               label="допустимый порог: +5%")

    ax.set_xticks(x)
    ax.set_xticklabels(labels)
    ax.set_ylabel("Задержка ответа, мкс")
    ax.set_title("Влияние зеркалирования на задержку ответа")
    ax.legend(loc="upper center", bbox_to_anchor=(0.5, -0.12))

    for i, val in enumerate(values):
        ax.text(i, val - 40, f"{val:.0f}", ha="center", va="top", fontsize=8, color="white")

    delta_us = on_val - off_val
    overhead_pct = (on_val / off_val - 1) * 100
    ax.text(
        0.5,
        -0.22,
        f"Накладные расходы {delta_us:+.0f} мкс ({overhead_pct:+.1f}%)",
        transform=ax.transAxes,
        ha="center",
        va="top",
        fontsize=8,
    )

    stem = "fig05_h4_p95"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        "Рисунок 5 — Влияние зеркалирования на полную задержку ответа GET /work через "
        f"mirror-proxy: без зеркалирования {off_val:.0f} мкс, с зеркалированием {on_val:.0f} мкс. "
        f"Абсолютная разница {delta_us:+.0f} мкс, относительные накладные расходы {overhead_pct:+.1f}\\%. "
        f"Пунктир — допустимое увеличение на 5\\%. Метрика: {metric_label} стабильной фазы.",
    )
