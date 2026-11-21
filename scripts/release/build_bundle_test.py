"""Packaging integration checks with stub compilers; no downloads or real builds."""

import hashlib
import os
import plistlib
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
        shutil.copytree(SCRIPT.with_name("tinygo-extra"), script.with_name("tinygo-extra"))
        for name in ("LICENSE", "scripts/release/BUNDLE-README.md", "third_party/mgba/LICENSE",
                     "docs/third-party/mgba.md", "cmd/mtile-player/mgba_halt_compat.cmake", "docs/third-party/tinygo.md",
                     "docs/third-party/TinyGo-LICENSE.txt", "runtime/maxmod/COPYING",
                     "docs/third-party/maxmod.md", "fonts/JetBrainsMono/OFL.txt",
                     "fonts/OpenDyslexic/LICENSE.txt", "fonts/pico-8-wide.otf/license.txt",
                     "runtime/third_party/aurora/README.md", "runtime/third_party/aurora/COPYING",
                     "runtime/third_party/aurora/pfunc_pal_Aurora11.lua"):
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("fixture notice: " + name + "\n")
        self.tools = Path(self.temp.name) / "tools"
        self.tools.mkdir()
        self.env = dict(os.environ, PATH=str(self.tools) + os.pathsep + os.environ["PATH"],
                        GOOS="linux", GOARCH="amd64", FAKE_OS="linux", FAKE_ARCH="amd64")
        self.env.pop("TINYGO_SHA256", None)
        self.env.pop("TINYGO_VERSION", None)
        self.env.pop("TINYGO_ARCHIVE", None)
        sdk = Path(self.temp.name) / "Go SDK"
        for name in ("bin", "lib", "pkg", "src"):
            (sdk / name).mkdir(parents=True)
        for name in ("VERSION", "go.env", "LICENSE", "PATENTS"):
            (sdk / name).write_text(name)
        self.sdk = sdk
        self.env["GOROOT"] = str(sdk)
        self.env["MTILE_TEST_CLI_SOURCE"] = "#!" + sys.executable + """
import os, pathlib, sys
assert not list(pathlib.Path(os.environ["PATH"]).iterdir())
assert "TINYGOROOT" not in os.environ and "GOROOT" not in os.environ
assert os.environ["GOPROXY"] == "off"
args = sys.argv[1:]
if args == ["new", "-template", "coin-quest", "game"]:
    pathlib.Path("game").mkdir()
elif args == ["build", "game"]:
    pathlib.Path("game/game.gba").write_bytes(b"Coin Quest")
    print("SMOKE:", os.getcwd())
elif args != ["doctor"]:
    raise SystemExit("unexpected CLI command: " + str(args))
"""
        self.write_tool("go", """
import os, pathlib, sys
args = sys.argv[1:]
if args[0] == 'env':
 key = args[1]
 if key == 'GOHOSTOS': print(os.environ['FAKE_OS'])
 elif key == 'GOHOSTARCH': print(os.environ['FAKE_ARCH'])
 else: print(os.environ[key])
elif args[0] == 'list':
 print('example.org/lib@v1.0.0' + chr(9) + os.environ['GOROOT'])
elif args[0] == 'version': print('go version go1.27.1 fixture')
elif args[0] == 'build':
 out = pathlib.Path(args[args.index('-o') + 1]); out.parent.mkdir(parents=True, exist_ok=True)
 out.write_text(os.environ['MTILE_TEST_CLI_SOURCE'])
 out.chmod(0o755)
else: raise SystemExit(2)
""")
        self.write_tool("cmake", '''
import os, pathlib, shutil, sys
args = sys.argv[1:]
if args[:3] == ['-E', 'tar', 'cf']:
 shutil.make_archive(str(pathlib.Path(args[3]).with_suffix('')), 'zip', '.', args[5])
elif '--build' in args:
 root = pathlib.Path(args[args.index('--build') + 1])
 if os.environ['FAKE_OS'] == 'windows': root /= 'Release'
 root.mkdir(parents=True, exist_ok=True)
 (root / ('mtile-player.exe' if os.environ['FAKE_OS'] == 'windows' else 'mtile-player')).write_text('player')
''')
        self.write_tool("codesign", "import sys\nassert sys.argv[1] in ('--force', '--verify')\n")
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
        shutil.copy2(self.tools / "go", self.sdk / ("bin/go" + exe))
        tinygo = Path(self.temp.name) / "tinygo"
        for directory in ("bin", "lib/clang", "lib/compiler-rt-builtins", "lib/picolibc",
                          "src/device/arm", "src/device/gba", "src/runtime", "targets"):
            (tinygo / directory).mkdir(parents=True, exist_ok=True)
            (tinygo / directory / "fixture").write_text("input")
        (tinygo / "lib/picolibc-stdio.c").write_text("fixture stdio")
        compiler = tinygo / ("bin/tinygo" + exe)
        compiler.write_text("#!/bin/sh\necho 'tinygo version 0.42.0 fixture'\n")
        compiler.chmod(0o755)
        (tinygo / "src/device/asm.go").touch()
        (tinygo / "targets/gameboy-advance.json").write_text("{}")
        (tinygo / "targets/gameboy-advance.s").write_text("fixture GBA startup")
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
        archive = output / "mtile-linux-amd64.tar.gz"
        self.assert_digest(archive)
        with tarfile.open(archive) as tar:
            tar.extractall(Path(self.temp.name) / "unpacked", filter="data")
        bundle = Path(self.temp.name) / "unpacked/mtile-linux-amd64"
        self.assertTrue(os.access(bundle / "Mtile", os.X_OK))
        self.assertTrue((bundle / "licenses/go-modules/example.org/lib@v1.0.0/LICENSE").is_file())
        self.assertTrue(os.access(bundle / "bin/mtile-editor", os.X_OK))
        self.assertEqual((bundle / "coin-quest.gba").read_bytes(), b"Coin Quest")
        self.assertTrue((bundle / "toolchains/go/bin/go").is_file())
        self.assertTrue((bundle / "toolchains/go/LICENSE").is_file())
        self.assertEqual((bundle / "toolchains/tinygo/lib/picolibc-stdio.c").read_text(), "fixture stdio")
        for wrapper in SCRIPT.with_name("tinygo-extra").iterdir():
            self.assertEqual((bundle / "toolchains/tinygo/targets/mtile" / wrapper.name).read_bytes(),
                             wrapper.read_bytes())
        self.assertIn("Working tree: clean", (bundle / "BUILD-INFO.txt").read_text())
        self.assertEqual((bundle / "toolchains/tinygo/targets/gameboy-advance.s").read_text(), "fixture GBA startup")
        self.assertIn("BUNDLE-README.md", (bundle / "README.md").read_text())
        for name in ("MGBA-HALT-COMPAT.cmake", "JETBRAINS-MONO-OFL.txt", "OPENDYSLEXIC-LICENSE.txt", "PICO-8-WIDE-CC0.txt",
                     "aurora/README.md", "aurora/COPYING", "aurora/pfunc_pal_Aurora11.lua"):
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
        archive = output / "mtile-windows-amd64.zip"
        self.assert_digest(archive)
        with zipfile.ZipFile(archive) as z:
            for name in ("Mtile.exe", "coin-quest.gba", "bin/mtile.exe", "bin/mtile-player.exe",
                         "toolchains/go/bin/go.exe", "toolchains/tinygo/bin/tinygo.exe"):
                self.assertIn("mtile-windows-amd64/" + name, z.namelist())

    def test_macos_archive_opens_as_an_app(self):
        self.env.update(GOOS="darwin", FAKE_OS="darwin")
        output = self.root / "mac output"
        self.archive(output, "darwin")
        result = self.run_bundle(output)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        archive = output / "mtile-darwin-amd64.zip"
        self.assert_digest(archive)
        with zipfile.ZipFile(archive) as z:
            contents = "Mtile.app/Contents/"
            info = plistlib.loads(z.read(contents + "Info.plist"))
            self.assertEqual(info["LSMinimumSystemVersion"], "15.0")
            executable = z.getinfo(contents + "MacOS/" + info["CFBundleExecutable"])
            self.assertTrue((executable.external_attr >> 16) & 0o111)
            for name in ("Resources/coin-quest.gba", "MacOS/mtile", "MacOS/mtile-player",
                         "toolchains/go/bin/go", "toolchains/tinygo/bin/tinygo",
                         "Resources/licenses/GO-BSD-3-CLAUSE.txt"):
                self.assertIn(contents + name, z.namelist())

    def assert_digest(self, archive):
        checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
        self.assertEqual(Path(str(archive) + ".sha256").read_text(), f"{checksum}  {archive.name}\n")


if __name__ == "__main__":
    unittest.main()
