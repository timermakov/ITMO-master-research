#!/usr/bin/env python3
"""Генерация всех рисунков для НИР из results/*.json → figures/."""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

# Allow running as script from repo root or from this directory.
SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import fig01_tfirst_p50 as f01
import fig02_tfirst_runs as f02
import fig03_hypotheses as f03
import fig04_cv as f04
import fig05_h4_p95 as f05
import fig06_h4_blocks as f06
import fig07_load_profile as f07
from common.data import project_root


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def source_meta(path: Path) -> dict:
    with path.open(encoding="utf-8") as f:
        payload = json.load(f)
    manifest = payload.get("manifest", {})
    return {
        "path": str(path.resolve()),
        "sha256": sha256_file(path),
        "protocolVersion": manifest.get("protocolVersion"),
        "gitCommit": manifest.get("gitCommit"),
        "startedAt": manifest.get("startedAt"),
        "hypotheses": payload.get("hypotheses", {}),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Построить рисунки benchmark для НИР")
    parser.add_argument(
        "--full",
        type=Path,
        default=None,
        help="Путь к results/full/results.json",
    )
    parser.add_argument(
        "--overhead",
        type=Path,
        default=None,
        help="Путь к results/overhead/results.json",
    )
    parser.add_argument(
        "--h4",
        type=Path,
        default=None,
        help="Устаревший alias для --overhead",
    )
    parser.add_argument(
        "--out",
        type=Path,
        default=None,
        help="Каталог вывода (по умолчанию figures/)",
    )
    args = parser.parse_args()

    out_dir = args.out or project_root() / "figures"
    out_dir.mkdir(parents=True, exist_ok=True)

    full_path = args.full or project_root() / "results" / "full" / "results.json"
    overhead_path = args.overhead or args.h4 or project_root() / "results" / "overhead" / "results.json"
    if not overhead_path.is_file():
        legacy_h4 = project_root() / "results" / "h4" / "results.json"
        if legacy_h4.is_file():
            overhead_path = legacy_h4

    if not full_path.is_file():
        print(f"ERROR: не найден {full_path}", file=sys.stderr)
        return 1

    figures = [
        "fig01_tfirst_p50",
        "fig02_tfirst_runs",
        "fig03_hypotheses_h1_h2",
        "fig04_cv_scenarios",
    ]

    print(f"Источник: {full_path}")
    f01.plot(out_dir, full_path)
    f02.plot(out_dir, full_path)
    f03.plot(out_dir, full_path)
    f04.plot(out_dir, full_path)
    f07.plot(out_dir, full_path)

    if overhead_path.is_file():
        print(f"Источник overhead: {overhead_path}")
        f05.plot(out_dir, overhead_path)
        f06.plot(out_dir, overhead_path)
        figures.extend(["fig05_h4_p95", "fig06_h4_blocks_timeseries"])
    else:
        print(f"WARN: overhead пропущен — нет {overhead_path}", file=sys.stderr)
    figures.append("fig07_load_profile")

    manifest = {
        "generatedAt": datetime.now(timezone.utc).isoformat(),
        "fullResults": str(full_path.resolve()),
        "overheadResults": str(overhead_path.resolve()) if overhead_path.is_file() else None,
        "sources": {
            "full": source_meta(full_path),
            "overhead": source_meta(overhead_path) if overhead_path.is_file() else None,
        },
        "figures": figures,
    }
    (out_dir / "manifest.json").write_text(
        json.dumps(manifest, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )

    print(f"Готово: {out_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
