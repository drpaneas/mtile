"""Regressions for the published desktop release comparison."""

import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from published_checksums import compare, read_manifest
from release_catalog import EXAMPLES, TARGETS


class PublishedChecksumsTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        self.manifest = root / "examples.sha256"
        self.reports = root / "reports"
        self.reports.mkdir()
        self.lines = []
        records = []
        for name in EXAMPLES:
            digest = hashlib.sha256(name.encode()).hexdigest()
            self.lines.append(f"{digest}  examples/{name}/game.gba\n")
            records.append({"example": name, "sha256": digest, "bytes": 2048})
        self.manifest.write_text("".join(self.lines), encoding="ascii")
        for target in TARGETS:
            report = {"target": target, "archive_sha256": hashlib.sha256(target.encode()).hexdigest(),
                      "manifest_sha256": hashlib.sha256(self.manifest.read_bytes()).hexdigest(),
                      "records": records}
            (self.reports / f"{target}.json").write_text(json.dumps(report))
        self.path = self.reports / "windows-amd64.json"

    def test_all_targets_match(self):
        self.assertIn("100/100", compare(self.reports, self.manifest))

    def test_modified_rom(self):
        report = json.loads(self.path.read_text())
        report["records"][0]["sha256"] = "f" * 64
        self.path.write_text(json.dumps(report))
        with self.assertRaisesRegex(ValueError, f"windows-amd64 {EXAMPLES[0]}"):
            compare(self.reports, self.manifest)

    def test_another_manifest_with_same_entries(self):
        self.manifest.write_text("".join(reversed(self.lines)))
        with self.assertRaisesRegex(ValueError, "different examples.sha256"):
            compare(self.reports, self.manifest)

    def test_malformed_manifest(self):
        for lines, message in [
                (self.lines[1:], "Missing checksums"), (self.lines + self.lines[:1], "Duplicate checksum"),
                (["0" * 64 + "  examples/../game.gba\n"] + self.lines[1:], "Invalid example checksum")]:
            with self.subTest(message=message):
                self.manifest.write_text("".join(lines))
                with self.assertRaisesRegex(ValueError, message):
                    read_manifest(self.manifest)

    def test_missing_or_duplicate_measurement(self):
        report = json.loads(self.path.read_text())
        records = report["records"]
        for altered, message in [(records[1:], "Missing measurements"), (records + records[:1], "duplicate example")]:
            with self.subTest(message=message):
                report["records"] = altered
                self.path.write_text(json.dumps(report))
                with self.assertRaisesRegex(ValueError, message):
                    compare(self.reports, self.manifest)

    def test_missing_or_duplicate_target(self):
        duplicate = self.reports / "retry" / self.path.name
        duplicate.parent.mkdir()
        duplicate.write_bytes(self.path.read_bytes())
        with self.assertRaisesRegex(ValueError, "duplicate report"):
            compare(self.reports, self.manifest)
        duplicate.unlink()
        self.path.unlink()
        with self.assertRaisesRegex(ValueError, "Missing target reports: windows-amd64"):
            compare(self.reports, self.manifest)


if __name__ == "__main__":
    unittest.main()
