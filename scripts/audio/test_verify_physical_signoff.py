#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("verify_physical_signoff.py")
SPEC = importlib.util.spec_from_file_location("verify_physical_signoff", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


MANIFEST = {
    "version": 2,
    "groups": [
        {
            "kind": kind,
            "title": f"CH{index + 1}",
            "cases": [f"CASE {case}" for case in range(count)],
        }
        for index, (kind, count) in enumerate(
            zip(MODULE.EXPECTED_KINDS, MODULE.EXPECTED_COUNTS, strict=True)
        )
    ],
}


def render_results(*, completed: bool = False, failed_case: bool = False) -> str:
    mark = "x" if completed else " "
    metadata = "value" if completed else ""
    lines = ["# Results", ""]
    for name in MODULE.REQUIRED_METADATA:
        lines.append(f"- {name}: {metadata}")
    lines.append("")
    first = True
    for group in MANIFEST["groups"]:
        lines.extend((f"## {group['title']}", ""))
        for index, name in enumerate(group["cases"]):
            result = "PASS" if completed else "PASS / FAIL"
            if completed and failed_case and first:
                result = "FAIL"
            lines.append(f"- [{mark}] {index:02d} — {name}: {result} — notes:")
            first = False
        lines.extend((f"- [{mark}] {MODULE.STOP_CHECK}", ""))
    lines.extend(("## Managed controller", ""))
    for label in MODULE.MANAGED_CHECKS:
        lines.append(f"- [{mark}] {label}")
    lines.extend(("", "## Final decision", ""))
    lines.append(f"- [{mark if completed and not failed_case else ' '}] PASS")
    lines.append(f"- [{'x' if completed and failed_case else ' '}] FAIL")
    return "\n".join(lines) + "\n"


class PhysicalSignoffTest(unittest.TestCase):
    def audit(self, results: str, manifest: dict = MANIFEST):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            manifest_path = root / "CASES.json"
            results_path = root / "RESULTS.md"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            results_path.write_text(results, encoding="utf-8")
            return MODULE.audit_results(manifest_path, results_path)

    def test_blank_template_is_valid_but_pending(self):
        audit = self.audit(render_results())
        self.assertFalse(audit.structural_errors)
        self.assertFalse(audit.failures)
        self.assertEqual(len(audit.pending), 6 + 42 + 4 + 6 + 1)

    def test_complete_pass(self):
        audit = self.audit(render_results(completed=True))
        self.assertTrue(audit.passed, audit)

    def test_explicit_failure_cannot_pass(self):
        audit = self.audit(render_results(completed=True, failed_case=True))
        self.assertFalse(audit.passed)
        self.assertEqual(len(audit.failures), 2)

    def test_manifest_must_have_exact_psg_matrix(self):
        manifest = json.loads(json.dumps(MANIFEST))
        manifest["groups"][0]["cases"].pop()
        audit = self.audit(render_results(), manifest)
        self.assertTrue(audit.structural_errors)


if __name__ == "__main__":
    unittest.main()
