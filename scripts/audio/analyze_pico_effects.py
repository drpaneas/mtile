#!/usr/bin/env python3
"""Compare normalized PICO-8 and mGBA note-effect trajectories."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

import numpy as np

from analyze_pico_sfx_notes import Recording, active_runs, read_wav


PITCH_ZERO_HZ = 65.406391


def segment(recording: Recording, run: tuple[int, int]) -> Recording:
    return Recording(recording.rate, recording.samples[run[0] : run[1]])


def normalized_envelope(recording: Recording, points: int = 384) -> np.ndarray:
    radius = max(1, round(recording.rate * 0.006))
    centers = np.linspace(radius, len(recording.samples) - radius - 1, points)
    values = np.array(
        [
            np.sqrt(
                np.mean(
                    recording.samples[
                        max(0, round(center) - radius) : min(
                            len(recording.samples), round(center) + radius
                        )
                    ]
                    ** 2
                )
            )
            for center in centers
        ]
    )
    maximum = float(values.max())
    return values / maximum if maximum else values


def local_pitch(recording: Recording, points: int = 384) -> np.ndarray:
    radius = max(4, round(recording.rate * 0.009))
    centers = np.linspace(radius, len(recording.samples) - radius - 1, points)
    result = np.full(points, np.nan)
    for output, center in enumerate(centers):
        index = round(center)
        samples = recording.samples[index - radius : index + radius]
        samples = samples - float(np.mean(samples))
        crossings = np.flatnonzero((samples[:-1] <= 0) & (samples[1:] > 0))
        if len(crossings) < 3:
            continue
        positions = []
        for crossing in crossings:
            left, right = samples[crossing], samples[crossing + 1]
            fraction = 0.0 if right == left else -left / (right - left)
            positions.append(float(crossing) + float(fraction))
        periods = np.diff(np.asarray(positions))
        median = float(np.median(periods))
        periods = periods[(periods > median * 0.65) & (periods < median * 1.35)]
        if len(periods) < 2:
            continue
        frequency = recording.rate / float(np.median(periods))
        result[output] = 12.0 * math.log2(frequency / PITCH_ZERO_HZ)
    return result


def crossing_pitch_trajectory(recording: Recording, points: int = 384) -> np.ndarray:
    """Track small modulation from individual square-wave cycle lengths."""
    samples = recording.samples - float(np.mean(recording.samples))
    crossings = np.flatnonzero((samples[:-1] <= 0) & (samples[1:] > 0))
    positions = []
    for crossing in crossings:
        left, right = samples[crossing], samples[crossing + 1]
        fraction = 0.0 if right == left else -left / (right - left)
        positions.append(float(crossing) + float(fraction))
    positions = np.asarray(positions)
    if len(positions) < 16:
        return np.full(points, np.nan)
    frequencies = recording.rate / np.diff(positions)
    pitches = 12.0 * np.log2(frequencies / PITCH_ZERO_HZ)
    times = (positions[:-1] + positions[1:]) * 0.5
    valid = np.isfinite(pitches) & (pitches >= 34.0) & (pitches <= 38.0)
    pitches, times = pitches[valid], times[valid]
    if len(pitches) < 16:
        return np.full(points, np.nan)
    # Seven carrier periods suppress sample-quantized crossing jitter while
    # remaining much shorter than one 16-tick vibrato cycle.
    pitches = np.convolve(pitches, np.ones(7) / 7.0, mode="same")
    normalized = (times - times[0]) / (times[-1] - times[0])
    return np.interp(np.linspace(0.03, 0.97, points), normalized, pitches)


def aligned_metrics(
    left: np.ndarray,
    right: np.ndarray,
    max_lag: int = 10,
    keep_error_fraction: float = 0.95,
) -> tuple[float, float, int]:
    best = (-math.inf, math.inf, 0)
    for lag in range(-max_lag, max_lag + 1):
        if lag < 0:
            one, two = left[:lag], right[-lag:]
        elif lag > 0:
            one, two = left[lag:], right[:-lag]
        else:
            one, two = left, right
        valid = np.isfinite(one) & np.isfinite(two)
        one, two = one[valid], two[valid]
        if len(one) < 16:
            continue
        if float(np.std(one)) < 1e-6 and float(np.std(two)) < 1e-6:
            correlation = 1.0
        elif float(np.std(one)) < 1e-6 or float(np.std(two)) < 1e-6:
            correlation = 0.0
        else:
            correlation = float(np.corrcoef(one, two)[0, 1])
        squared_errors = np.sort((one - two) ** 2)
        kept = max(1, math.ceil(len(squared_errors) * keep_error_fraction))
        # Frequency windows centered on a discontinuous slide/arpeggio edge
        # can legitimately see opposite sides of the same transition after
        # sample-rate conversion. Correlation checks the whole trajectory;
        # this trimmed RMSE measures stable windows without letting a handful
        # of edge-straddling windows dominate the physical pitch error.
        error = float(np.sqrt(np.mean(squared_errors[:kept])))
        if correlation > best[0] or (
            math.isclose(correlation, best[0]) and error < best[1]
        ):
            best = (correlation, error, lag)
    return best


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--pico", type=Path, required=True)
    parser.add_argument("--mgba", type=Path, required=True)
    parser.add_argument("--min-pitch-correlation", type=float, default=0.94)
    parser.add_argument("--max-pitch-rmse-semitones", type=float, default=0.35)
    parser.add_argument("--min-envelope-correlation", type=float, default=0.95)
    parser.add_argument("--max-envelope-rmse", type=float, default=0.12)
    parser.add_argument("--json", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    cases = manifest["cases"]
    pico_recording = read_wav(args.pico)
    mgba_recording = read_wav(args.mgba)
    pico_runs = active_runs(pico_recording)
    mgba_runs = active_runs(mgba_recording)
    if len(pico_runs) != len(cases) or len(mgba_runs) != len(cases):
        raise ValueError(
            f"unexpected active-run count: PICO={len(pico_runs)}, "
            f"mGBA={len(mgba_runs)}, expected={len(cases)}"
        )

    results = []
    failures = 0
    for case, pico_run, mgba_run in zip(cases, pico_runs, mgba_runs, strict=True):
        pico = segment(pico_recording, pico_run)
        mgba = segment(mgba_recording, mgba_run)
        pitch_correlation = math.nan
        pitch_rmse = math.nan
        pitch_lag = 0
        envelope_correlation = math.nan
        envelope_rmse = math.nan
        envelope_lag = 0
        passed = True
        if case["checkPitch"]:
            pico_pitch = (
                crossing_pitch_trajectory(pico)
                if case["name"] == "vibrato"
                else local_pitch(pico)
            )
            mgba_pitch = (
                crossing_pitch_trajectory(mgba)
                if case["name"] == "vibrato"
                else local_pitch(mgba)
            )
            # Arpeggios contain many discontinuous pitch edges. At speed 8,
            # more than five percent of the fixed analysis windows straddle a
            # transition after 22,050 -> 16,384 Hz conversion even when every
            # transition boundary agrees. Correlation still checks the whole
            # contour; trim only those edge windows from the physical RMSE.
            pitch_error_fraction = (
                0.93 if case["name"].startswith("arpeggio-") else 0.95
            )
            pitch_correlation, pitch_rmse, pitch_lag = aligned_metrics(
                pico_pitch,
                mgba_pitch,
                max_lag=40 if case["name"] == "vibrato" else 10,
                keep_error_fraction=pitch_error_fraction,
            )
            passed = passed and (
                pitch_correlation >= args.min_pitch_correlation
                and pitch_rmse <= args.max_pitch_rmse_semitones
            )
        if case["checkEnvelope"]:
            envelope_correlation, envelope_rmse, envelope_lag = aligned_metrics(
                normalized_envelope(pico),
                normalized_envelope(mgba),
                keep_error_fraction=1.0,
            )
            passed = passed and (
                envelope_correlation >= args.min_envelope_correlation
                and envelope_rmse <= args.max_envelope_rmse
            )
        if not passed:
            failures += 1
        result = {
            "id": case["id"],
            "name": case["name"],
            "picoDurationMS": len(pico.samples) * 1000.0 / pico.rate,
            "mtileDurationMS": len(mgba.samples) * 1000.0 / mgba.rate,
            "pitchCorrelation": pitch_correlation,
            "pitchRMSESemitones": pitch_rmse,
            "pitchLag": pitch_lag,
            "envelopeCorrelation": envelope_correlation,
            "envelopeRMSE": envelope_rmse,
            "envelopeLag": envelope_lag,
            "pass": passed,
        }
        results.append(result)
        pitch_text = (
            f"pitch={pitch_correlation:.4f}/{pitch_rmse:.3f}st"
            if case["checkPitch"]
            else "pitch=--"
        )
        envelope_text = (
            f"env={envelope_correlation:.4f}/{envelope_rmse:.3f}"
            if case["checkEnvelope"]
            else "env=--"
        )
        print(
            f"{case['name']:23s} {pitch_text:25s} {envelope_text:21s} "
            f"duration={result['picoDurationMS']:.1f}/{result['mtileDurationMS']:.1f}ms "
            f"{'PASS' if passed else 'FAIL'}"
        )

    output = {
        "version": 1,
        "metric": "normalized-effect-trajectories",
        "fixture": manifest,
        "cases": results,
        "summary": {"cases": len(results), "failures": failures},
    }
    if args.json:
        args.json.write_text(json.dumps(output, indent=2) + "\n", encoding="utf-8")
    print(f"summary: {len(results) - failures}/{len(results)} effect cases pass")
    if failures:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
