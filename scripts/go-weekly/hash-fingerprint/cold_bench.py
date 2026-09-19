#!/usr/bin/env python3
"""Compare fresh insertion/image construction in two frozen runtime trees."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import statistics
import subprocess
import tempfile

p = argparse.ArgumentParser()
p.add_argument("baseline", type=Path)
p.add_argument("candidate", type=Path)
p.add_argument("output", type=Path)
args = p.parse_args()
args.output.mkdir(parents=True, exist_ok=True)
samples = {"baseline": {}, "candidate": {}}
with tempfile.TemporaryDirectory(prefix="mtile-hash-cold-") as tmp:
    trees = {}
    for name, source in [("baseline", args.baseline), ("candidate", args.candidate)]:
        trees[name] = Path(tmp) / name
        shutil.copytree(source, trees[name])
        shutil.copy2(args.candidate / "hash_fingerprint_test.go", trees[name] / "hash_fingerprint_test.go")
        (args.output / f"cold-{name}.log").write_text("")
    for pair in range(5):
        for name in (["baseline", "candidate"] if pair % 2 == 0 else ["candidate", "baseline"]):
            command = ["go", "test", ".", "-run", "^$", "-bench", "^BenchmarkHashFingerprintCold$", "-benchtime=300ms", "-count=1", "-cpu=1"]
            result = subprocess.run(command, cwd=trees[name], text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.STDOUT, timeout=180,
                                    env={**os.environ, "GOCACHE": str(Path(tempfile.gettempdir()) / "mtile-generated-gocache")})
            with (args.output / f"cold-{name}.log").open("a") as log:
                log.write("$ " + " ".join(command) + "\n" + result.stdout)
            if result.returncode:
                raise RuntimeError(result.stdout)
            found = re.findall(r"BenchmarkHashFingerprintCold/(\S+)\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op", result.stdout)
            if len(found) != 4:
                raise RuntimeError(f"Missing benchmark data: {result.stdout}")
            for case, ns, byte_count, allocs in found:
                samples[name].setdefault(case, []).append({"ns/op": float(ns), "B/op": int(byte_count), "allocs/op": int(allocs)})
            print(f"pair {pair+1} {name} complete", flush=True)
summary = {}
for case in samples["baseline"]:
    before = statistics.median(x["ns/op"] for x in samples["baseline"][case])
    after = statistics.median(x["ns/op"] for x in samples["candidate"][case])
    summary[case] = {"baseline_ns/op": before, "candidate_ns/op": after, "changePercent": 100 * (after/before - 1)}
(args.output / "cold-samples.json").write_text(json.dumps(samples, indent=2) + "\n")
(args.output / "cold-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2))
