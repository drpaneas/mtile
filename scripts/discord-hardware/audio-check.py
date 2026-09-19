#!/usr/bin/env python3
"""Capture the 42 PSG reference cases with the pinned headless mGBA library."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    args = parser.parse_args()
    out = args.output_dir.resolve()
    out.mkdir(parents=True, exist_ok=False)
    stub = out / "main.c"
    stub.write_text("int main(void) { return 2; }\n")
    library = ROOT / "build/player/mgba/libmgba-core.a"
    runner = out / "capture"
    command = ["cc", "-std=c11", "-O2", "-Wall", "-Wextra",
               "-I" + str(ROOT / "third_party/mgba/include"),
               "-I" + str(ROOT / "build/player/mgba/include"),
               str(ROOT / "scripts/audio/mgba_audio_capture.c"), str(stub),
               str(library), "-lm", "-pthread", "-o", str(runner)]
    if platform.system() == "Darwin":
        command += ["-framework", "Foundation"]
    subprocess.run(command, check=True)
    report = {"environment": "native mGBA; emulator-only audio validation",
              "mgba_sha256": hashlib.sha256(library.read_bytes()).hexdigest(),
              "cases": {}}
    for kind, count in [("ch1", 12), ("ch2", 10), ("ch3", 10), ("ch4", 10)]:
        rom = args.bundle.resolve() / ("audioref-" + kind + ".gba")
        wav = out / (kind + ".wav")
        env = dict(os.environ, MGBA_CAPTURE_ROM=str(rom),
                   MGBA_CAPTURE_OUTPUT=str(wav), MGBA_CAPTURE_FRAMES=str(count * 120))
        capture = subprocess.run([str(runner)], env=env, text=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (out / (kind + "-capture.log")).write_text(capture.stdout)
        capture.check_returncode()
        analysis = subprocess.run([sys.executable,
                                   str(ROOT / "scripts/audio/analyze_gba_reference_capture.py"),
                                   "--kind", kind, "--input", str(wav)], text=True,
                                  stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (out / (kind + "-analysis.log")).write_text(analysis.stdout)
        print(analysis.stdout, end="")
        analysis.check_returncode()
        report["cases"][kind] = {"count": count, "pass": True,
                                  "rom_sha256": hashlib.sha256(rom.read_bytes()).hexdigest(),
                                  "wav_sha256": hashlib.sha256(wav.read_bytes()).hexdigest()}
    (out / "report.json").write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
