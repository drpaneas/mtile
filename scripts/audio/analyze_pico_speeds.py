#!/usr/bin/env python3
"""Compare PICO-8 and mGBA four-note melodies across SFX speeds."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

import numpy as np

from analyze_pico_sfx_notes import (
    PITCH_ZERO_HZ,
    Recording,
    active_runs,
    cents,
    harmonic_signature,
    read_wav,
    zero_crossing_frequency,
)


def clip(recording: Recording, run: tuple[int, int]) -> Recording:
    return Recording(recording.rate, recording.samples[run[0] : run[1]])


def pitch_block(recording: Recording, start: float, end: float) -> Recording:
    low = max(0, round(len(recording.samples) * start))
    high = min(len(recording.samples), round(len(recording.samples) * end))
    return Recording(recording.rate, recording.samples[low:high])


def block_metrics(
    pico: Recording, mgba: Recording, pitch: int, start: float, end: float
) -> tuple[float, float, float]:
    pico_block = pitch_block(pico, start, end)
    mgba_block = pitch_block(mgba, start, end)
    expected = PITCH_ZERO_HZ * 2.0 ** (pitch / 12.0)
    # The speed fixture uses duplicated square-wave notes, leaving enough
    # complete periods even at speed 1. Crossing periods are independent of
    # the differently sized PICO/mGBA FFT windows and therefore avoid leakage
    # differences that are unrelated to the sequencer clock under test.
    pico_hz = zero_crossing_frequency(pico_block)
    mgba_hz = zero_crossing_frequency(mgba_block)
    delta = cents(mgba_hz, pico_hz)
    harmonics = max(1, min(12, int(7372.0 // expected)))
    timbre = float(
        np.dot(
            harmonic_signature(pico_block, pico_hz, harmonics),
            harmonic_signature(mgba_block, mgba_hz, harmonics),
        )
    )
    pico_rms = float(np.sqrt(np.mean(pico_block.samples**2)))
    mgba_rms = float(np.sqrt(np.mean(mgba_block.samples**2)))
    gain = mgba_rms / pico_rms if pico_rms else math.nan
    return delta, timbre, gain


def transition_time(recording: Recording) -> float:
    samples = recording.samples - float(np.mean(recording.samples))
    crossings = np.flatnonzero((samples[:-1] <= 0) & (samples[1:] > 0))
    positions = []
    for index in crossings:
        left, right = samples[index], samples[index + 1]
        fraction = 0.0 if right == left else -left / (right - left)
        positions.append(float(index) + float(fraction))
    positions = np.asarray(positions)
    if len(positions) < 6:
        return math.nan
    frequencies = recording.rate / np.diff(positions)
    times = (positions[:-1] + positions[1:]) * 0.5 / recording.rate
    duration = len(samples) / recording.rate
    high = frequencies > 370.0
    for index in range(2, len(high) - 2):
        if (
            duration * 0.2 < times[index] < duration * 0.8
            and bool(np.all(high[index : index + 3]))
        ):
            return float(times[index])
    return math.nan


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--pico", type=Path, required=True)
    parser.add_argument("--mgba", type=Path, required=True)
    parser.add_argument("--max-duration-ms", type=float, default=5.0)
    parser.add_argument("--max-transition-ms", type=float, default=4.0)
    parser.add_argument("--max-pitch-cents", type=float, default=18.0)
    parser.add_argument("--max-gain-error", type=float, default=0.05)
    parser.add_argument("--json", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    speeds = [int(value) for value in manifest["speeds"]]
    pitches = [int(value) for value in manifest["pitches"]]
    if pitches != [24, 24, 36, 36]:
        raise ValueError(f"unsupported speed melody: {pitches}")
    pico_recording = read_wav(args.pico)
    mgba_recording = read_wav(args.mgba)
    pico_runs = active_runs(pico_recording)
    mgba_runs = active_runs(mgba_recording)
    if len(pico_runs) != len(speeds) or len(mgba_runs) != len(speeds):
        raise ValueError(
            f"unexpected active-run count: PICO={len(pico_runs)}, "
            f"mGBA={len(mgba_runs)}, expected={len(speeds)}"
        )

    rows: list[dict[str, object]] = []
    failures = 0
    pico_unit_samples = int(manifest["picoSamplesPerSpeedUnit"])
    pico_rate = int(manifest["picoSampleRate"])
    for speed, pico_run, mgba_run in zip(speeds, pico_runs, mgba_runs, strict=True):
        pico = clip(pico_recording, pico_run)
        mgba = clip(mgba_recording, mgba_run)
        pico_duration_ms = len(pico.samples) * 1000.0 / pico.rate
        mgba_duration_ms = len(mgba.samples) * 1000.0 / mgba.rate
        nominal_duration_ms = speed * len(pitches) * pico_unit_samples * 1000.0 / pico_rate
        duration_delta_ms = mgba_duration_ms - pico_duration_ms
        pico_transition_ms = transition_time(pico) * 1000.0
        mgba_transition_ms = transition_time(mgba) * 1000.0
        transition_delta_ms = mgba_transition_ms - pico_transition_ms
        low = block_metrics(pico, mgba, 24, 0.04, 0.46)
        high = block_metrics(pico, mgba, 36, 0.54, 0.96)
        max_pitch_delta = max(abs(low[0]), abs(high[0]))
        min_timbre = min(low[1], high[1])
        max_gain_error = max(abs(low[2] - 1.0), abs(high[2] - 1.0))
        passed = (
            abs(duration_delta_ms) <= args.max_duration_ms
            and math.isfinite(transition_delta_ms)
            and abs(transition_delta_ms) <= args.max_transition_ms
            and max_pitch_delta <= args.max_pitch_cents
            and max_gain_error <= args.max_gain_error
        )
        if not passed:
            failures += 1
        rows.append(
            {
                "speed": speed,
                "nominalDurationMS": nominal_duration_ms,
                "picoDurationMS": pico_duration_ms,
                "mtileDurationMS": mgba_duration_ms,
                "durationDeltaMS": duration_delta_ms,
                "picoTransitionMS": pico_transition_ms,
                "mtileTransitionMS": mgba_transition_ms,
                "transitionDeltaMS": transition_delta_ms,
                "lowPitchDeltaCents": low[0],
                "highPitchDeltaCents": high[0],
                "lowTimbreCorrelation": low[1],
                "highTimbreCorrelation": high[1],
                "lowGainRatio": low[2],
                "highGainRatio": high[2],
                "pass": passed,
            }
        )
        print(
            f"speed {speed:03d}  duration={pico_duration_ms:8.2f}/"
            f"{mgba_duration_ms:8.2f}ms ({duration_delta_ms:+5.2f})  "
            f"edge={transition_delta_ms:+5.2f}ms  "
            f"pitch={low[0]:+6.2f}/{high[0]:+6.2f}c  "
            f"timbre={low[1]:.4f}/{high[1]:.4f}  "
            f"gain={low[2]:.4f}/{high[2]:.4f}  "
            f"{'PASS' if passed else 'FAIL'}"
        )

    output = {
        "version": 1,
        "metric": "four-note-melody-speed",
        "fixture": manifest,
        "speeds": rows,
        "summary": {"cases": len(rows), "failures": failures},
    }
    if args.json:
        args.json.write_text(json.dumps(output, indent=2) + "\n", encoding="utf-8")
    print(f"summary: {len(rows) - failures}/{len(rows)} speeds pass")
    if failures:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
