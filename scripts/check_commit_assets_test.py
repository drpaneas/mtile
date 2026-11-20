import gzip
import hashlib
import io
from pathlib import Path
import os
import subprocess
import sys
import tarfile
import tempfile
import unittest
import zipfile


CHECKER = Path(__file__).with_name("check_commit_assets.py").resolve()


def gba_header():
    header = bytearray(192)
    header[3] = 0xea
    header[0xb2] = 0x96
    header[0xbd] = (-0x19 - 0x96) % 256
    return header


def archive_bytes(kind, filename, payload):
    out = io.BytesIO()
    if kind == "zip":
        with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as archive:
            archive.writestr(filename, payload)
    else:
        with tarfile.open(fileobj=out, mode="w:" + kind) as archive:
            member = tarfile.TarInfo(filename)
            member.size = len(payload)
            archive.addfile(member, io.BytesIO(payload))
    return out.getvalue()


class CommitAssetsTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
        self.git("init", "-q")
        self.stage("scripts/restricted_asset_blobs.txt", b"")

    def git(self, *args):
        subprocess.run(["git", *args], cwd=self.root, env=self.env, check=True,
                       stdout=subprocess.DEVNULL)

    def stage(self, name, data):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        self.git("add", "-f", "--", name)
        return path

    def scan(self, success=False):
        result = subprocess.run([sys.executable, str(CHECKER)], cwd=self.root,
                                env=self.env, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0 if success else 1, result.stderr)
        return result.stderr

    def test_staged_bytes_and_staged_ignore_files(self):
        self.stage("local/.gitignore", b"/private/\n").write_bytes(b"")
        self.stage("local/private/scene.lua", b"return 1")
        self.stage("manual.txt", b"NES\x1a" + bytes(12)).write_bytes(b"safe")
        self.stage("other.txt", b"safe").write_bytes(gba_header())
        self.env["GIT_INDEX_FILE"] = ".git/index"
        errors = self.scan()
        self.assertIn("local/private/scene.lua", errors)
        self.assertIn("manual.txt", errors)
        self.assertNotIn("other.txt", errors)

    def test_names_magic_and_nested_archives(self):
        self.stage("DUMP.GbA", b"")
        self.stage("art.data", gba_header())
        self.stage("sample.txt", archive_bytes("gz", "evidence.zip",
                                               archive_bytes("zip", "memo", b"\x7fELF" + bytes(32))))
        self.stage("evidence.tar.xz", archive_bytes("xz", "notes/part", gba_header()))
        self.stage("other.zip", archive_bytes("zip", "demo.NES", b""))
        self.stage("system.zip", archive_bytes("zip", "firmware/GBC_BIOS.BIN", b""))
        errors = self.scan()
        for name in ("DUMP.GbA", "art.data", "sample.txt", "evidence.zip", "memo",
                     "evidence.tar.xz", "notes/part", "other.zip", "demo.NES", "firmware/GBC_BIOS.BIN"):
            self.assertIn(name, errors)

    def test_measurements_and_ordinary_gzip_logs(self):
        text = b"ROM path: /tmp/game.gba (sha256 recorded)\n"
        self.stage("capture.json.gz", gzip.compress(text))
        self.stage("evidence.tar.gz", archive_bytes("gz", "frames.log", text))
        self.stage("data.zip", archive_bytes("zip", "result.json", b"{}"))
        self.scan(success=True)

    def test_retired_png_under_new_name_and_inside_archive(self):
        image = b"\x89PNG\r\n\x1a\n" + bytes(range(100))
        digest = hashlib.sha1(f"blob {len(image)}\0".encode() + image, usedforsecurity=False)
        row = f"{digest.hexdigest()} {len(image)} private/sprites.png\n"
        self.stage("scripts/restricted_asset_blobs.txt", row.encode()).write_bytes(b"")
        self.stage("assets/new-scene.png", image)
        self.stage("report.zip", archive_bytes("zip", "preview.png", image))
        self.stage("compressed.dat", gzip.compress(image[:12]) + gzip.compress(image[12:]))
        self.stage("unrelated.png", image[::-1])
        errors = self.scan()
        self.assertIn("assets/new-scene.png", errors)
        self.assertIn("preview.png", errors)
        self.assertIn("compressed.dat", errors)
        self.assertIn("private/sprites.png", errors)
        self.assertNotIn("unrelated.png", errors)

    def test_opaque_archives(self):
        self.stage("evidence.7Z", b"")
        self.stage("bundle.dat", b"Rar!\x1a\x07\x01\x00")
        errors = self.scan()
        self.assertIn("evidence.7Z", errors)
        self.assertIn("bundle.dat", errors)


if __name__ == "__main__":
    unittest.main()
