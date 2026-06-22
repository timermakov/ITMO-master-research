"""Рис. 6 — временной ряд block-medians H4 (steady phase)."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import NS_TO_US, load_h4_results
from common.style import COLORS, apply_thesis_style, new_figure, save_figure, write_caption


def plot(out_dir: Path, h4_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_h4_results(h4_path)
    manifest = payload["manifest"]
    steady_sec = manifest["h4SteadySec"]

    fig, ax = new_figure(width=170 / 25.4, height=75 / 25.4, ncols=2)

    for name, color, label in (
        ("h4-mirror-off", COLORS["mirror_off"], "Зеркалирование выключено"),
        ("h4-mirror-on", COLORS["mirror_on"], "Зеркалирование включено"),
    ):
        row = next(r for r in payload["results"] if r["scenario"] == name)
        blocks = [v * NS_TO_US for v in row.get("blockValuesNs", [])]
        if not blocks:
            continue
        t = np.arange(1, len(blocks) + 1)
        ax.plot(t, blocks, color=color, linewidth=0.9, label=label, alpha=0.85)

    ax.set_xlabel("Номер блока (1 блок = 1 с steady)")
    ax.set_ylabel("Медиана E2E, мкс")
    ax.set_title(f"Диагностика H4: медианы 1-секундных блоков ({steady_sec} с)")
    ax.set_xlim(1, None)
    ax.legend(loc="upper right")

    stem = "fig06_h4_blocks_timeseries"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 6 — Временной ряд медиан E2E задержки по 1-секундным блокам "
        f"steady-фазы ({steady_sec} с) при выключенном и включённом зеркалировании. "
        f"Ramp-up и ramp-down в анализ не включались; график используется как диагностический "
        f"контроль стабильности H4.",
    )
