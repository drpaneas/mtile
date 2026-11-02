#!/usr/bin/env python3
"""Recheck retained PCM evidence without a GBA toolchain."""
import csv
import gzip
import io
import json
import pathlib
import statistics

ROOT = pathlib.Path(__file__).resolve().parent
count = 0
for group, expected in (("raw", 19), ("prepared", 6)):
    rows = json.loads((ROOT / group / "results.json").read_text())
    assert len(rows) == expected
    lookup = {(d["variant"], d["workload"]["name"]): d for d in rows}
    for d in rows:
        folder = ROOT / group / (d["variant"] + "-" + d["workload"]["name"])
        original = json.loads((folder / "result.json").read_text())
        assert original["profile"] == d["profile"]
        trace = gzip.decompress((folder / "measured.ticks.csv.gz").read_bytes()).decode()
        ticks = list(csv.DictReader(io.StringIO(trace)))
        assert len(ticks) == 300
        work = [int(t["instructions"]) - int(t["wait"]) for t in ticks]
        assert abs(statistics.mean(work) - d["profile"]["cpu"]["mean"]) < 1e-8
        for t in ticks:
            assert int(t["instructions"]) + int(t["activeEvents"]) + int(t["haltEvents"]) == int(t["elapsed"])
        m = d["profile"]["metadata"]
        assert m["instructionCycles"] + m["activeEventCycles"] + m["haltEventCycles"] == m["elapsedCycles"]
        assert m["maxObservedVoices"] == d["workload"]["voices"] + d["workload"]["effects"]
        if d["workload"]["name"] != "off":
            assert m["mixerCapacity"] == 32
        assert len(d["captures"]) == 2
        for c in d["captures"]:
            assert c["samples"] == 600 and c["refreshGaps"] == {"2": 599}
            assert c["pcm"]["overflows"] == c["pcm"]["clippedFrames"] == 0
            if d["variant"] == "mtile":
                assert all(c[k] == 0 for k in ("heapCapacity", "heapAllocated", "heapLive", "heapPeak", "gcCollections", "heapError", "missedDelta"))
        if d["variant"] == "mtile" and d["workload"]["name"] != "off":
            peer = lookup["butano-matched", d["workload"]["name"]]
            assert d["bankSHA256"] == peer["bankSHA256"]
            assert d["bankBytes"] == peer["bankBytes"]
        count += 1
print(f"PASS: {count} configurations; cycle accounting, matched banks/voices, cadence, heap/GC and PCM telemetry")
