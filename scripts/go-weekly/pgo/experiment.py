#!/usr/bin/env python3
"""One frozen training profile and ten alternating Go-prefix sample pairs.

Requires the pre8-source and inputs snapshots documented in the evidence README.
The run refuses to overwrite an existing profile; it never tunes to held-out data.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import statistics
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument("snapshot", type=Path)
parser.add_argument("evidence", type=Path)
args = parser.parse_args()
snapshot, evidence = args.snapshot.resolve(), args.evidence.resolve()
repository = Path(__file__).resolve().parents[3]
source = snapshot / "source"
profile = snapshot / "training.pprof"
if source.exists() or profile.exists():
    raise SystemExit("A source/profile snapshot already exists; do not rerun or tune this experiment")
shutil.copytree(snapshot / "pre8-source", source)
shutil.copy2(repository / "internal/build/pgo_benchmark_test.go", source / "internal/build/pgo_benchmark_test.go")
evidence.mkdir(parents=True, exist_ok=True)
env = {**os.environ, "GOCACHE": "/tmp/mtile-generated-gocache", "GOWORK": "off",
       "MTILE_PGO_INPUT_ROOT": str(snapshot / "inputs"),
       "MTILE_EXPERIMENTAL_TILECANVAS_PROJECT": "", "MTILE_EXPERIMENTAL_TILECANVAS_OBJECTS": ""}
env.pop("MTILE_PGO_PROFILE", None)
env.pop("MTILE_PGO_VERIFY_BUILD", None)
env.pop("MTILE_PGO_MANIFEST", None)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(command, log, extra=None, cwd=source, timeout=180):
    with (evidence / log).open("w") as output:
        output.write("$ " + " ".join(map(str, command)) + "\n")
        output.flush()
        result = subprocess.run(list(map(str, command)), cwd=cwd, env={**env, **(extra or {})},
                                stdout=output, stderr=subprocess.STDOUT, timeout=timeout)
        output.write(f"\nexit_code={result.returncode}\n")
    if result.returncode:
        raise RuntimeError(f"Failed {log}; no later stage executed")
    return (evidence / log).read_text()


manifest = {str(p.relative_to(source)): digest(p) for p in source.rglob("*") if p.is_file()}
(evidence / "measured-source-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
metadata = {"goVersion": subprocess.check_output(["go", "version"], text=True).strip(),
            "goEnv": json.loads(subprocess.check_output(["go", "env", "-json", "GOARCH", "GOOS", "GOVERSION", "GOAMD64", "GOARM64"], text=True)),
            "gomaxprocsEnvironment": os.environ.get("GOMAXPROCS"), "benchtime": "1s", "pairs": 10, "trainingSeconds": 30,
            "source": str(source), "inputs": str(snapshot / "inputs"),
            "thresholds": {"heldoutImprovementPercent": 3, "eachControlMaxSlowdownPercent": 2}}
(evidence / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
off, trained = snapshot / "off.test", snapshot / "trained.test"
run(["go", "test", "-c", "-pgo=off", "-o", off, "./internal/build"], "compile-off.log")
run([off, "-test.run=^TestPGOPreparationManifest$", "-test.v"], "off-manifest.log",
    {"MTILE_PGO_VERIFY_BUILD": "1", "MTILE_PGO_MANIFEST": str(evidence / "off-manifest.json")}, cwd=source / "internal/build")
cpu_match = re.search(r"GOMAXPROCS=(\d+) NumCPU=(\d+)", (evidence / "off-manifest.log").read_text())
if not cpu_match:
    raise RuntimeError("Missing runtime CPU settings")
metadata["gomaxprocs"], metadata["numCPU"] = map(int, cpu_match.groups())
print("Actual Build/helper parity passed for all frozen projects; collecting the one 30-second profile", flush=True)
run([off, "-test.run=^TestPGOTraining$", "-test.v"], "training.log",
    {"MTILE_PGO_PROFILE": str(profile)}, cwd=source / "internal/build", timeout=90)
shutil.copy2(profile, evidence / "training.pprof")
run(["go", "test", "-c", "-pgo=" + str(profile), "-o", trained, "./internal/build"], "compile-trained.log")
run([trained, "-test.run=^TestPGOPreparationManifest$", "-test.v"], "trained-manifest.log",
    {"MTILE_PGO_VERIFY_BUILD": "1", "MTILE_PGO_MANIFEST": str(evidence / "trained-manifest.json")}, cwd=source / "internal/build")
if (evidence / "off-manifest.json").read_bytes() != (evidence / "trained-manifest.json").read_bytes():
    raise RuntimeError("Output manifests differ; stop before performance comparison")
metadata["outputManifestsEqual"] = True
metadata["profileSHA256"] = digest(profile)
metadata["binaries"] = {name: {"path": str(path), "bytes": path.stat().st_size, "sha256": digest(path)}
                        for name, path in [("off", off), ("trained", trained)]}
(evidence / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
print("Exact output manifests match; warming both binaries before paired evaluation", flush=True)
bench = ["-test.run=^$", "-test.bench=^BenchmarkPGO(Heldout|Hello|Noalloc)$", "-test.benchmem", "-test.count=1"]
for name, binary in [("off", off), ("trained", trained)]:
    run([binary, *bench, "-test.benchtime=100ms"], f"warm-{name}.log", cwd=source / "internal/build")
samples = []
pattern = re.compile(r"^BenchmarkPGO(Heldout|Hello|Noalloc)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$", re.M)
for pair in range(10):
    order = [("off", off), ("trained", trained)] if pair % 2 == 0 else [("trained", trained), ("off", off)]
    for name, binary in order:
        text = run([binary, *bench, "-test.benchtime=1s"], f"pair-{pair+1:02}-{name}.log", cwd=source / "internal/build")
        rows = pattern.findall(text)
        if len(rows) != 3:
            raise RuntimeError(f"Incomplete benchmark output in pair {pair+1} {name}")
        for case, iterations, ns, byte_count, allocs in rows:
            samples.append({"pair": pair+1, "variant": name, "case": case, "iterations": int(iterations),
                            "ns/op": float(ns), "B/op": int(byte_count), "allocs/op": int(allocs)})
    (evidence / "samples.json").write_text(json.dumps(samples, indent=2) + "\n")
    print(f"Paired sample {pair+1}/10 complete", flush=True)
summary = {}
for case in ["Heldout", "Hello", "Noalloc"]:
    variants = {}
    for name in ["off", "trained"]:
        rows = [s for s in samples if s["case"] == case and s["variant"] == name]
        values = [s["ns/op"] for s in rows]
        variants[name] = {"median_ns/op": statistics.median(values), "min_ns/op": min(values), "max_ns/op": max(values),
                          "cvPercent": 100 * statistics.stdev(values) / statistics.mean(values),
                          "median_B/op": statistics.median(s["B/op"] for s in rows),
                          "median_allocs/op": statistics.median(s["allocs/op"] for s in rows)}
    change = 100 * (variants["trained"]["median_ns/op"] / variants["off"]["median_ns/op"] - 1)
    pair_changes = []
    for pair in range(1, 11):
        values = {s["variant"]: s["ns/op"] for s in samples if s["case"] == case and s["pair"] == pair}
        pair_changes.append(100 * (values["trained"] / values["off"] - 1))
    summary[case] = {**variants, "medianChangePercent": change, "pairedChangesPercent": pair_changes,
                     "improvedPairs": sum(v < 0 for v in pair_changes),
                     "gatePassed": change <= (-3 if case == "Heldout" else 2)}
summary["performanceGatePassed"] = all(summary[c]["gatePassed"] for c in ["Heldout", "Hello", "Noalloc"])
(evidence / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
assert manifest == {str(p.relative_to(source)): digest(p) for p in source.rglob("*") if p.is_file()}, "measured source changed"
print(json.dumps(summary, indent=2), flush=True)
