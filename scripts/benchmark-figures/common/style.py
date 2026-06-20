"""Оформление графиков для НИР (ГОСТ 7.32 / практика научных публикаций)."""

from __future__ import annotations

from pathlib import Path

import matplotlib as mpl
import matplotlib.pyplot as plt

# Увеличенные размеры холста: меньше риск наложения текста.
MM_TO_IN = 1.0 / 25.4
FIG_W_SINGLE = 120 * MM_TO_IN
FIG_W_DOUBLE = 200 * MM_TO_IN
FIG_H_DEFAULT = 85 * MM_TO_IN

# Палитра: контрастна в ч/б и для дальтонизма (ColorBrewer Set2-подобная).
COLORS = {
    "s0": "#4C72B0",
    "s_ref": "#55A868",
    "s_dw": "#C44E52",
    "mirror_off": "#4C72B0",
    "mirror_on": "#DD8452",
    "threshold": "#7F7F7F",
    "grid": "#B0B0B0",
}

SCENARIO_LABELS = {
    "s0-control": r"$S_0$ (контроль)",
    "s-ref": r"$S_{\mathrm{ref}}$ (эталон)",
    "s-dw": r"$S_{\mathrm{dw}}$ (СДПС)",
    "h4-mirror-off": "Mirror off",
    "h4-mirror-on": "Mirror on",
}


def apply_thesis_style() -> None:
    """Глобальные параметры matplotlib для печати и вставки в текст НИР."""
    # Обычный ровный шрифт без засечек для лучшей читаемости на плотных графиках.
    preferred = ["Arial", "DejaVu Sans", "Liberation Sans", "sans-serif"]
    mpl.rcParams.update(
        {
            "font.family": "sans-serif",
            "font.sans-serif": preferred,
            "font.size": 8,
            "axes.labelsize": 8,
            "axes.titlesize": 9,
            "xtick.labelsize": 7,
            "ytick.labelsize": 7,
            "legend.fontsize": 7,
            "figure.dpi": 300,
            "savefig.dpi": 300,
            "savefig.bbox": "tight",
            "savefig.pad_inches": 0.03,
            "axes.linewidth": 0.8,
            "xtick.major.width": 0.8,
            "ytick.major.width": 0.8,
            "xtick.direction": "in",
            "ytick.direction": "in",
            "axes.grid": True,
            "grid.color": COLORS["grid"],
            "grid.linestyle": "--",
            "grid.linewidth": 0.5,
            "grid.alpha": 0.45,
            "legend.frameon": False,
            "mathtext.fontset": "dejavusans",
            "figure.constrained_layout.use": True,
        }
    )


def new_figure(
    width: float = FIG_W_SINGLE,
    height: float = FIG_H_DEFAULT,
    *,
    ncols: int = 1,
) -> tuple[plt.Figure, plt.Axes]:
    w = FIG_W_DOUBLE if ncols > 1 else width
    fig, ax = plt.subplots(figsize=(w, height))
    return fig, ax


def save_figure(fig: plt.Figure, out_dir: Path, stem: str) -> None:
    """Сохранить PDF (вектор) и PNG (растровый архив)."""
    out_dir.mkdir(parents=True, exist_ok=True)
    fig.savefig(out_dir / f"{stem}.pdf")
    fig.savefig(out_dir / f"{stem}.png")
    plt.close(fig)


def write_caption(out_dir: Path, stem: str, text: str) -> None:
    """Текст подрисуночной подписи для вставки в Word/LaTeX."""
    out_dir.mkdir(parents=True, exist_ok=True)
    (out_dir / f"{stem}_caption.txt").write_text(text.strip() + "\n", encoding="utf-8")
