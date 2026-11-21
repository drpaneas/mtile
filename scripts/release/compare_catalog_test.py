"""Regression tests for the comparison that permits creation of a draft release."""

import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from release_catalog import ATTEMPTS, EXAMPLES, SHARDS, TARGETS


def digest(name):
    return hashlib.sha256(name.encode()).hexdigest()


class CompareCatalogTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        self.reports = root / "measurements"
        self.reports.mkdir()
        self.summary = root / "summary.md"
        self.checksums = root / "archives.sha256"
        self.example_checksums = root / "examples.sha256"
        for target in TARGETS:
            for shard in range(SHARDS):
                records = [
                    {"example": name, "attempt": n, "bytes": 512, "sha256": digest(name)}
                    for name in EXAMPLES[shard::SHARDS] for n in range(1, ATTEMPTS + 1)
                ]
                path = self.reports / f"{target}-{shard}.json"
                path.write_text(json.dumps({"target": target, "shard": shard, "records": records,
                                            "archive_sha256": digest(target)}))
        self.shard = self.reports / "windows-amd64-0.json"
        self.data = json.loads(self.shard.read_text())

    def check_command(self, success):
        script = Path(__file__).with_name("compare_catalog.py")
        result = subprocess.run([sys.executable, script, self.reports, "--summary", self.summary,
                                 "--archive-checksums", self.checksums, "--example-checksums", self.example_checksums],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0 if success else 1, result.stdout + result.stderr)
        self.assertEqual(self.checksums.exists(), success)
        self.assertEqual(self.example_checksums.exists(), success)
        return self.summary.read_text(encoding="utf-8")

    def test_all_300_agree_and_bind_the_five_archives(self):
        report = self.check_command(True)
        self.assertIn("300/300", report)
        self.assertIn("20/20", report)
        expected = {f"{digest(target)}  mtile-{target}." + ("tar.gz" if target.startswith("linux-") else "zip")
                    for target in TARGETS}
        self.assertEqual(set(self.checksums.read_text().splitlines()), expected)
        examples = "".join(f"{digest(name)}  examples/{name}/game.gba\n" for name in sorted(EXAMPLES))
        self.assertEqual(self.example_checksums.read_bytes(), examples.encode("ascii"))

    def test_one_different_rom_blocks_the_release(self):
        self.check_command(True)
        record = self.data["records"][1]
        record["sha256"] = digest("changed bytes")
        self.shard.write_text(json.dumps(self.data))
        report = self.check_command(False)
        self.assertIn(record["example"], report)
        self.assertIn(record["sha256"], report)
        self.assertIn("windows-amd64 attempt 2", report)

    def test_different_archive_on_one_shard_blocks_the_release(self):
        self.data["archive_sha256"] = digest("different package")
        self.shard.write_text(json.dumps(self.data))
        self.assertIn("archive SHA-256", self.check_command(False))

    def test_duplicate_cannot_replace_missing_attempt(self):
        records = self.data["records"]
        records[1] = records[0]
        self.shard.write_text(json.dumps(self.data))
        self.assertIn("duplicate result", self.check_command(False))

    def test_missing_attempt_or_entire_shard(self):
        self.data["records"].pop()
        self.shard.write_text(json.dumps(self.data))
        self.assertIn("missing builds", self.check_command(False))
        self.shard.unlink()
        self.assertIn(f"Missing report: {self.shard.name}", self.check_command(False))

    def test_malformed_results_block_the_release(self):
        self.data["records"][0]["sha256"] = "z" * 64
        self.shard.write_text(json.dumps(self.data))
        self.assertIn("invalid ROM size or SHA-256", self.check_command(False))
        self.shard.write_text("{")
        self.assertIn(self.shard.name, self.check_command(False))


if __name__ == "__main__":
    unittest.main()
