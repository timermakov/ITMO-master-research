"""Рис. 7 — профиль нагрузки ramp-up / steady / ramp-down."""

from __future__ import annotations

from pathlib import Path

import numpy as np

from common.data import load_full_results
from common.style import COLORS, apply_thesis_style, new_figure, save_figure, write_caption


def _target_rps(t: float, ramp_up: int, steady: int, ramp_down: int, max_rps: int) -> float:
    total = ramp_up + steady + ramp_down
    if t < 0 or t >= total:
        return 0.0
    if ramp_up > 0 and t < ramp_up:
        return max_rps * t / ramp_up
    if t < ramp_up + steady:
        return float(max_rps)
    if ramp_down <= 0:
        return 0.0
    remaining = ramp_up + steady + ramp_down - t
    return max_rps * remaining / ramp_down


def plot(out_dir: Path, full_path: Path | None = None) -> None:
    apply_thesis_style()
    payload = load_full_results(full_path)
    m = payload["manifest"]
    ramp = m["rampUpSec"]
    steady = m["steadySec"]
    down = m["rampDownSec"]
    max_rps = m["rps"]
    total = ramp + steady + down

    t = np.linspace(0, total, 500)
    rps = [_target_rps(x, ramp, steady, down, max_rps) for x in t]

    fig, ax = new_figure(width=170 / 25.4, height=58 / 25.4, ncols=2)
    ax.plot(t, rps, color=COLORS["s_dw"], linewidth=1.2, zorder=3)
    ax.axvspan(0, ramp, alpha=0.12, color=COLORS["s_ref"], label="Ramp-up")
    ax.axvspan(ramp, ramp + steady, alpha=0.12, color=COLORS["s0"], label="Steady (метрики)")
    ax.axvspan(ramp + steady, total, alpha=0.12, color=COLORS["mirror_on"], label="Ramp-down")

    ax.set_xlabel("Время, с")
    ax.set_ylabel("Целевой RPS")
    ax.set_title("Профиль нагрузки loadgen (протокол v2)")
    ax.set_xlim(0, total)
    ax.set_ylim(0, max_rps * 1.08)
    ax.legend(loc="upper right", ncol=3, fontsize=10)

    stem = "fig07_load_profile"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        f"Рисунок 7 — Трёхфазный профиль нагрузки: ramp-up {ramp} с, "
        f"steady {steady} с (окно съёма метрик), ramp-down {down} с; "
        f"max RPS = {max_rps}. Линейный рост/спад RPS согласно методике нагрузочного тестирования.",
    )
