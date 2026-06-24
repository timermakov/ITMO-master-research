"""Рис. 3 — сводная проверка гипотез benchmark."""

from __future__ import annotations

from pathlib import Path

from common.data import load_full_results, summary_us
from common.style import apply_thesis_style, new_figure, save_figure, write_caption


def _verdict(value: str) -> str:
    return "пройдено" if value == "pass" else "не пройдено" if value == "fail" else value


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
    h2_bound = 2.0

    rows = [
        ["H1", "динамический прогрев ≤ 1/2 холодного старта", f"{h1_ratio:.3f} ≤ {h1_bound:.1f}", _verdict(hypo.get("H1", "?"))],
        ["H2", "динамический прогрев ≤ 2× ручного прогрева (P50)", f"{h2_ratio:.3f} ≤ {h2_bound:.1f}", _verdict(hypo.get("H2", "?"))],
        ["H2_CI", "верхняя 95% ДИ P50 динамического ≤ 2× верхней ДИ ручного", "см. results.json", _verdict(hypo.get("H2_CI", "?"))],
        ["H3", "readyz=200 после динамического прогрева", "readyz проверен", _verdict(hypo.get("H3", "?"))],
    ]

    fig, ax = new_figure(width=200 / 25.4, height=82 / 25.4, ncols=2)
    ax.axis("off")
    ax.set_title("Сводная проверка гипотез")
    table = ax.table(
        cellText=rows,
        colLabels=["ID", "Критерий", "Наблюдаемое", "Итог"],
        cellLoc="left",
        colLoc="left",
        loc="center",
        colWidths=[0.10, 0.48, 0.24, 0.12],
    )
    table.auto_set_font_size(False)
    table.set_fontsize(7)
    table.scale(1, 1.4)
    for (row, col), cell in table.get_celld().items():
        cell.set_linewidth(0.4)
        if row == 0:
            cell.set_text_props(weight="bold")
            cell.set_facecolor("#EFEFEF")
        elif col == 3:
            cell.set_text_props(weight="bold")
            if cell.get_text().get_text() == "пройдено":
                cell.set_facecolor("#DDEEDD")

    stem = "fig03_hypotheses_h1_h2"
    save_figure(fig, out_dir, stem)
    write_caption(
        out_dir,
        stem,
        "Рисунок 3 — Сводная проверка гипотез бенчмарка. H1 сравнивает динамический "
        f"прогрев с холодным стартом ({h1_ratio:.3f} при пороге 0,5), H2 — с ручным "
        f"прогревом ({h2_ratio:.3f} при пороге 2,0). H2_CI проверяет верхнюю границу "
        f"95\\% bootstrap ДИ, H3 фиксирует успешную готовность warmup-инстанса.",
    )
