#!/usr/bin/env python3
"""Validate the completed physical-GBA Audio v2 sign-off record.

The handoff bundle is useful only if its human listening result stays tied to
the exact 42-case PSG matrix.  This validator accepts the generated blank or
partially completed form in ``--allow-pending`` mode, but its default mode
certifies only an unambiguous physical PASS.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path


EXPECTED_KINDS = ("ch1", "ch2", "ch3", "ch4")
EXPECTED_COUNTS = (12, 10, 10, 10)
REQUIRED_METADATA = (
    "Date/time",
    "Listener",
    "GBA model/serial",
    "Flash cartridge",
    "Output (speaker/headphones/capture)",
    "Power source",
)
MANAGED_CHECKS = (
    "`audiopsg.gba`: the first note sounds after a cold boot.",
    "`audiopsg.gba`: loop timing is stable and every restart begins cleanly.",
    "`audiopsgcoexist.gba`: screen reaches `SELF TEST: PASS`.",
    "`audiopsgcoexist.gba`: `HW CHANNELS SEEN` reaches 15.",
    "All four channel roles remain separately audible for ten minutes.",
    "No missing first note, drops, stuck notes, clicks, drift, or graphics interference.",
)
STOP_CHECK = "SELECT stops cleanly; START restarts the selected case."

CHECK_RE = re.compile(r"^- \[([ xX])\] (.+)$")
CASE_RE = re.compile(
    r"^(\d{2}) — (.*?): (PASS / FAIL|PASS|FAIL) — notes:(.*)$"
)


@dataclass
class Audit:
    structural_errors: list[str] = field(default_factory=list)
    pending: list[str] = field(default_factory=list)
    failures: list[str] = field(default_factory=list)

    @property
    def passed(self) -> bool:
        return not self.structural_errors and not self.pending and not self.failures


def checked(mark: str) -> bool:
    return mark.lower() == "x"


def load_manifest(path: Path, audit: Audit) -> list[dict[str, object]]:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        audit.structural_errors.append(f"cannot read CASES.json: {exc}")
        return []

    if raw.get("version") != 2:
        audit.structural_errors.append("CASES.json version must be 2")
    groups = raw.get("groups")
    if not isinstance(groups, list):
        audit.structural_errors.append("CASES.json groups must be a list")
        return []
    if len(groups) != len(EXPECTED_KINDS):
        audit.structural_errors.append(
            f"CASES.json has {len(groups)} groups; want {len(EXPECTED_KINDS)}"
        )
        return []

    for index, (group, kind, count) in enumerate(
        zip(groups, EXPECTED_KINDS, EXPECTED_COUNTS, strict=True)
    ):
        if not isinstance(group, dict):
            audit.structural_errors.append(f"group {index} is not an object")
            continue
        if group.get("kind") != kind:
            audit.structural_errors.append(
                f"group {index} kind is {group.get('kind')!r}; want {kind!r}"
            )
        cases = group.get("cases")
        if not isinstance(cases, list) or len(cases) != count:
            got = len(cases) if isinstance(cases, list) else "non-list"
            audit.structural_errors.append(
                f"{kind} has {got} cases; want {count}"
            )
    return groups


def parse_checkboxes(text: str, audit: Audit) -> dict[str, dict[str, list[str]]]:
    sections: dict[str, dict[str, list[str]]] = {}
    section = ""
    for line_number, line in enumerate(text.splitlines(), 1):
        if line.startswith("## "):
            section = line[3:].strip()
            continue
        if not line.startswith("- ["):
            continue
        match = CHECK_RE.fullmatch(line)
        if match is None:
            audit.structural_errors.append(
                f"RESULTS.md line {line_number} has a malformed checkbox"
            )
            continue
        mark, label = match.groups()
        sections.setdefault(section, {}).setdefault(label, []).append(mark)
    return sections


def one_checkbox(
    checkboxes: dict[str, list[str]], label: str, audit: Audit, description: str
) -> str | None:
    marks = checkboxes.get(label, [])
    if len(marks) != 1:
        audit.structural_errors.append(
            f"RESULTS.md contains {len(marks)} {description} checkboxes; want 1"
        )
        return None
    return marks[0]


def audit_results(manifest_path: Path, results_path: Path) -> Audit:
    audit = Audit()
    groups = load_manifest(manifest_path, audit)
    try:
        text = results_path.read_text(encoding="utf-8")
    except OSError as exc:
        audit.structural_errors.append(f"cannot read RESULTS.md: {exc}")
        return audit

    sections = parse_checkboxes(text, audit)

    for field_name in REQUIRED_METADATA:
        match = re.search(
            rf"^- {re.escape(field_name)}:[ \t]*([^\r\n]*?)[ \t]*$",
            text,
            flags=re.MULTILINE,
        )
        if match is None:
            audit.structural_errors.append(
                f"RESULTS.md is missing metadata field {field_name!r}"
            )
        elif not match.group(1):
            audit.pending.append(f"metadata: {field_name}")

    for group in groups:
        if not isinstance(group, dict):
            continue
        kind = str(group.get("kind", "unknown"))
        title = str(group.get("title", ""))
        checkboxes = sections.get(title)
        if checkboxes is None:
            audit.structural_errors.append(
                f"RESULTS.md is missing section {title!r}"
            )
            continue
        cases = group.get("cases")
        if not isinstance(cases, list):
            continue
        expected_case_labels: set[str] = set()
        for index, name in enumerate(cases):
            prefix = f"{index:02d} — {name}:"
            matching_labels = [label for label in checkboxes if label.startswith(prefix)]
            if len(matching_labels) != 1:
                audit.structural_errors.append(
                    f"RESULTS.md contains {len(matching_labels)} entries for {kind} case {index:02d}; want 1"
                )
                continue
            label = matching_labels[0]
            expected_case_labels.add(label)
            match = CASE_RE.fullmatch(label)
            if match is None:
                audit.structural_errors.append(
                    f"RESULTS.md has malformed result for {kind} case {index:02d}"
                )
                continue
            _, parsed_name, result, _ = match.groups()
            if parsed_name != name:
                audit.structural_errors.append(
                    f"RESULTS.md name for {kind} case {index:02d} is {parsed_name!r}; want {name!r}"
                )
            marks = checkboxes[label]
            if len(marks) != 1:
                audit.structural_errors.append(
                    f"RESULTS.md duplicates {kind} case {index:02d}"
                )
                continue
            if result == "FAIL":
                audit.failures.append(f"{kind} case {index:02d}: {name}")
            elif result != "PASS" or not checked(marks[0]):
                audit.pending.append(f"{kind} case {index:02d}: {name}")

        parsed_case_labels = {label for label in checkboxes if CASE_RE.fullmatch(label)}
        for label in sorted(parsed_case_labels - expected_case_labels):
            audit.structural_errors.append(
                f"RESULTS.md has unexpected case entry in {title}: {label}"
            )

        stop_mark = one_checkbox(checkboxes, STOP_CHECK, audit, f"{kind} stop/restart")
        if stop_mark is not None and not checked(stop_mark):
            audit.pending.append(f"{kind.upper()} stop/restart")

    managed = sections.get("Managed controller", {})
    for label in MANAGED_CHECKS:
        mark = one_checkbox(managed, label, audit, "managed-controller")
        if mark is not None and not checked(mark):
            audit.pending.append(f"managed controller: {label}")

    final = sections.get("Final decision", {})
    pass_mark = one_checkbox(final, "PASS", audit, "final PASS")
    fail_mark = one_checkbox(final, "FAIL", audit, "final FAIL")
    if fail_mark is not None and checked(fail_mark):
        audit.failures.append("final decision is FAIL")
    if pass_mark is not None and not checked(pass_mark):
        audit.pending.append("final decision PASS")
    if pass_mark is not None and fail_mark is not None and checked(pass_mark) and checked(fail_mark):
        audit.structural_errors.append("final PASS and FAIL cannot both be checked")

    return audit


def print_audit(audit: Audit, *, allow_pending: bool) -> int:
    for message in audit.structural_errors:
        print(f"ERROR: {message}", file=sys.stderr)
    for message in audit.failures:
        print(f"FAIL: {message}", file=sys.stderr)

    if audit.structural_errors or audit.failures:
        return 1
    if audit.pending:
        if allow_pending:
            print(
                f"Physical-GBA result form: structurally valid; {len(audit.pending)} checks pending"
            )
            return 0
        print(
            f"PENDING: physical-GBA sign-off has {len(audit.pending)} incomplete checks",
            file=sys.stderr,
        )
        for message in audit.pending:
            print(f"  - {message}", file=sys.stderr)
        return 1

    print("Physical-GBA Audio v2 sign-off: PASS")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "bundle", type=Path, help="hardware handoff directory containing CASES.json and RESULTS.md"
    )
    parser.add_argument(
        "--allow-pending",
        action="store_true",
        help="validate form structure without requiring a completed physical PASS",
    )
    args = parser.parse_args()
    audit = audit_results(args.bundle / "CASES.json", args.bundle / "RESULTS.md")
    return print_audit(audit, allow_pending=args.allow_pending)


if __name__ == "__main__":
    raise SystemExit(main())
