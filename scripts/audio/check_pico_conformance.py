#!/usr/bin/env python3
"""Validate and summarize one complete external PICO-8 audio run."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path


WAVEFORMS = (
    "triangle",
    "tilted-saw",
    "saw",
    "square",
    "pulse",
    "organ",
    "noise",
    "phaser",
)
FILTER_GROUP_CASES = {
    "noiz-noise-all": 64,
    **{f"buzz-{waveform}-p36": 1 for waveform in WAVEFORMS},
    **{
        f"detune{level}-{waveform}-p36": 1
        for level in (1, 2)
        for waveform in WAVEFORMS
    },
    **{f"reverb{level}-triangle-p36": 1 for level in (1, 2)},
    **{f"dampen{level}-saw-p36": 1 for level in (1, 2)},
}
EFFECT_CASES = (
    "none",
    "slide-up",
    "slide-down",
    "vibrato",
    "drop",
    "fade-in",
    "fade-out",
    "arpeggio-fast",
    "arpeggio-slow",
    "arpeggio-fast-speed8",
    "arpeggio-slow-speed8",
)


def load(path: Path) -> dict[str, object]:
    if not path.is_file():
        raise ValueError(f"missing result: {path}")
    return json.loads(path.read_text(encoding="utf-8"))


def json_value(value: object) -> object:
    """Return a strict-JSON value, replacing unavailable metrics with null."""
    if isinstance(value, float) and not math.isfinite(value):
        return None
    if isinstance(value, dict):
        return {key: json_value(item) for key, item in value.items()}
    if isinstance(value, list):
        return [json_value(item) for item in value]
    return value


def summary_counts(result: dict[str, object], path: Path) -> tuple[int, int]:
    summary = result.get("summary")
    if not isinstance(summary, dict):
        raise ValueError(f"missing summary object: {path}")
    return int(summary.get("cases", -1)), int(summary.get("failures", -1))


def instrument_summary(root: Path) -> list[dict[str, object]]:
    rows = []
    for waveform in WAVEFORMS:
        path = root / waveform / "result.json"
        result = load(path)
        cases, failures = summary_counts(result, path)
        fixture = result.get("fixture")
        case_rows = result.get("cases")
        if not isinstance(fixture, dict) or fixture.get("waveform") != waveform:
            raise ValueError(f"{waveform}: result fixture names another waveform")
        if not isinstance(case_rows, list):
            raise ValueError(f"{waveform}: missing cases array")
        pitches = [int(row.get("pitch", -1)) for row in case_rows]
        if pitches != list(range(64)) or not all(bool(row.get("pass")) for row in case_rows):
            raise ValueError(f"{waveform}: cases are not ordered passing pitches 0..63")
        if cases != 64 or failures != 0:
            raise ValueError(
                f"{waveform}: expected 64 cases and no failures, found "
                f"{cases} cases and {failures} failures"
            )
        summary = result["summary"]
        rows.append(
            {
                "waveform": waveform,
                "cases": cases,
                "failures": failures,
                "maxAbsolutePitchCents": summary["maxAbsolutePitchCents"],
                "minTimbreCorrelation": summary["minTimbreCorrelation"],
                "medianAmplitudeRatio": summary["medianAmplitudeRatio"],
            }
        )
    return rows


def speed_summary(root: Path) -> dict[str, object]:
    rows: list[dict[str, object]] = []
    for path in sorted(root.glob("*/result.json")):
        result = load(path)
        values = result.get("speeds")
        if not isinstance(values, list):
            raise ValueError(f"missing speeds array: {path}")
        rows.extend(values)
    by_speed = {int(row["speed"]): row for row in rows}
    expected = set(range(1, 256))
    if set(by_speed) != expected or len(rows) != len(expected):
        missing = sorted(expected - set(by_speed))
        duplicates = len(rows) - len(by_speed)
        raise ValueError(
            f"speed matrix is incomplete: {len(by_speed)}/255 unique, "
            f"{duplicates} duplicates, missing {missing[:8]}"
        )
    failures = [speed for speed, row in by_speed.items() if not bool(row["pass"])]
    if failures:
        raise ValueError(f"speed failures: {failures}")
    values = list(by_speed.values())
    return {
        "cases": len(values),
        "failures": 0,
        "maxAbsoluteDurationDeltaMS": max(abs(float(row["durationDeltaMS"])) for row in values),
        "maxAbsoluteTransitionDeltaMS": max(abs(float(row["transitionDeltaMS"])) for row in values),
        "maxAbsoluteLowPitchCents": max(abs(float(row["lowPitchDeltaCents"])) for row in values),
        "maxAbsoluteHighPitchCents": max(abs(float(row["highPitchDeltaCents"])) for row in values),
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--instruments", type=Path, required=True)
    parser.add_argument("--effects", type=Path, required=True)
    parser.add_argument("--filters", type=Path, required=True)
    parser.add_argument("--speeds", type=Path, required=True)
    parser.add_argument("--json", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    instruments = instrument_summary(args.instruments)

    effects_path = args.effects / "result.json"
    effects_result = load(effects_path)
    effect_cases, effect_failures = summary_counts(effects_result, effects_path)
    effect_results = effects_result.get("cases")
    if not isinstance(effect_results, list):
        raise ValueError("effects: missing cases array")
    effect_names = [str(row.get("name")) for row in effect_results]
    if effect_names != list(EFFECT_CASES) or not all(bool(row.get("pass")) for row in effect_results):
        raise ValueError("effects: cases are missing, out of order, or failed")
    if effect_cases != 11 or effect_failures != 0:
        raise ValueError(
            f"effects: expected 11 cases and no failures, found "
            f"{effect_cases} cases and {effect_failures} failures"
        )

    filters_path = args.filters / "summary.json"
    filters = load(filters_path)
    filter_results = filters.get("results")
    if not isinstance(filter_results, list):
        raise ValueError("filters: missing results array")
    filter_by_name = {str(row.get("case")): row for row in filter_results}
    if set(filter_by_name) != set(FILTER_GROUP_CASES) or len(filter_results) != len(FILTER_GROUP_CASES):
        missing = sorted(set(FILTER_GROUP_CASES) - set(filter_by_name))
        extra = sorted(set(filter_by_name) - set(FILTER_GROUP_CASES))
        raise ValueError(f"filters: wrong group set; missing {missing}, extra {extra}")
    for name, expected_cases in FILTER_GROUP_CASES.items():
        row = filter_by_name[name]
        if int(row.get("cases", -1)) != expected_cases or int(row.get("failures", -1)) != 0:
            raise ValueError(
                f"filters: {name} expected {expected_cases} cases and no failures; "
                f"found {row.get('cases')} and {row.get('failures')}"
            )
    if (
        int(filters.get("groups", -1)) != 29
        or int(filters.get("cases", -1)) != 92
        or int(filters.get("failures", -1)) != 0
    ):
        raise ValueError(
            "filters: expected 29 groups, 92 cases, and no failures; "
            f"found {filters.get('groups')}, {filters.get('cases')}, "
            f"{filters.get('failures')}"
        )

    report = {
        "version": 1,
        "summary": {
            "cases": 512 + effect_cases + int(filters["cases"]) + 255,
            "failures": 0,
        },
        "instruments": instruments,
        "effects": {
            "cases": effect_cases,
            "failures": 0,
            "results": effect_results,
        },
        "filters": {
            "groups": int(filters["groups"]),
            "cases": int(filters["cases"]),
            "failures": 0,
            "results": filter_results,
        },
        "speeds": speed_summary(args.speeds),
    }
    encoded = json.dumps(json_value(report), indent=2, allow_nan=False) + "\n"
    if args.json:
        args.json.write_text(encoded, encoding="utf-8")
    print(encoded, end="")


if __name__ == "__main__":
    try:
        main()
    except ValueError as error:
        raise SystemExit(f"conformance incomplete: {error}") from error
