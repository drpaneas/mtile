#!/usr/bin/env python3
"""Freeze the fixed Go-prefix corpus and source before any PGO profiling."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil

p = argparse.ArgumentParser()
p.add_argument("snapshot", type=Path)
p.add_argument("evidence", type=Path)
args = p.parse_args()
root = Path(__file__).resolve().parents[3]
snapshot, evidence = args.snapshot.resolve(), args.evidence.resolve()
if snapshot.exists():
    raise SystemExit("Choose a new snapshot directory; existing evidence is never overwritten")
corpus = {"train": ["celeste", "multi-file", "audio-gb-song"],
          "heldout": ["platformer", "pong", "mapdemo"], "controls": ["hello", "noalloc-call-stress"]}
for names in corpus.values():
    for name in names:
        if not (root / "examples" / name).is_dir():
            raise SystemExit(f"Missing frozen-corpus source: examples/{name}; do not substitute another project silently")
snapshot.mkdir(parents=True)
evidence.mkdir(parents=True, exist_ok=True)
digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
source_manifest = {}
for name in ["go.mod", "go.sum", "internal", "cmd", "runtime"]:
    src, dst = root / name, snapshot / "pre8-source" / name
    if src.is_file():
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dst)
        source_manifest[name] = digest(src)
    else:
        shutil.copytree(src, dst)
        source_manifest.update({str(f.relative_to(root)): digest(f) for f in src.rglob("*") if f.is_file()})
inputs = {}
for role, names in corpus.items():
    for name in names:
        src, dst = root / "examples" / name, snapshot / "inputs" / name
        shutil.copytree(src, dst)
        files = [f for f in dst.rglob("*") if f.is_file()]
        inputs[name] = {"role": role, "sourcePath": str(src), "snapshotPath": str(dst),
                        "fileCount": len(files), "totalBytes": sum(f.stat().st_size for f in files),
                        "files": {str(f.relative_to(dst)): digest(f) for f in files}}
for name, data in [("corpus.json", corpus), ("input-manifest.json", inputs), ("pre8-source-manifest.json", source_manifest)]:
    (evidence / name).write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")
print(snapshot)
