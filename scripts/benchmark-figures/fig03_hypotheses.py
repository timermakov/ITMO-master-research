"""Рис. 3 — проверка гипотез H1 и H2 (отношения медиан)."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import load_full_results, summary_us
from common.style import COLORS, apply_thesis_style, new_figure, save_figure, write_caption


def plot(out_dir: Path, full_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_full_results(full_path)
    hypo = payload.get("hypotheses", {})

    s0 = summary_us(next(r for r in payload["results"] if r["scenario"] == "s0-control")["summary"])
    s_ref = summary_us(next(r for r in payload["results"] if r["scenario"] == "s-ref")["summary"])
    s_dw = summary_us(next(r for r in payload["results"] if r["scenario"] == "s-dw")["summary"])

    h1_ratio = s_dw["p50"] / s0["p50"]
    h2_ratio = s_dw["p50"] / s_ref["p50"]
    h1_bound = 0.5
    h2_bound = 1.1

    labels = [r"$S_{\mathrm{dw}}/S_0$" + f"\n(H1, {hypo.get('H1', '?')})",
              r"$S_{\mathrm{dw}}/S_{\mathrm{ref}}$" + f"\n(H2, {hypo.get('H2', '?')})"]
    values = [h1_ratio, h2_ratio]
    bounds = [h1_bound, h2_bound]

    fig, ax = new_figure()
    x = np.arange(2)
    colors = [COLORS["s_dw"] if v <= b else "#999999" for v, b in zip(values, bounds, strict=True)]
    ax.bar(x, values, color=colors, edgecolor="black", linewidth=0.6, width=0.45, zorder=3)
    ax.axhline(h1_bound, color=COLORS["threshold"], linestyle="--", linewidth=1.0, label=r"H1: $\leq 0{,}5$")
    ax.axhline(h2_bound, color=COLORS["threshold"], linestyle=":", linewidth=1.0, label=r"H2: $\leq 1{,}1$")

    ax.set_xticks(x)
    ax.set_xticklabels(labels)
    ax.set_ylabel("Отношение P50")
    ax.set_ylim(0, max(values + bounds) * 1.25)
    ax.legend(loc="upper right")

    for i, val in enumerate(values):
        ax.text(i, val + 0.02, f"{val:.3f}", ha="center", va="bottom", fontsize=11)

    stem = "fig03_hypotheses_h1_h2"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        "Рисунок 3 — Проверка гипотез H1 и H2 по отношению медиан P50 метрики "
        f"$T_{{\\mathrm{{first}}}}$: H1 — $S_{{\\mathrm{{dw}}}}/S_0 = {h1_ratio:.3f}$ "
        f"(порог 0,5); H2 — $S_{{\\mathrm{{dw}}}}/S_{{\\mathrm{{ref}}}} = {h2_ratio:.3f}$ "
        f"(порог 1,1). H2_CI: {hypo.get('H2_CI', '?')}; H3: {hypo.get('H3', '?')}.",
    )
