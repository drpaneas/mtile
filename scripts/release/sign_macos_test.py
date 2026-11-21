"""Exercise release signing failure paths without access to a real Apple account."""
import hashlib
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest


SCRIPTS = Path(__file__).resolve().parent
TEAM = "ABC1234567"
LEAF = hashlib.sha1(b"test-leaf").hexdigest()


class MacSigningTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.archive = self.root / "mtile-darwin-arm64.zip"
        contents = self.root / "candidate/Mtile.app/Contents"
        for path in ("MacOS/mtile", "MacOS/mtile-editor", "MacOS/mtile-player", "toolchains/sdk/helper"):
            file = contents / path
            file.parent.mkdir(exist_ok=True, parents=True)
            file.write_bytes(b"test executable")
            file.chmod(0o755)
        with tarfile.open(self.archive, "w") as tar:
            tar.add(self.root / "candidate/Mtile.app", arcname="Mtile.app")
        self.archive.with_suffix(".zip.sha256").write_text(
            f"{hashlib.sha256(self.archive.read_bytes()).hexdigest()}  {self.archive.name}\n")
        self.output = self.root / "release"
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                        MTILE_MACOS_SIGN_IDENTITY=LEAF, MTILE_MACOS_NOTARY_PROFILE="fixture",
                        MTILE_MACOS_EXPECTED_TEAM_ID=TEAM, TEAM=TEAM, TRACE=str(self.root/"trace"))
        # This suite uses tar for the archive round trip on Linux and macOS.
        self.tool("ditto", """
            if [ "$1" = -x ]; then tar -xf "$3" -C "$4"
            else tar -cf "$5" -C "$4" .; fi
        """)
        self.tool("file", "echo 'Mach-O 64-bit executable arm64'")
        self.tool("plutil", 'if [ "$1" = -extract ]; then cat "$6"; fi')
        self.tool("codesign", """
            echo "$*" >> "$TRACE"
            test "$1" = --display || exit 0
            case "$3" in --extract-certificates=*) printf 'test-leaf' > "${3#*=}0";; esac
            echo "TeamIdentifier=$TEAM"
            echo 'CodeDirectory v=20500 flags=0x10000(runtime)'
            case "$*:$FAIL" in */final/*/toolchains/sdk/helper:timestamp) ;;
                *) echo 'Timestamp=2026-01-01';; esac
        """)
        self.tool("xcrun", """
            echo "$1 $2" >> "$TRACE"
            case "$1 $2" in
                'notarytool store-credentials') test "$FAIL" != auth ;;
                'notarytool submit') if [ "$FAIL" = notary ]; then echo Invalid; else echo Accepted; fi ;;
                'stapler staple') touch "$3/.ticket" ;;
                'stapler validate') test -f "$3/.ticket" && test "$FAIL" != ticket ;;
                *) exit 1 ;;
            esac
        """)
        self.tool("spctl", """
            case "$*" in */final/Mtile.app) test "$FAIL" != gatekeeper ;; *) exit 1 ;; esac
        """)

    def tool(self, name, body):
        file = self.bin / name
        file.write_text("#!/bin/sh\nset -e\n" + body + "\n")
        file.chmod(0o755)

    def sign(self, script="sign-macos.sh", **env):
        return subprocess.run([str(SCRIPTS/script), str(self.archive), str(self.output), "0.1.5"],
                              env=dict(self.env, **env), capture_output=True, text=True, timeout=30)

    def test_final_archive_is_measured_and_signed_helpers_checked(self):
        result = self.sign()
        self.assertEqual(result.returncode, 0, result.stderr)
        trace = (self.root/"trace").read_text()
        helper = next(line for line in trace.splitlines() if " -R " in line and "/toolchains/sdk/helper" in line)
        self.assertIn("/final/Mtile.app/", helper)
        self.assertIn(f'certificate leaf = H"{LEAF}"', helper)
        self.assertIn(f'certificate leaf[subject.OU] = "{TEAM}"', helper)
        self.assertIn("certificate leaf[field.1.2.840.113635.100.6.1.13]", helper)
        final = self.output/self.archive.name
        self.assertEqual(final.with_suffix(".zip.sha256").read_text(),
                         f"{hashlib.sha256(final.read_bytes()).hexdigest()}  {final.name}\n")

    def test_failures_leave_no_releasable_output(self):
        cases = [dict(FAIL=reason) for reason in ("notary", "timestamp", "ticket", "gatekeeper")]
        cases += [dict(MTILE_MACOS_NOTARY_PROFILE=""), dict(MTILE_MACOS_SIGN_IDENTITY="-"),
                  dict(MTILE_MACOS_SIGN_IDENTITY="1"*40), dict(MTILE_MACOS_EXPECTED_TEAM_ID="1234567890")]
        for env in cases:
            with self.subTest(env=env):
                self.assertNotEqual(self.sign(**env).returncode, 0)
                self.assertFalse((self.output/self.archive.name).exists())
                self.assertFalse((self.output/(self.archive.name+".sha256")).exists())

    def run_import(self, identity, failure=""):
        self.tool("base64", "cat")
        self.tool("security", """
            case "$1" in
                list-keychains) if [ "$4" = -s ]; then printf '%s\\n' "$@" >> "$TRACE"
                                else echo '    "/test/Previous Keychain"'; fi ;;
                create-keychain) touch "$4" ;;
                delete-keychain) rm "$2" ;;
                find-identity) echo "$IDENTITY" ;;
                unlock-keychain|set-keychain-settings|set-key-partition-list|import) ;;
                *) exit 1 ;;
            esac
        """)
        secret_values = dict(MTILE_MACOS_CERTIFICATE_P12="testP12", MTILE_MACOS_CERTIFICATE_PASSWORD="testPassword",
                             MTILE_MACOS_NOTARY_KEY_P8="testPrivateKey", MTILE_MACOS_NOTARY_KEY_ID="testID",
                             MTILE_MACOS_NOTARY_ISSUER_ID="testIssuer")
        temp = self.root/"runner temp"
        temp.mkdir()
        result = self.sign("sign-macos-release.sh", RUNNER_TEMP=str(temp), IDENTITY=identity,
                           FAIL=failure, **secret_values)
        trace = (self.root/"trace").read_text()
        self.assertEqual(trace.splitlines()[-4:], ["-d", "user", "-s", "/test/Previous Keychain"])
        self.assertEqual(list(temp.iterdir()), [])
        for secret in secret_values.values():
            self.assertNotIn(secret, result.stdout + result.stderr)
        return result, trace

    def test_release_import_rejects_wrong_identity_and_restores_keychains(self):
        result, _ = self.run_import(f'  1) {LEAF} "Apple Development: Test"')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exactly one valid Developer ID Application", result.stderr)

    def test_release_import_rejects_invalid_notary_credentials(self):
        result, trace = self.run_import(f'  1) {LEAF} "Developer ID Application: Test ({TEAM})"', "auth")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("notarization credentials", result.stderr)
        self.assertNotIn("notarytool submit", trace)

    def test_release_uses_imported_identity_and_private_keychain(self):
        result, trace = self.run_import(f'  1) {LEAF} "Developer ID Application: Test ({TEAM})"')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("--keychain ", trace)
        self.assertIn(f"--sign {LEAF} --options runtime --timestamp", trace)
        self.assertIn("notarytool submit", trace)
        self.assertTrue((self.output/self.archive.name).exists())

    def test_release_import_requires_all_credentials(self):
        required = dict(MTILE_MACOS_CERTIFICATE_P12="x", MTILE_MACOS_CERTIFICATE_PASSWORD="x",
                        MTILE_MACOS_NOTARY_KEY_P8="x", MTILE_MACOS_NOTARY_KEY_ID="x",
                        MTILE_MACOS_NOTARY_ISSUER_ID="x")
        for name in required:
            with self.subTest(name=name):
                result = self.sign("sign-macos-release.sh", **dict(required, **{name: ""}))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(name, result.stderr)


if __name__ == "__main__":
    unittest.main()
