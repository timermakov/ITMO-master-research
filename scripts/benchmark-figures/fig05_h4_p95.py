"""Рис. 5 — H4: p95 E2E latency (block-medians, steady phase)."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import load_h4_results, summary_us
from common.style import COLORS, SCENARIO_LABELS, apply_thesis_style, new_figure, save_figure, write_caption


def plot(out_dir: Path, h4_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_h4_results(h4_path)
    hypo = payload.get("hypotheses", {}).get("H4", "?")

    off = summary_us(next(r for r in payload["results"] if r["scenario"] == "h4-mirror-off")["summary"])
    on = summary_us(next(r for r in payload["results"] if r["scenario"] == "h4-mirror-on")["summary"])
    bound = off["p95"] * 1.05

    labels = [SCENARIO_LABELS["h4-mirror-off"], SCENARIO_LABELS["h4-mirror-on"]]
    p95 = [off["p95"], on["p95"]]
    colors = [COLORS["mirror_off"], COLORS["mirror_on"]]

    fig, ax = new_figure(height=72 / 25.4)
    x = np.arange(2)
    ax.bar(x, p95, color=colors, edgecolor="black", linewidth=0.6, width=0.45, zorder=3)
    ax.axhline(bound, color=COLORS["threshold"], linestyle="--", linewidth=1.0,
               label=r"H4: p95(on) $\leq$ p95(off) $\times 1{,}05$")

    ax.set_xticks(x)
    ax.set_xticklabels(labels)
    ax.set_ylabel("E2E P95, мкс")
    ax.set_title("Overhead mirror-proxy (steady phase)")
    ax.legend(loc="upper center", bbox_to_anchor=(0.5, -0.12))

    for i, val in enumerate(p95):
        ax.text(i, val - 40, f"{val:.0f}", ha="center", va="top", fontsize=8, color="white")

    ax.text(
        0.5,
        -0.22,
        f"H4: {hypo}",
        transform=ax.transAxes,
        ha="center",
        va="top",
        fontsize=8,
    )

    stem = "fig05_h4_p95"
    save_figure(fig, out_dir, stem)
    overhead_pct = (on["p95"] / off["p95"] - 1) * 100
    write_caption(
        out_dir,
        stem,
        "Рисунок 5 — Сравнение P95 end-to-end задержки GET /work через mirror-proxy "
        f"при mirror off ({off['p95']:.0f} мкс) и mirror on ({on['p95']:.0f} мкс). "
        f"Относительный overhead: {overhead_pct:+.1f}\\%. Пунктир — граница H4 (+5\\%). "
        f"Агрегация: медианы 1-секундных блоков steady-фазы.",
    )
