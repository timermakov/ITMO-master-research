"""Загрузка results.json из benchmark."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

NS_TO_US = 1e-3
NS_TO_MS = 1e-6

SCENARIO_ORDER = ("s0-control", "s-ref", "s-dw")
OVERHEAD_ORDER = ("overhead-mirror-off", "overhead-mirror-on")
# Legacy ids from older benchmark runs.
OVERHEAD_ORDER_LEGACY = ("h4-mirror-off", "h4-mirror-on")


def project_root() -> Path:
    return Path(__file__).resolve().parents[3]


def load_json(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as f:
        return json.load(f)


def find_result_by_scenario(payload: dict[str, Any], name: str) -> dict[str, Any] | None:
    for item in payload.get("results", []):
        if item.get("scenario") == name:
            return item
    return None


def load_full_results(path: Path | None = None) -> dict[str, Any]:
    p = path or project_root() / "results" / "full" / "results.json"
    if not p.is_file():
        raise FileNotFoundError(f"Не найден файл результатов: {p}")
    return load_json(p)


def load_overhead_results(path: Path | None = None) -> dict[str, Any]:
    p = path or project_root() / "results" / "overhead" / "results.json"
    if not p.is_file():
        legacy = project_root() / "results" / "h4" / "results.json"
        if legacy.is_file():
            p = legacy
    if not p.is_file():
        raise FileNotFoundError(f"Не найден файл overhead: {p}")
    return load_json(p)


def load_h4_results(path: Path | None = None) -> dict[str, Any]:
    return load_overhead_results(path)


def us(values_ns: list[float]) -> list[float]:
    return [v * NS_TO_US for v in values_ns]


def summary_us(summary: dict[str, Any]) -> dict[str, float]:
    return {
        "p50": summary["p50Ns"] * NS_TO_US,
        "p95": summary["p95Ns"] * NS_TO_US,
        "p50_lo": summary["p50Ci95LowNs"] * NS_TO_US,
        "p50_hi": summary["p50Ci95HighNs"] * NS_TO_US,
        "cv": summary["cvPercent"],
        "n": summary["n"],
        "n_filtered": summary["nFiltered"],
    }
