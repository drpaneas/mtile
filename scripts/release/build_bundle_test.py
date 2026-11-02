"""Packaging integration checks with stub compilers; no downloads or real builds."""

import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest
import zipfile


SCRIPT = Path(__file__).with_name("build-bundle.sh")


class BundleTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="mtile-bundle-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "source"
        self.root.mkdir()
        script = self.root / "scripts/release/build-bundle.sh"
        script.parent.mkdir(parents=True)
        shutil.copyfile(SCRIPT, script)
        self.script = script
        for name in ("LICENSE", "scripts/release/BUNDLE-README.md", "third_party/mgba/LICENSE",
                     "docs/third-party/mgba.md", "cmd/mtile-player/mgba_halt_compat.cmake", "docs/third-party/tinygo.md",
                     "docs/third-party/TinyGo-LICENSE.txt", "runtime/maxmod/COPYING",
                     "docs/third-party/maxmod.md", "fonts/JetBrainsMono/OFL.txt",
                     "fonts/OpenDyslexic/LICENSE.txt", "fonts/pico-8-wide.otf/license.txt"):
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("fixture notice: " + name + "\n")
        self.tools = Path(self.temp.name) / "tools"
        self.tools.mkdir()
        self.env = dict(os.environ, PATH=str(self.tools) + os.pathsep + os.environ["PATH"],
                        GOOS="linux", GOARCH="amd64", FAKE_OS="linux", FAKE_ARCH="amd64")
        self.env.pop("TINYGO_SHA256", None)
        self.env.pop("TINYGO_VERSION", None)
        self.env["MTILE_TEST_CLI_SOURCE"] = """#!/bin/sh
set -eu
case "$1" in
 new) mkdir "$2";;
 build) test "$2" = -tinygo; test -x "$3"; test -d "$TINYGOROOT"; printf gba > "$4/game.gba"; echo "SMOKE: $PWD";;
 *) exit 2;;
esac
"""
        self.write_tool("go", """
import os, pathlib, sys
args = sys.argv[1:]
if args[0] == 'env':
 key = args[1]
 if key == 'GOHOSTOS': print(os.environ['FAKE_OS'])
 elif key == 'GOHOSTARCH': print(os.environ['FAKE_ARCH'])
 else: print(os.environ[key])
elif args[0] == 'version': print('go version go1.27.1 fixture')
elif args[0] == 'build':
 out = pathlib.Path(args[args.index('-o') + 1]); out.parent.mkdir(parents=True, exist_ok=True)
 out.write_text(os.environ['MTILE_TEST_CLI_SOURCE'])
 out.chmod(0o755)
else: raise SystemExit(2)
""")
        self.write_tool("cmake", '''
import os, pathlib, sys
args = sys.argv[1:]
if '--build' in args:
 root = pathlib.Path(args[args.index('--build') + 1])
 if os.environ['FAKE_OS'] == 'windows': root /= 'Release'
 root.mkdir(parents=True, exist_ok=True)
 (root / ('mtile-player.exe' if os.environ['FAKE_OS'] == 'windows' else 'mtile-player')).write_text('player')
''')
        self.write_tool("git", '''
import sys
if 'rev-parse' in sys.argv: print('a' * 40)
elif 'status' not in sys.argv: raise SystemExit(2)
''')
        self.write_tool("curl", "raise SystemExit('unexpected network request')\n")

    def write_tool(self, name, source):
        path = self.tools / name
        path.write_text("#!" + sys.executable + "\n" + source)
        path.chmod(0o755)

    def archive(self, output, platform="linux"):
        exe = ".exe" if platform == "windows" else ""
        tinygo = Path(self.temp.name) / "tinygo"
        for directory in ("bin", "lib/clang", "lib/compiler-rt-builtins", "lib/picolibc",
                          "src/device/arm", "src/device/gba", "src/runtime", "targets"):
            (tinygo / directory).mkdir(parents=True, exist_ok=True)
            (tinygo / directory / "fixture").write_text("input")
        compiler = tinygo / ("bin/tinygo" + exe)
        compiler.write_text("#!/bin/sh\necho 'tinygo version 0.42.0 fixture'\n")
        compiler.chmod(0o755)
        (tinygo / "src/device/asm.go").touch()
        (tinygo / "targets/gameboy-advance.json").write_text("{}")
        work = output / (".build-" + platform + "-amd64")
        work.mkdir(parents=True)
        suffix = "zip" if platform == "windows" else "tar.gz"
        archive = work / ("tinygo0.42.0." + platform + "-amd64." + suffix)
        if platform == "windows":
            with zipfile.ZipFile(archive, "w") as z:
                for path in tinygo.rglob("*"):
                    z.write(path, path.relative_to(tinygo.parent))
        else:
            with tarfile.open(archive, "w:gz") as tar:
                tar.add(tinygo, arcname="tinygo")
        self.env["TINYGO_SHA256"] = hashlib.sha256(archive.read_bytes()).hexdigest()

    def run_bundle(self, output):
        return subprocess.run(["sh", str(self.script), str(output)], cwd=self.root,
                              env=self.env, capture_output=True, text=True, timeout=30)

    def test_missing_digest_and_cross_target_fail_before_output(self):
        output = self.root / "absent"
        result = self.run_bundle(output)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("TINYGO_SHA256", result.stderr)
        self.assertFalse(output.exists())
        self.env.update(TINYGO_SHA256="a" * 64, GOOS="windows")
        result = self.run_bundle(output)
        self.assertIn("cross-compiling", result.stderr)
        self.assertFalse(output.exists())

    def test_checksum_failure_does_not_extract_toolchain(self):
        output = self.root / "bad"
        self.archive(output)
        self.env["TINYGO_SHA256"] = "0" * 64
        result = self.run_bundle(output)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("FAILED", result.stdout + result.stderr)
        self.assertFalse((output / "mtile-linux-amd64/toolchains/tinygo").exists())

    def test_relative_output_notices_manifest_and_smoke(self):
        output = self.root / "relative-output"
        self.archive(output)
        result = self.run_bundle(Path("relative-output"))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        bundle = output / "mtile-linux-amd64"
        self.assertIn("Working tree: clean", (bundle / "BUILD-INFO.txt").read_text())
        self.assertIn("BUNDLE-README.md", (bundle / "README.md").read_text())
        for name in ("MGBA-HALT-COMPAT.cmake", "JETBRAINS-MONO-OFL.txt", "OPENDYSLEXIC-LICENSE.txt", "PICO-8-WIDE-CC0.txt"):
            self.assertTrue((bundle / "licenses" / name).is_file())
        smoke_line = next(line for line in result.stdout.splitlines() if line.startswith("SMOKE: "))
        smoke = Path(smoke_line.removeprefix("SMOKE: "))
        self.assertFalse(smoke.is_relative_to(self.root))
        self.assertFalse(smoke.exists(), "the temporary smoke project should be cleaned up")

    def test_windows_multiconfig_player_location(self):
        self.env.update(GOOS="windows", FAKE_OS="windows")
        output = self.root / "windows-output"
        self.archive(output, "windows")
        result = self.run_bundle(output)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((output / "mtile-windows-amd64/bin/mtile-player.exe").is_file())


if __name__ == "__main__":
    unittest.main()
