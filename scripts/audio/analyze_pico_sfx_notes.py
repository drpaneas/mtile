#!/usr/bin/env python3
"""Compare ordered PICO-8 and mGBA single-note captures."""

from __future__ import annotations

import argparse
import json
import math
import wave
from dataclasses import dataclass
from pathlib import Path

import numpy as np


PITCH_ZERO_HZ = 65.406391


@dataclass
class Recording:
    rate: int
    samples: np.ndarray


def read_wav(path: Path) -> Recording:
    with wave.open(str(path), "rb") as source:
        width = source.getsampwidth()
        channels = source.getnchannels()
        rate = source.getframerate()
        raw = source.readframes(source.getnframes())
    if width == 1:
        samples = np.frombuffer(raw, dtype=np.uint8).astype(np.float64) - 128.0
    elif width == 2:
        samples = np.frombuffer(raw, dtype="<i2").astype(np.float64)
    else:
        raise ValueError(f"{path}: expected 8-bit or 16-bit PCM, found {width * 8}-bit")
    samples = samples.reshape(-1, channels).mean(axis=1)
    return Recording(rate=rate, samples=samples)


def active_runs(
    recording: Recording, bridge_seconds: float = 0.012
) -> list[tuple[int, int]]:
    samples = recording.samples
    window = max(1, round(recording.rate * 0.004))
    energy = np.convolve(np.abs(samples), np.ones(window) / window, mode="same")
    threshold = max(4.0, float(energy.max()) * 0.025)
    active = energy > threshold
    bridge = max(1, round(recording.rate * bridge_seconds))
    active_indices = np.flatnonzero(active)
    if len(active_indices) == 0:
        return []
    runs: list[tuple[int, int]] = []
    start = int(active_indices[0])
    previous = start
    for value in active_indices[1:]:
        index = int(value)
        if index - previous > bridge:
            runs.append((start, previous + 1))
            start = index
        previous = index
    runs.append((start, previous + 1))
    pad = window // 2
    return [(max(0, start - pad), min(len(samples), end + pad)) for start, end in runs]


def trim_active(recording: Recording, bridge_seconds: float = 0.012) -> Recording:
    runs = active_runs(recording, bridge_seconds)
    if not runs:
        raise ValueError("recording contains no active audio")
    start, end = max(runs, key=lambda run: run[1] - run[0])
    return Recording(recording.rate, recording.samples[start:end])


def spectral_frequency(recording: Recording, expected: float) -> float:
    samples = recording.samples - float(np.mean(recording.samples))
    if len(samples) < 8:
        return math.nan
    # Zero-crossing periods become heavily quantized near the top of PICO's
    # pitch range. A padded, windowed FFT gives a phase-independent estimate
    # at both source rates while the one-semitone search window prevents a
    # stronger harmonic from being mistaken for the authored fundamental.
    size = 1 << (max(256, len(samples) * 32) - 1).bit_length()
    spectrum = np.abs(np.fft.rfft(samples * np.hanning(len(samples)), n=size))
    frequencies = np.fft.rfftfreq(size, d=1.0 / recording.rate)
    ratio = 2.0 ** (1.0 / 24.0)
    candidates = np.flatnonzero(
        (frequencies >= expected / ratio) & (frequencies <= expected * ratio)
    )
    if len(candidates) == 0:
        return math.nan
    index = int(candidates[np.argmax(spectrum[candidates])])
    offset = 0.0
    if 0 < index < len(spectrum) - 1:
        left, center, right = np.log(np.maximum(spectrum[index - 1 : index + 2], 1e-30))
        denominator = left - 2.0 * center + right
        if denominator != 0:
            offset = float(0.5 * (left - right) / denominator)
    return (index + offset) * recording.rate / size


def zero_crossing_frequency(recording: Recording) -> float:
    samples = recording.samples - float(np.mean(recording.samples))
    crossings = np.flatnonzero((samples[:-1] <= 0) & (samples[1:] > 0))
    if len(crossings) < 4:
        return math.nan
    positions = []
    for index in crossings:
        left, right = samples[index], samples[index + 1]
        fraction = 0.0 if right == left else -left / (right - left)
        positions.append(float(index) + float(fraction))
    periods = np.diff(np.asarray(positions))
    median = float(np.median(periods))
    periods = periods[(periods > median * 0.75) & (periods < median * 1.25)]
    if len(periods) < 2:
        return math.nan
    return recording.rate / float(np.median(periods))


def tonal_frequency(recording: Recording, expected: float, waveform: str) -> float:
    # For the lowest primitive-wave notes, interpolated crossings beat a
    # short-window FFT. Above 128 Hz their sample-quantized crossing position
    # can drift enough to contaminate the harmonic signature, so use the
    # padded spectral estimate. Complex waves can have several crossings per
    # cycle and therefore always use their fundamental's spectral peak.
    if waveform == "organ":
        # Organ's octave triangle is 2.5x stronger than its fundamental, so
        # normally its dominant second harmonic is the more stable short-note
        # clock. Near isolated FFT leakage nulls the fundamental is cleaner;
        # select whichever estimate is closer to the authored note.
        fundamental = spectral_frequency(recording, expected)
        octave = spectral_frequency(recording, expected * 2.0) / 2.0
        return min((fundamental, octave), key=lambda value: abs(cents(value, expected)))
    if waveform == "phaser" and expected < 300.0:
        # A short phaser note contains two fundamentals less than a few hertz
        # apart. Their overlapping FFT lobes move with capture length even
        # when the waveforms agree; interpolated crossings remain stable until
        # high-pitch sample quantization takes over.
        return zero_crossing_frequency(recording)
    if expected < 128.0 and waveform in {
        "triangle",
        "tilted-saw",
        "saw",
        "square",
        "pulse",
    }:
        return zero_crossing_frequency(recording)
    return spectral_frequency(recording, expected)


def harmonic_signature(recording: Recording, frequency: float, harmonics: int) -> np.ndarray:
    samples = recording.samples - float(np.mean(recording.samples))
    if len(samples) < 8 or not math.isfinite(frequency):
        return np.zeros(harmonics)
    window = np.hanning(len(samples))
    times = np.arange(len(samples), dtype=np.float64) / recording.rate
    values = np.array(
        [
            abs(np.sum(samples * window * np.exp(-2j * np.pi * frequency * order * times)))
            for order in range(1, harmonics + 1)
        ]
    )
    norm = float(np.linalg.norm(values))
    return values / norm if norm else values


def noise_signature(recording: Recording) -> tuple[float, np.ndarray]:
    """Return physical spectral centroid and normalized log-band energy."""
    samples = recording.samples - float(np.mean(recording.samples))
    if len(samples) < 8:
        return math.nan, np.zeros(24)
    power = np.abs(np.fft.rfft(samples * np.hanning(len(samples)))) ** 2
    frequencies = np.fft.rfftfreq(len(samples), d=1.0 / recording.rate)
    common_limit = min(7372.0, recording.rate / 2.0)
    audible = (frequencies >= 20.0) & (frequencies <= common_limit)
    total = float(np.sum(power[audible]))
    centroid = (
        float(np.sum(frequencies[audible] * power[audible]) / total)
        if total
        else math.nan
    )
    edges = np.geomspace(20.0, common_limit, 25)
    bands = np.array(
        [
            np.sum(power[(frequencies >= edges[index]) & (frequencies < edges[index + 1])])
            for index in range(24)
        ]
    )
    # Square-root power prevents one broad stochastic peak from dominating
    # the comparison while retaining the pitch-dependent spectral slope.
    bands = np.sqrt(bands)
    norm = float(np.linalg.norm(bands))
    return centroid, bands / norm if norm else bands


def cents(left: float, right: float) -> float:
    if left <= 0 or right <= 0 or not math.isfinite(left) or not math.isfinite(right):
        return math.nan
    return 1200.0 * math.log2(left / right)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    pico_source = parser.add_mutually_exclusive_group(required=True)
    pico_source.add_argument("--pico", type=Path)
    pico_source.add_argument(
        "--pico-dir",
        type=Path,
        help="directory containing pitch-NN/pico-notes.wav isolated captures",
    )
    parser.add_argument("--mgba", type=Path, required=True)
    parser.add_argument("--max-pitch-cents", type=float, default=15.0)
    parser.add_argument("--min-timbre-correlation", type=float, default=0.95)
    parser.add_argument("--max-gain-error", type=float, default=0.03)
    parser.add_argument("--max-noise-centroid-cents", type=float, default=450.0)
    parser.add_argument("--min-noise-spectral-correlation", type=float, default=0.96)
    parser.add_argument("--max-noise-gain-error", type=float, default=0.15)
    parser.add_argument("--json", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    pitches = [int(pitch) for pitch in manifest["pitches"]]
    filters = manifest.get("filters", {})
    # Detuned oscillators can cancel below the active threshold, while reverb
    # tails cross zero between captured echoes. The generated fixture leaves
    # about 67 ms between speed-16 notes, so a 40 ms bridge preserves case
    # boundaries while joining either kind of short null.
    bridge_seconds = (
        0.040
        if int(filters.get("detune", 0)) or int(filters.get("reverb", 0))
        else 0.012
    )
    mgba = read_wav(args.mgba)
    mgba_runs = active_runs(mgba, bridge_seconds)
    if len(mgba_runs) != len(pitches):
        raise ValueError(
            f"mGBA active-run count is {len(mgba_runs)}, expected {len(pitches)}"
        )
    if args.pico_dir is not None:
        pico_notes = [
            trim_active(
                read_wav(args.pico_dir / f"pitch-{pitch:02d}" / "pico-notes.wav"),
                bridge_seconds,
            )
            for pitch in pitches
        ]
    else:
        pico_capture = read_wav(args.pico)
        pico_runs = active_runs(pico_capture, bridge_seconds)
        if len(pico_runs) != len(pitches):
            raise ValueError(
                f"PICO-8 active-run count is {len(pico_runs)}, expected {len(pitches)}"
            )
        pico_notes = [
            Recording(
                pico_capture.rate,
                pico_capture.samples[start:end],
            )
            for start, end in pico_runs
        ]

    rows = []
    failures = 0
    waveform = str(manifest["waveform"])
    for pitch, pico, gba_run in zip(pitches, pico_notes, mgba_runs, strict=True):
        gba = Recording(mgba.rate, mgba.samples[gba_run[0] : gba_run[1]])
        expected_hz = PITCH_ZERO_HZ * 2.0 ** (pitch / 12.0)
        if waveform == "noise":
            pico_hz, pico_signature = noise_signature(pico)
            gba_hz, gba_signature = noise_signature(gba)
            expected_pico = math.nan
            expected_gba = math.nan
        else:
            pico_hz = tonal_frequency(pico, expected_hz, waveform)
            gba_hz = tonal_frequency(gba, expected_hz, waveform)
            expected_pico = cents(pico_hz, expected_hz)
            expected_gba = cents(gba_hz, expected_hz)
            harmonic_count = max(1, min(16, int(7372.0 // expected_hz)))
            pico_signature = harmonic_signature(pico, pico_hz, harmonic_count)
            gba_signature = harmonic_signature(gba, gba_hz, harmonic_count)
        pitch_delta = cents(gba_hz, pico_hz)
        timbre = float(np.dot(pico_signature, gba_signature))
        pico_rms = float(np.sqrt(np.mean(pico.samples**2)))
        gba_rms = float(np.sqrt(np.mean(gba.samples**2)))
        amplitude_ratio = gba_rms / pico_rms if pico_rms else math.nan
        if waveform == "noise":
            passed = (
                math.isfinite(pitch_delta)
                and abs(pitch_delta) <= args.max_noise_centroid_cents
                and timbre >= args.min_noise_spectral_correlation
                and math.isfinite(amplitude_ratio)
                and abs(amplitude_ratio - 1.0) <= args.max_noise_gain_error
            )
        else:
            passed = (
                math.isfinite(pitch_delta)
                and abs(pitch_delta) <= args.max_pitch_cents
                and timbre >= args.min_timbre_correlation
                and math.isfinite(amplitude_ratio)
                and abs(amplitude_ratio - 1.0) <= args.max_gain_error
            )
        if not passed:
            failures += 1
        rows.append(
            {
                "pitch": pitch,
                "expectedHz": expected_hz,
                "picoHz": pico_hz,
                "mtileHz": gba_hz,
                "picoExpectedCents": expected_pico,
                "mtileExpectedCents": expected_gba,
                "mtileVsPicoCents": pitch_delta,
                "timbreCorrelation": timbre,
                "picoRMS": pico_rms,
                "mtileRMS": gba_rms,
                "mtileVsPicoAmplitude": amplitude_ratio,
                "picoDurationMS": len(pico.samples) * 1000.0 / pico.rate,
                "mtileDurationMS": len(gba.samples) * 1000.0 / gba.rate,
                "pass": passed,
            }
        )
        metric = "centroid" if waveform == "noise" else "pitch"
        print(
            f"{metric} {pitch:02d}  pico={pico_hz:9.3f}Hz  gba={gba_hz:9.3f}Hz  "
            f"delta={pitch_delta:+7.2f}c  timbre={timbre:.5f}  "
            f"{'PASS' if passed else 'FAIL'}"
        )

    finite_deltas = [abs(row["mtileVsPicoCents"]) for row in rows if math.isfinite(row["mtileVsPicoCents"])]
    timbres = [row["timbreCorrelation"] for row in rows]
    amplitude_ratios = [row["mtileVsPicoAmplitude"] for row in rows]
    pico_durations = [row["picoDurationMS"] for row in rows]
    gba_durations = [row["mtileDurationMS"] for row in rows]
    result = {
        "version": 1,
        "metric": "spectral-centroid" if waveform == "noise" else "fundamental-pitch",
        "fixture": manifest,
        "cases": rows,
        "summary": {
            "cases": len(rows),
            "failures": failures,
            "maxAbsolutePitchCents": max(finite_deltas, default=math.nan),
            "minTimbreCorrelation": min(timbres, default=math.nan),
            "medianAmplitudeRatio": float(np.median(amplitude_ratios)),
            "medianPICODurationMS": float(np.median(pico_durations)),
            "medianMtileDurationMS": float(np.median(gba_durations)),
        },
    }
    if args.json:
        args.json.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    metric_name = "centroid" if waveform == "noise" else "pitch"
    print(
        f"summary: {len(rows) - failures}/{len(rows)} pass, "
        f"max {metric_name} delta={result['summary']['maxAbsolutePitchCents']:.2f}c, "
        f"min timbre={result['summary']['minTimbreCorrelation']:.5f}, "
        f"median gain={result['summary']['medianAmplitudeRatio']:.4f}x, "
        f"duration={result['summary']['medianPICODurationMS']:.3f}/"
        f"{result['summary']['medianMtileDurationMS']:.3f} ms PICO/Mtile"
    )
    if failures:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
