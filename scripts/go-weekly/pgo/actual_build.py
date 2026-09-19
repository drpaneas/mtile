#!/usr/bin/env python3
"""Six paired deployment-path checks, reusing the one unchanged PGO profile."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import statistics
import subprocess

p = argparse.ArgumentParser()
p.add_argument("snapshot", type=Path)
p.add_argument("evidence", type=Path)
args = p.parse_args()
snapshot, evidence = args.snapshot.resolve(), args.evidence.resolve()
repository = Path(__file__).resolve().parents[3]
source = snapshot / "actual-source"
if source.exists():
    raise SystemExit("Do not repeat deployment validation or overwrite prior evidence")
shutil.copytree(snapshot / "source", source)
driver = Path(__file__).with_name("actual_build_test.go.in")
shutil.copy2(driver, source / "internal/build/pgo_actual_benchmark_test.go")
profile = snapshot / "training.pprof"
digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
expected = json.loads((evidence / "metadata.json").read_text())
assert digest(profile) == expected["profileSHA256"]
for name, info in expected["binaries"].items():
    assert digest(Path(info["path"])) == info["sha256"], "original binary changed"
env = {**os.environ, "GOCACHE": "/tmp/mtile-generated-gocache", "GOWORK": "off",
       "MTILE_PGO_INPUT_ROOT": str(snapshot / "inputs"), "MTILE_EXPERIMENTAL_TILECANVAS_PROJECT": "",
       "MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS": ""}
assert os.environ.get("GOMAXPROCS") == expected["gomaxprocsEnvironment"]
samples = []
binaries = {}


def run(command, log, cwd=source):
    with (evidence / log).open("w") as f:
        f.write("$ " + " ".join(map(str, command)) + "\n")
        f.flush()
        result = subprocess.run(list(map(str, command)), cwd=cwd, env=env,
                                stdout=f, stderr=subprocess.STDOUT, timeout=180)
        f.write(f"\nexit_code={result.returncode}\n")
    if result.returncode:
        raise RuntimeError(log)
    return (evidence / log).read_text()


for name, setting in [("off", "off"), ("trained", str(profile))]:
    binary = snapshot / ("actual-" + name + ".test")
    run(["go", "test", "-c", "-pgo=" + setting, "-o", binary, "./internal/build"], f"actual-compile-{name}.log")
    binaries[name] = binary
metadata = {"pairs": 6, "gomaxprocs": expected["gomaxprocs"], "profileSHA256": digest(profile),
            "driverSHA256": digest(driver), "productionMethod": "build.Service.Build",
            "stopBefore": "PhaseWorkspace", "harnessOverhead": "Trace callback plus one panic/recover included identically",
            "thresholds": expected["thresholds"],
            "binaries": {name: {"path": str(binary), "sha256": digest(binary), "bytes": binary.stat().st_size}
                         for name, binary in binaries.items()}}
(evidence / "actual-metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
bench = ["-test.run=^$", "-test.bench=^BenchmarkPGOActual(Heldout|Hello|Noalloc)$", "-test.benchmem", "-test.count=1"]
for name, binary in binaries.items():
    run([binary, *bench, "-test.benchtime=100ms"], f"actual-warm-{name}.log", source / "internal/build")
pattern = re.compile(r"^BenchmarkPGOActual(Heldout|Hello|Noalloc)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$", re.M)
for pair in range(6):
    order = ["off", "trained"] if pair % 2 == 0 else ["trained", "off"]
    for name in order:
        text = run([binaries[name], *bench, "-test.benchtime=1s"], f"actual-pair-{pair+1:02}-{name}.log", source / "internal/build")
        rows = pattern.findall(text)
        assert len(rows) == 3, text
        for case, iterations, ns, byte_count, allocs in rows:
            samples.append({"pair": pair+1, "variant": name, "case": case, "iterations": int(iterations),
                            "ns/op": float(ns), "B/op": int(byte_count), "allocs/op": int(allocs)})
    (evidence / "actual-samples.json").write_text(json.dumps(samples, indent=2) + "\n")
    print(f"Actual Build pair {pair+1}/6 complete", flush=True)
summary = {}
for case in ["Heldout", "Hello", "Noalloc"]:
    variants = {}
    for name in binaries:
        rows = [s for s in samples if s["case"] == case and s["variant"] == name]
        values = [s["ns/op"] for s in rows]
        variants[name] = {"median_ns/op": statistics.median(values), "min_ns/op": min(values), "max_ns/op": max(values),
                          "cvPercent": 100*statistics.stdev(values)/statistics.mean(values),
                          "median_B/op": statistics.median(s["B/op"] for s in rows),
                          "median_allocs/op": statistics.median(s["allocs/op"] for s in rows)}
    change = 100*(variants["trained"]["median_ns/op"]/variants["off"]["median_ns/op"]-1)
    pairs = []
    for pair in range(1, 7):
        values = {s["variant"]: s["ns/op"] for s in samples if s["case"] == case and s["pair"] == pair}
        pairs.append(100*(values["trained"]/values["off"]-1))
    summary[case] = {**variants, "medianChangePercent": change, "pairedChangesPercent": pairs,
                     "improvedPairs": sum(v < 0 for v in pairs), "gatePassed": change <= (-3 if case == "Heldout" else 2)}
summary["performanceGatePassed"] = all(summary[c]["gatePassed"] for c in ["Heldout", "Hello", "Noalloc"])
assert digest(profile) == expected["profileSHA256"]
(evidence / "actual-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2), flush=True)
