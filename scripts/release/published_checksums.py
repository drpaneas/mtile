"""Compare the ROMs built from all published packages with their public checksum file."""

import argparse
import hashlib
import json
from pathlib import Path
import re

from compare_catalog import is_sha256
from release_catalog import EXAMPLES, TARGETS


def read_manifest(path):
    digests = {}
    for number, line in enumerate(path.read_text(encoding="ascii").splitlines(), 1):
        match = re.fullmatch(r"([0-9a-f]{64})  examples/([a-z0-9-]+)/game\.gba", line)
        if not match or match[2] not in EXAMPLES:
            raise ValueError(f"Invalid example checksum at line {number} in {path.name}")
        digest, name = match.groups()
        if name in digests:
            raise ValueError(f"Duplicate checksum for {name} in {path.name}")
        digests[name] = digest
    missing = set(EXAMPLES) - digests.keys()
    if missing:
        raise ValueError(f"Missing checksums in {path.name}: {', '.join(sorted(missing))}")
    return digests


def compare(directory, manifest):
    expected = read_manifest(manifest)
    manifest_sha256 = hashlib.sha256(manifest.read_bytes()).hexdigest()
    targets = set()
    sizes = {}
    for path in sorted(directory.rglob("*.json")):
        target = path.stem
        if target not in TARGETS or target in targets:
            raise ValueError(f"Unexpected or duplicate report {path.relative_to(directory)}")
        report = json.loads(path.read_text(encoding="utf-8-sig"))
        if (not isinstance(report, dict) or report.get("target") != target or
                not is_sha256(report.get("archive_sha256")) or not isinstance(report.get("records"), list)):
            raise ValueError(f"Invalid target, archive SHA-256 or records in {path.name}")
        if report.get("manifest_sha256") != manifest_sha256:
            raise ValueError(f"{target} used a different examples.sha256")
        names = set()
        for record in report["records"]:
            if not isinstance(record, dict):
                raise ValueError(f"Invalid measurement in {path.name}")
            name = record.get("example")
            if not isinstance(name, str) or name not in expected or name in names:
                raise ValueError(f"Unexpected or duplicate example {name!r} on {target}")
            if record.get("sha256") != expected[name]:
                raise ValueError(f"{target} {name}: expected {expected[name]}, got {record.get('sha256')}")
            size = record.get("bytes")
            if type(size) is not int or size <= 192 or sizes.setdefault(name, size) != size:
                raise ValueError(f"{target} {name}: invalid or inconsistent ROM size {size}")
            names.add(name)
        missing = expected.keys() - names
        if missing:
            raise ValueError(f"Missing measurements on {target}: {', '.join(sorted(missing))}")
        targets.add(target)
    if targets != set(TARGETS):
        raise ValueError(f"Missing target reports: {', '.join(sorted(set(TARGETS) - targets))}")

    count = len(targets) * len(expected)
    lines = [f"**{count}/{count}** offline builds matched the published `examples.sha256`.", "",
             "| Example | Packages verified | ROM bytes | SHA-256 |", "| --- | ---: | ---: | --- |"]
    for name in EXAMPLES:
        lines.append(f"| `{name}` | {len(targets)} | {sizes[name]:,} | `{expected[name]}` |")
    return "\n".join(lines)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("reports", type=Path)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--summary", type=Path, required=True)
    args = parser.parse_args()
    try:
        body = compare(args.reports, args.manifest)
    except (ValueError, OSError) as error:
        args.summary.write_text(f"**Published release verification failed.** {error}\n", encoding="utf-8")
        raise SystemExit(str(error))
    args.summary.write_text(f"## Published release verification\n\n{body}\n", encoding="utf-8")
    print(body)
