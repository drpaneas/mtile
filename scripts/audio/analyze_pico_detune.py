#!/usr/bin/env python3
"""Compare both oscillator fundamentals in one PICO-8/Mtile detune capture."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

import numpy as np

from analyze_pico_sfx_notes import Recording, active_runs, read_wav


PITCH_ZERO_HZ = 65.406391


def active_note(path: Path) -> Recording:
    recording = read_wav(path)
    runs = active_runs(recording, 0.040)
    if len(runs) != 1:
        raise ValueError(f"{path}: found {len(runs)} active notes, expected 1")
    start, end = runs[0]
    pad = min(round(recording.rate * 0.050), (end - start) // 8)
    return Recording(recording.rate, recording.samples[start + pad : end - pad])


def spectrum(recording: Recording) -> tuple[np.ndarray, np.ndarray]:
    samples = recording.samples - float(np.mean(recording.samples))
    size = 1 << (max(65536, len(samples) * 64) - 1).bit_length()
    values = np.abs(np.fft.rfft(samples * np.hanning(len(samples)), n=size))
    frequencies = np.fft.rfftfreq(size, 1.0 / recording.rate)
    return frequencies, values


def strongest(
    frequencies: np.ndarray, values: np.ndarray, low: float, high: float
) -> tuple[float, float]:
    candidates = np.flatnonzero((frequencies >= low) & (frequencies <= high))
    if len(candidates) == 0:
        return math.nan, math.nan
    index = int(candidates[np.argmax(values[candidates])])
    return float(frequencies[index]), float(values[index])


def cents(left: float, right: float) -> float:
    return 1200.0 * math.log2(left / right)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pico", type=Path, required=True)
    parser.add_argument("--mgba", type=Path, required=True)
    parser.add_argument("--waveform", required=True)
    parser.add_argument("--pitch", type=int, required=True)
    parser.add_argument("--level", type=int, choices=(1, 2), required=True)
    parser.add_argument("--json", type=Path)
    return parser.parse_args()


def secondary_range(waveform: str, level: int) -> tuple[float, float]:
    if waveform == "triangle":
        return (0.70, 0.80) if level == 1 else (1.45, 1.55)
    if waveform == "organ" and level == 2:
        return 3.90, 4.05
    if waveform == "phaser" and level == 1:
        return 0.94, 0.99
    return (0.98, 1.00) if level == 1 else (1.90, 2.05)


def measure(recording: Recording, expected: float, ratio_range: tuple[float, float]) -> dict[str, float]:
    frequencies, values = spectrum(recording)
    primary_hz, primary_amplitude = strongest(
        frequencies, values, expected * 0.985, expected * 1.015
    )
    secondary_hz, secondary_amplitude = strongest(
        frequencies,
        values,
        expected * ratio_range[0],
        expected * ratio_range[1],
    )
    return {
        "primaryHz": primary_hz,
        "secondaryHz": secondary_hz,
        "frequencyRatio": secondary_hz / primary_hz,
        "amplitudeRatio": secondary_amplitude / primary_amplitude,
        "rms": float(np.sqrt(np.mean(recording.samples**2))),
    }


def main() -> None:
    args = parse_args()
    if args.pitch < 0 or args.pitch > 63:
        raise ValueError("--pitch must be in 0..63")
    expected = PITCH_ZERO_HZ * 2.0 ** (args.pitch / 12.0)
    ratio_range = secondary_range(args.waveform, args.level)
    pico = active_note(args.pico)
    gba = active_note(args.mgba)
    pico_result = measure(pico, expected, ratio_range)
    gba_result = measure(gba, expected, ratio_range)
    primary_delta = cents(gba_result["primaryHz"], pico_result["primaryHz"])
    secondary_delta = cents(gba_result["secondaryHz"], pico_result["secondaryHz"])
    amplitude_error = gba_result["amplitudeRatio"] / pico_result["amplitudeRatio"] - 1.0
    gain = gba_result["rms"] / pico_result["rms"]
    passed = (
        abs(primary_delta) <= 15.0
        and abs(secondary_delta) <= 15.0
        and abs(amplitude_error) <= 0.05
        and abs(gain - 1.0) <= 0.03
    )
    result = {
        "version": 1,
        "waveform": args.waveform,
        "pitch": args.pitch,
        "level": args.level,
        "pico": pico_result,
        "mtile": gba_result,
        "primaryDeltaCents": primary_delta,
        "secondaryDeltaCents": secondary_delta,
        "secondaryAmplitudeError": amplitude_error,
        "mtileVsPicoAmplitude": gain,
        "pass": passed,
    }
    print(
        f"detune-{args.level} {args.waveform} pitch {args.pitch:02d}: "
        f"primary {primary_delta:+.2f}c, secondary {secondary_delta:+.2f}c, "
        f"secondary gain {1.0 + amplitude_error:.4f}x, total gain {gain:.4f}x  "
        f"{'PASS' if passed else 'FAIL'}"
    )
    if args.json:
        args.json.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    raise SystemExit(0 if passed else 1)


if __name__ == "__main__":
    main()
