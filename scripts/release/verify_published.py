"""Build the published examples in the extracted folder using the installation guide."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

from published_checksums import read_manifest
from release_catalog import TARGETS
from verify_catalog import check_projects, check_target
from verify_download import desktop_env, unpack, windows_offline


POSIX_CHECK = r"""
[[ "$(command -v mtile)" == "$MTILE_CLI" ]]
mtile build "examples/$MTILE_EXAMPLE"
grep "  examples/$MTILE_EXAMPLE/game\.gba$" examples.sha256 | "$@" -c -
"""

WINDOWS_CHECK = r"""
$ErrorActionPreference = 'Stop'
if ((Get-Command mtile -CommandType Application).Source -ne $env:MTILE_CLI) { throw 'Unexpected mtile on PATH' }
$project = "examples/$env:MTILE_EXAMPLE"
mtile build $project
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$rom = "$project/game.gba"
$hash = (Get-FileHash $rom -Algorithm SHA256 -ErrorAction Stop).Hash
if ((Get-Content examples.sha256 -ErrorAction Stop) -notcontains "$hash  $rom") { throw "$rom checksum mismatch" }
Write-Output "${rom}: OK"
"""


def verify(archive, target, manifest, output, block_windows_network):
    check_target(archive, target)
    expected = read_manifest(manifest)
    with tempfile.TemporaryDirectory(prefix="mtile-published-") as temp:
        work = Path(temp)
        install = work / "Desktop Download"
        install.mkdir()
        launcher, tools, _ = unpack(archive.resolve(), install)
        root = install if sys.platform == "darwin" else launcher.parent
        check_projects(root / "examples")
        shutil.copyfile(manifest, root / "examples.sha256")
        env = desktop_env(work / "home")
        shells = []
        if os.name == "nt":
            powershell = Path(os.environ["SystemRoot"]) / "System32/WindowsPowerShell/v1.0/powershell.exe"
            shells = [powershell, os.environ["COMSPEC"]]
            command = [powershell, "-NoProfile", "-NonInteractive", "-Command", WINDOWS_CHECK]
            env["MTILE_CLI"] = str(tools / "mtile.exe")
        else:
            checker = ["shasum", "-a", "256"] if sys.platform == "darwin" else ["sha256sum"]
            # Expose only the two system utilities used by the guide on the clean PATH.
            for program in ("grep", checker[0]):
                (Path(env["PATH"]) / program).symlink_to(Path("/usr/bin") / program)
            command = ["/bin/bash", "-eu", "-o", "pipefail", "-c", POSIX_CHECK, "verify", *checker]
            env["MTILE_CLI"] = str(tools / "mtile")
        env["PATH"] = str(tools) + os.pathsep + env["PATH"]
        records = []
        with windows_offline(install, block_windows_network, shells):
            for name, digest in expected.items():
                print(f"[{target}] mtile build examples/{name}", flush=True)
                env["MTILE_EXAMPLE"] = name
                subprocess.run(command, cwd=root, env=env, check=True, timeout=600)
                data = (root / "examples" / name / "game.gba").read_bytes()
                actual = hashlib.sha256(data).hexdigest()
                if actual != digest:
                    raise ValueError(f"{target} {name}: expected {digest}, got {actual}")
                records.append({"example": name, "bytes": len(data), "sha256": actual})
    # unpack compared this sidecar with the bytes of the downloaded archive.
    report = {"target": target, "archive_sha256": Path(str(archive) + ".sha256").read_text().split()[0],
              "manifest_sha256": hashlib.sha256(manifest.read_bytes()).hexdigest(), "records": records}
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(f"{target}: all {len(records)} ROMs matched the public examples.sha256.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--target", choices=TARGETS, required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--block-windows-network", action="store_true")
    args = parser.parse_args()
    verify(args.archive, args.target, args.manifest, args.output, args.block_windows_network)
