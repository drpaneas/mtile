#!/usr/bin/env python3
"""Compare deterministic mGBA and PICO-8 arpeggio pitch contours.

The PCM renderers and cadence clocks intentionally differ, so raw sample
correlation is not meaningful. This script extracts square-wave pitch labels,
fits transition cadence, normalizes each effect by its fitted duration, and
reports normalized pitch-contour cross-correlation.
"""

from __future__ import annotations

import argparse
import math
import wave
from dataclasses import dataclass
from pathlib import Path

import numpy as np


PITCH_ZERO_HZ = 65.406391
ALLOWED_NOTES = np.array([24, 28, 31, 36, 40, 43, 48])
FIRST_GROUP = np.array([36, 40, 43, 48])
SECOND_GROUP = np.array([24, 28, 31, 36])


@dataclass
class Recording:
    rate: int
    samples: np.ndarray


@dataclass
class Fit:
    recording: Recording
    start: float
    cadence: float
    expected: np.ndarray


def read_wav(path: Path) -> Recording:
    with wave.open(str(path), "rb") as source:
        if source.getsampwidth() != 2:
            raise ValueError(f"{path}: expected signed 16-bit PCM")
        rate = source.getframerate()
        channels = source.getnchannels()
        raw = source.readframes(source.getnframes())
    samples = np.frombuffer(raw, dtype="<i2").reshape(-1, channels)
    return Recording(rate, samples.mean(axis=1))


def active_runs(recording: Recording) -> list[tuple[float, float]]:
    hop = max(1, round(0.002 * recording.rate))
    window = max(1, round(0.020 * recording.rate))
    envelope = np.array(
        [
            np.sqrt(np.mean(recording.samples[i : i + window].astype(float) ** 2))
            for i in range(0, len(recording.samples) - window, hop)
        ]
    )
    active = envelope > max(80.0, float(envelope.max()) * 0.08)
    runs: list[tuple[float, float]] = []
    start: int | None = None
    for index, value in enumerate(active):
        if value and start is None:
            start = index
        if start is not None and (not value or index == len(active) - 1):
            end = index if not value else index + 1
            if (end - start) * hop / recording.rate > 0.03:
                runs.append(
                    (start * hop / recording.rate, (end * hop + window) / recording.rate)
                )
            start = None
    return runs


def zero_crossing_pitch(recording: Recording, time: float, window_s: float = 0.016) -> float:
    size = round(window_s * recording.rate)
    center = round(time * recording.rate)
    low = max(0, center - size // 2)
    high = min(len(recording.samples), center + size // 2)
    samples = recording.samples[low:high]
    crossings = np.flatnonzero((samples[:-1] <= 0) & (samples[1:] > 0))
    if len(crossings) < 3:
        return math.nan
    periods = np.diff(crossings)
    median = np.median(periods)
    periods = periods[(periods > median * 0.6) & (periods < median * 1.5)]
    frequency = recording.rate / np.median(periods)
    return 12.0 * math.log2(frequency / PITCH_ZERO_HZ)


def nearest_note(pitch: float) -> int:
    return int(ALLOWED_NOTES[np.argmin(np.abs(ALLOWED_NOTES - pitch))])


def expected_sequence(kind: str, groups: bool) -> np.ndarray:
    per_group = 18 if kind == "fast" else 9
    count = per_group * (2 if groups else 1)
    return np.array(
        [
            (FIRST_GROUP if not groups or index < per_group else SECOND_GROUP)[index % 4]
            for index in range(count)
        ]
    )


def fit_transitions(
    recording: Recording,
    run: tuple[float, float],
    kind: str,
    groups: bool,
) -> Fit:
    expected = expected_sequence(kind, groups)
    times = np.arange(run[0] + 0.004, run[1] + 0.010, 0.001)
    pitches = np.array([zero_crossing_pitch(recording, time) for time in times])
    labels = np.full(len(pitches), -99, dtype=int)
    valid = np.isfinite(pitches)
    labels[valid] = ALLOWED_NOTES[
        np.argmin(np.abs(pitches[valid, None] - ALLOWED_NOTES), axis=1)
    ]

    smoothed = labels.copy()
    for index in range(3, len(labels) - 3):
        values, counts = np.unique(labels[index - 3 : index + 4], return_counts=True)
        smoothed[index] = values[np.argmax(counts)]
    transitions = [
        (times[index], int(smoothed[index - 1]), int(smoothed[index]))
        for index in range(1, len(smoothed))
        if smoothed[index] != smoothed[index - 1]
        and smoothed[index - 1] != -99
        and smoothed[index] != -99
    ]

    interval_low, interval_high = (0.020, 0.045) if kind == "fast" else (0.045, 0.085)
    best: list[float] = []
    for first_index, (time, previous, current) in enumerate(transitions):
        if (previous, current) != (expected[0], expected[1]):
            continue
        chain = [time]
        last = time
        position = first_index + 1
        for expected_index in range(2, len(expected)):
            found: tuple[float, int] | None = None
            for transition_index in range(position, len(transitions)):
                next_time, _, next_note = transitions[transition_index]
                if next_time - last > interval_high:
                    break
                if next_time - last >= interval_low and next_note == expected[expected_index]:
                    found = (next_time, transition_index + 1)
                    break
            if found is None:
                break
            chain.append(found[0])
            last, position = found
        if len(chain) > len(best):
            best = chain

    if len(best) != len(expected) - 1:
        raise ValueError(
            f"found {len(best)} transitions for {kind}{'-groups' if groups else ''}; "
            f"expected {len(expected) - 1}"
        )
    cadence, start = np.polyfit(np.arange(1, len(expected)), np.array(best), 1)
    return Fit(recording, float(start), float(cadence), expected)


def pitch_contour(fit: Fit, points: int = 1440) -> np.ndarray:
    normalized = (np.arange(points) + 0.5) / points
    duration = len(fit.expected) * fit.cadence
    pitches = np.array(
        [
            zero_crossing_pitch(fit.recording, fit.start + value * duration, 0.014)
            for value in normalized
        ]
    )
    labels = np.full(points, -99, dtype=int)
    valid = np.isfinite(pitches)
    labels[valid] = ALLOWED_NOTES[
        np.argmin(np.abs(pitches[valid, None] - ALLOWED_NOTES), axis=1)
    ]
    return labels


def normalized_correlation(left: Fit, right: Fit) -> tuple[float, float, int]:
    left_labels = pitch_contour(left)
    right_labels = pitch_contour(right)
    valid = (left_labels != -99) & (right_labels != -99)
    left_labels = left_labels[valid]
    right_labels = right_labels[valid]

    best = (-1.0, 0.0, 0)
    for lag in range(-15, 16):
        if lag < 0:
            left_slice = left_labels[:lag]
            right_slice = right_labels[-lag:]
        elif lag > 0:
            left_slice = left_labels[lag:]
            right_slice = right_labels[:-lag]
        else:
            left_slice = left_labels
            right_slice = right_labels
        correlation = float(np.corrcoef(left_slice, right_slice)[0, 1])
        match = float(np.mean(left_slice == right_slice))
        if correlation > best[0]:
            best = (correlation, match, lag)
    return best


def boundary_note(fit: Fit, kind: str) -> int:
    boundary = 18 if kind == "fast" else 9
    time = fit.start + (boundary + 0.5) * fit.cadence
    return nearest_note(zero_crossing_pitch(fit.recording, time))


def parse_args() -> argparse.Namespace:
    repository = Path(__file__).resolve().parents[2]
    pico_dir = Path.home() / "Library/Application Support/pico-8/carts"
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--mgba",
        type=Path,
        default=repository / "docs/audio/captures/mgba-arpeggio-conformance.wav",
    )
    parser.add_argument(
        "--pico-reference",
        type=Path,
        default=pico_dir / "mtile_arp_reference.wav",
    )
    parser.add_argument(
        "--pico-groups",
        type=Path,
        default=pico_dir / "mtile_arp_groups.wav",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    mgba = read_wav(args.mgba)
    pico_reference = read_wav(args.pico_reference)
    pico_groups = read_wav(args.pico_groups)
    mgba_runs = active_runs(mgba)
    pico_reference_runs = active_runs(pico_reference)
    pico_group_runs = active_runs(pico_groups)
    if len(mgba_runs) != 4 or len(pico_reference_runs) != 2 or len(pico_group_runs) != 2:
        raise ValueError(
            "unexpected active-run count: "
            f"mGBA={len(mgba_runs)}, PICO reference={len(pico_reference_runs)}, "
            f"PICO groups={len(pico_group_runs)}"
        )

    cases = [
        ("fast", False, mgba_runs[0], pico_reference, pico_reference_runs[0]),
        ("slow", False, mgba_runs[1], pico_reference, pico_reference_runs[1]),
        ("fast-groups", True, mgba_runs[2], pico_groups, pico_group_runs[0]),
        ("slow-groups", True, mgba_runs[3], pico_groups, pico_group_runs[1]),
    ]
    fits: dict[str, tuple[Fit, Fit]] = {}
    for name, groups, mgba_run, pico_recording, pico_run in cases:
        kind = name.split("-", 1)[0]
        mgba_fit = fit_transitions(mgba, mgba_run, kind, groups)
        pico_fit = fit_transitions(pico_recording, pico_run, kind, groups)
        fits[name] = (mgba_fit, pico_fit)
        correlation, match, lag = normalized_correlation(mgba_fit, pico_fit)
        print(
            f"{name:11s} mGBA={mgba_fit.cadence * 1000:7.3f} ms  "
            f"PICO={pico_fit.cadence * 1000:7.3f} ms  "
            f"xcorr={correlation:.4f}  labels={match * 100:5.2f}%  lag={lag:+d}/1440"
        )

    fast_mgba, fast_pico = fits["fast-groups"]
    slow_mgba, slow_pico = fits["slow-groups"]
    print(
        "group boundary: "
        f"fast mGBA/PICO={boundary_note(fast_mgba, 'fast')}/{boundary_note(fast_pico, 'fast')}, "
        f"slow mGBA/PICO={boundary_note(slow_mgba, 'slow')}/{boundary_note(slow_pico, 'slow')}"
    )


if __name__ == "__main__":
    main()
