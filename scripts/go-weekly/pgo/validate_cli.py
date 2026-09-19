#!/usr/bin/env python3
"""Validate default cmd/mtile PGO deployment; this records no timing claims."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

p = argparse.ArgumentParser()
p.add_argument("snapshot", type=Path)
p.add_argument("evidence", type=Path)
args = p.parse_args()
root = Path(__file__).resolve().parents[3]
snapshot, evidence = args.snapshot.resolve(), args.evidence.resolve()
out = snapshot / "cli"
out.mkdir(exist_ok=True)
env = {**os.environ, "GOCACHE": "/tmp/mtile-generated-gocache", "GOWORK": "off",
       "MTILE_EXPERIMENTAL_TILECANVAS_PROJECT": "", "MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS": ""}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(command, name):
    result = subprocess.run(list(map(str, command)), cwd=root, env=env,
                            text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=240)
    (evidence / name).write_text("$ " + " ".join(map(str, command)) + "\n" + result.stdout + f"\nexit_code={result.returncode}\n")
    if result.returncode:
        raise RuntimeError(name)
    return result.stdout


report = {"binaries": {}, "projects": {}, "defaultProfileSHA256": digest(root / "cmd/mtile/default.pgo")}
for variant, flags in [("off", ["-pgo=off"]), ("default", [])]:
    binary = out / ("mtile-" + variant)
    run(["go", "build", "-x", *flags, "-o", binary, "./cmd/mtile"], f"cli-compile-{variant}.log")
    info = run(["go", "version", "-m", binary], f"cli-build-info-{variant}.log")
    report["binaries"][variant] = {"bytes": binary.stat().st_size, "sha256": digest(binary), "buildInfo": info}
for name in ["hello", "pong", "mapdemo"]:
    variants = {}
    for variant in ["off", "default"]:
        rom = out / f"{name}-{variant}.gba"
        text = run([out / ("mtile-" + variant), "build", "-runtime", root / "runtime",
                    "-keep-work", "-o", rom, snapshot / "inputs" / name], f"cli-{name}-{variant}.log")
        match = re.search(r"^workdir (.+)$", text, re.M)
        if not match:
            raise RuntimeError("CLI omitted retained workspace")
        work = Path(match.group(1))
        variants[variant] = {"ROMBytes": rom.stat().st_size, "ROMSHA256": digest(rom),
                             "generatedGoSHA256": digest(work / "main.go"), "workDir": str(work)}
    assert variants["off"]["ROMSHA256"] == variants["default"]["ROMSHA256"], name + " ROM changed"
    assert variants["off"]["generatedGoSHA256"] == variants["default"]["generatedGoSHA256"], name + " generated Go changed"
    report["projects"][name] = variants
    print(name + " actual CLI generated Go and ROM identical", flush=True)
    (evidence / "cli-validation.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps(report["binaries"], indent=2), flush=True)
