"""Compare the ROMs built from all candidate desktop archives before a release."""

import argparse
from collections import defaultdict
import json
from pathlib import Path
import re

from release_catalog import ATTEMPTS, EXAMPLES, SHARDS, TARGETS


def is_sha256(value):
    return isinstance(value, str) and re.fullmatch(r"[a-f0-9]{64}", value) is not None


def read_shard(path, target, shard):
    data = json.loads(path.read_text(encoding="utf-8"))
    if (not isinstance(data, dict) or data.get("target") != target or
            type(data.get("shard")) is not int or data["shard"] != shard or
            not isinstance(data.get("records"), list)):
        raise ValueError("invalid target, shard or records list")
    if data.get("problems") or data.get("error"):
        raise ValueError(f"catalog build failed: {data.get('problems') or data['error']}")
    if not is_sha256(data.get("archive_sha256")):
        raise ValueError("invalid archive SHA-256")

    expected = {(name, attempt) for name in EXAMPLES[shard::SHARDS]
                for attempt in range(1, ATTEMPTS + 1)}
    results = {}
    for index, record in enumerate(data["records"], 1):
        if not isinstance(record, dict):
            raise ValueError(f"invalid record {index}")
        name, attempt = record.get("example"), record.get("attempt")
        if not isinstance(name, str) or type(attempt) is not int or (name, attempt) not in expected:
            raise ValueError(f"unexpected record {index}: {name!r} attempt {attempt!r}")
        label = f"{name} on {target} attempt {attempt}"
        if (name, attempt) in results:
            raise ValueError(f"duplicate result for {label}")
        if "error" in record:
            raise ValueError(f"{label} failed: {record['error']}")
        size, digest = record.get("bytes"), record.get("sha256")
        if type(size) is not int or size <= 192 or not is_sha256(digest):
            raise ValueError(f"invalid ROM size or SHA-256 for {label}")
        results[name, attempt] = (size, digest)
    missing = sorted(expected - results.keys())
    if missing:
        raise ValueError("missing builds: " + ", ".join(f"{n} attempt {a}" for n, a in missing))
    return data["archive_sha256"], results


def compare(directory):
    expected = {f"{target}-{shard}.json": (target, shard)
                for target in TARGETS for shard in range(SHARDS)}
    seen = set()
    archives = {}
    results = defaultdict(list)
    issues = []
    for path in sorted(directory.rglob("*.json")):
        if path.name not in expected or path.name in seen:
            issues.append(f"Unexpected or duplicate report: {path.relative_to(directory)}")
            continue
        seen.add(path.name)
        target, shard = expected[path.name]
        try:
            archive, records = read_shard(path, target, shard)
        except (OSError, ValueError) as exc:
            issues.append(f"{path.name}: {exc}")
            continue
        if archives.setdefault(target, archive) != archive:
            issues.append(f"{path.name}: archive SHA-256 {archive} differs from {archives[target]} for {target}")
        for (name, attempt), output in records.items():
            results[name].append((target, attempt, output))
    for filename in sorted(expected.keys() - seen):
        issues.append(f"Missing report: {filename}")

    rows = []
    builds = 0
    matches = 0
    required = len(TARGETS) * ATTEMPTS
    for name in EXAMPLES:
        observations = results[name]
        builds += len(observations)
        groups = defaultdict(list)
        for target, attempt, output in observations:
            groups[output].append(f"{target} attempt {attempt}")
        size, digest = "", "Incomplete"
        if len(groups) > 1:
            digest = "Different ROMs"
            for (byte_count, sha256), runs in sorted(groups.items()):
                issues.append(f"`{name}` produced `{sha256}` ({byte_count:,} bytes) on " + ", ".join(runs))
        elif len(observations) == required:
            byte_count, sha256 = next(iter(groups))
            size, digest = f"{byte_count:,}", f"`{sha256}`"
            matches += 1
        rows.append(f"| `{name}` | {len(observations)}/{required} | {size} | {digest} |")
    passed = not issues and matches == len(EXAMPLES)
    lines = [
        "## Offline ROM reproducibility", "",
        f"**{'PASS' if passed else 'FAIL'}** · {builds}/{len(EXAMPLES) * required} valid builds. "
        f"{matches}/{len(EXAMPLES)} examples produced identical ROM size and SHA-256 across all five packages.", "",
        "| Example | Builds | Bytes | SHA-256 |", "| --- | ---: | ---: | --- |", *rows,
    ]
    if issues:
        lines.extend(["", "### Failed checks", "", *["- " + issue for issue in issues]])
    return passed, "\n".join(lines) + "\n", archives


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path, help="downloaded catalog result artifacts")
    parser.add_argument("--summary", type=Path, required=True, help="write the Markdown report here")
    parser.add_argument("--archive-checksums", type=Path, help="write verified archive checksums on success")
    args = parser.parse_args()
    ok, report, archives = compare(args.directory)
    args.summary.write_text(report, encoding="utf-8")
    if args.archive_checksums:
        if ok:
            lines = []
            for target, digest in sorted(archives.items()):
                ext = "tar.gz" if target.startswith("linux-") else "zip"
                lines.append(f"{digest}  mtile-{target}.{ext}\n")
            args.archive_checksums.write_text("".join(lines), encoding="ascii", newline="\n")
        else:
            args.archive_checksums.unlink(missing_ok=True)
    print(report, end="")
    raise SystemExit(0 if ok else 1)
