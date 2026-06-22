"""Рис. 6 — временной ряд медиан задержки H4 по 1-секундным окнам."""

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

    ax.set_xlabel("Номер 1-секундного окна стабильной фазы")
    ax.set_ylabel("Медиана задержки ответа, мкс")
    ax.set_title(f"Диагностика H4: задержка ответа по секундам ({steady_sec} с)")
    ax.set_xlim(1, None)
    ax.legend(loc="upper right")

    stem = "fig06_h4_blocks_timeseries"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 6 — Диагностический временной ряд H4. Одно 1-секундное окно — это все "
        f"запросы, отправленные за одну секунду стабильной фазы; точка на графике показывает "
        f"медиану задержки ответа внутри такого окна. Наращивание и снижение нагрузки "
        f"в анализ не включались; график нужен для контроля стабильности задержки при "
        f"выключенном и включённом зеркалировании.",
    )
