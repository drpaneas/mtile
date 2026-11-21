"""Test a fresh Mac's first CLI launch from the two downloads clicked in the install guide.

Keep browser quarantine intact. Leave any Gatekeeper prompt unanswered. To test the
whole Mac offline, save the live guide and GitHub release JSON before disconnecting.
"""
import argparse
import hashlib
from html.parser import HTMLParser
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import sys
import tempfile
from urllib.request import urlopen

from published_checksums import read_manifest
from verify_download import desktop_env

GUIDE = "https://drpaneas.com/mtile/guides/install.html"
RELEASES = "https://github.com/drpaneas/mtile/releases"
SETUP = 'export PATH="$PWD/Mtile.app/Contents/MacOS:$PATH"'
BUILD = "mtile build examples/hello"
CHECK = r"grep '  examples/hello/game\.gba$' examples.sha256 | shasum -a 256 -c -"
OK = "examples/hello/game.gba: OK"


class Guide(HTMLParser):
    def __init__(self):
        super().__init__()
        self.links = set()
        self.code = set()
        self.block = None

    def handle_starttag(self, tag, attrs):
        if tag == "a":
            self.links.add(dict(attrs).get("href"))
        elif tag == "pre":
            self.block = []

    def handle_data(self, data):
        if self.block is not None:
            self.block.append(data)

    def handle_endtag(self, tag):
        if tag == "pre" and self.block is not None:
            self.code.add("".join(self.block).strip())
            self.block = None


def source(path, url):
    if path:
        return path.read_bytes()
    with urlopen(url, timeout=30) as response:
        return response.read()


def check_downloads(archive, manifest, tag, guide_html, release):
    guide = Guide()
    guide.feed(guide_html.decode("utf-8-sig"))
    guide.close()
    if not {SETUP, BUILD, CHECK} <= guide.code:
        raise ValueError("The macOS instructions on the site have changed")
    if (release["tag_name"] != tag or release["html_url"] != f"{RELEASES}/tag/{tag}"
            or release["draft"] or release["prerelease"]):
        raise ValueError("Expected the published production release for the requested tag")
    for path in (archive, manifest):
        if f"{RELEASES}/latest/download/{path.name}" not in guide.links:
            raise ValueError(f"Missing download link on the site for {path.name}")
        asset, = [a for a in release["assets"] if a["name"] == path.name]
        with path.open("rb") as stream:
            hasher = hashlib.sha256()
            for chunk in iter(lambda: stream.read(1 << 20), b""):
                hasher.update(chunk)
            digest = hasher.hexdigest()
        if (asset["browser_download_url"] != f"{RELEASES}/download/{tag}/{path.name}"
                or asset["digest"] != f"sha256:{digest}"):
            raise ValueError(f"{path.name} differs from the public {tag} asset")
        quarantine(path, browser=True)


def quarantine(path, browser=False):
    record = subprocess.check_output(
        ["/usr/bin/xattr", "-p", "com.apple.quarantine", path], text=True).strip().split(";")
    if len(record) < 3 or not record[0]:
        raise ValueError(f"No quarantine record on {path}")
    if browser and not re.fullmatch(
            r"(?:Google )?Chrome|Chromium|Safari|Safari Technology Preview|(?:Mozilla )?Firefox|"
            r"(?:Microsoft )?Edge|Brave(?: Browser)?|Arc|Vivaldi", record[2], re.IGNORECASE):
        raise ValueError(f"Expected a supported browser in the quarantine record for {path.name}")


def build(work, root, cli):
    env = desktop_env(work / "fresh profile")
    env["GOWORK"] = "off"
    for command in ("grep", "shasum"):
        (Path(env["PATH"]) / command).symlink_to(Path("/usr/bin") / command)
    env["MTILE_CLI"] = str(cli)
    commands = "\n".join((SETUP, '[[ "$(command -v mtile)" == "$MTILE_CLI" ]]', BUILD, CHECK))
    args = ["/usr/bin/sandbox-exec", "-p", "(version 1) (allow default) (deny network*)",
            "/bin/bash", "-eu", "-o", "pipefail", "-c", commands]
    with subprocess.Popen(args, cwd=root, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT, text=True, start_new_session=True) as child:
        try:
            output, _ = child.communicate(timeout=300)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGKILL)
            child.communicate()
            raise RuntimeError("First launch did not finish within five minutes; check for a Gatekeeper prompt")
    print(output, flush=True)
    if child.returncode or not output.splitlines() or output.splitlines()[-1] != OK:
        raise RuntimeError(f"Offline Hello build or checksum failed (exit {child.returncode})")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--team-id", required=True, help="publisher's 10-character Apple Developer Team ID")
    parser.add_argument("--downloads", type=Path, required=True, help="folder with the browser's ZIP and examples.sha256")
    parser.add_argument("--guide-html", type=Path, help=f"saved copy of {GUIDE} for a disconnected Mac")
    parser.add_argument("--release-json", type=Path, help="saved public GitHub release API response for --tag")
    parser.add_argument("--validate-ticket-online", action="store_true",
                        help="also compare the ticket to Apple's live service after the offline build")
    args = parser.parse_args()
    if not re.fullmatch(r"v\d+\.\d+\.\d+", args.tag) or not re.fullmatch(r"[A-Z0-9]{10}", args.team_id):
        parser.error("Expected a vMAJOR.MINOR.PATCH tag and 10-character Apple Developer Team ID")
    arch = {"arm64": "arm64", "x86_64": "amd64"}.get(platform.machine())
    if sys.platform != "darwin" or not arch:
        parser.error("Run on a native Intel or Apple Silicon Mac")
    if subprocess.check_output(["/usr/sbin/sysctl", "-in", "sysctl.proc_translated"]).strip() == b"1":
        parser.error("Start a native shell and interpreter outside Rosetta")
    if subprocess.check_output(["/usr/sbin/spctl", "--status"]).strip() != b"assessments enabled":
        parser.error("This test requires Gatekeeper assessments to be enabled")

    archive = args.downloads.resolve() / f"mtile-darwin-{arch}.zip"
    manifest = archive.with_name("examples.sha256")
    info = f"https://api.github.com/repos/drpaneas/mtile/releases/tags/{args.tag}"
    check_downloads(archive, manifest, args.tag, source(args.guide_html, GUIDE),
                    json.loads(source(args.release_json, info)))
    expected = read_manifest(manifest)["hello"]
    with tempfile.TemporaryDirectory(prefix="mtile-browser-") as temp:
        work = Path(temp).resolve()
        root = work / "Browser Download"
        root.mkdir()
        subprocess.run(["/usr/bin/ditto", "-x", "-k", archive, root], check=True)
        app = root / "Mtile.app"
        cli = app / "Contents/MacOS/mtile"
        quarantine(cli)
        rom = root / "examples/hello/game.gba"
        if rom.exists() or not (rom.parent / "project.json").is_file():
            raise ValueError("Expected a pristine bundled Hello project")
        shutil.copyfile(manifest, root / "examples.sha256")
        print("First launch with quarantine intact. Leave any security prompt unanswered.", flush=True)
        build(work, root, cli)
        if hashlib.sha256(rom.read_bytes()).hexdigest() != expected:
            raise ValueError("Hello ROM differs from the published SHA-256")
        # Assess the artifact only after the observed first launch.
        req = ('=anchor apple generic and certificate leaf[field.1.2.840.113635.100.6.1.13] exists '
               f'and certificate leaf[subject.OU] = "{args.team_id}"')
        for executable in (app, cli):
            subprocess.run(["/usr/bin/codesign", "--verify", "--deep", "--strict", "-R", req, executable],
                           check=True)
        details = subprocess.check_output(
            ["/usr/bin/codesign", "--display", "--verbose=4", app],
            stderr=subprocess.STDOUT, text=True, env=dict(os.environ, LC_ALL="C"))
        if "Notarization Ticket=stapled" not in details.splitlines():
            raise ValueError("The downloaded app has no stapled notarization ticket")
        assessment = subprocess.check_output(
            ["/usr/sbin/spctl", "--assess", "--type", "execute", "--verbose=2", app],
            stderr=subprocess.STDOUT, text=True, env=dict(os.environ, LC_ALL="C"))
        print(assessment.strip())
        if "source=Notarized Developer ID" not in assessment.splitlines():
            raise ValueError("Gatekeeper did not report Notarized Developer ID")
        if args.validate_ticket_online:
            subprocess.run(["/usr/bin/xcrun", "stapler", "validate", app], check=True)
        print(f"{args.tag} darwin-{arch}  {OK}  SHA-256 {expected}")


if __name__ == "__main__":
    main()
