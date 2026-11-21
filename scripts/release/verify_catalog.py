"""Build a shard of the standalone examples with the candidate archive's CLI and SDKs."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile

from release_catalog import ATTEMPTS, EXAMPLES, SHARDS, TARGETS
from verify_download import desktop_env, unpack, windows_offline


def check_target(archive, target):
    system = platform.system().lower()
    machine = platform.machine().lower()
    arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(machine, machine)
    if f"{system}-{arch}" != target:
        raise ValueError(f"Expected native {target}, running on {system}-{arch}")
    extension = "tar.gz" if system == "linux" else "zip"
    if archive.name != f"mtile-{target}.{extension}":
        raise ValueError(f"Unexpected archive for {target}: {archive.name}")


def check_projects(examples):
    found = {p.parent.relative_to(examples).as_posix() for p in examples.rglob("project.json")}
    # smb3 imports assets from a separately supplied NES cartridge.
    expected = set(EXAMPLES) | {"smb3"}
    if found != expected:
        raise ValueError(f"Packaged catalog differs: missing {sorted(expected - found)}, "
                         f"unexpected {sorted(found - expected)}")
    prebuilt = next(examples.rglob("*.gba"), None)
    if prebuilt is not None:
        raise ValueError(f"Packaged example contains a prebuilt ROM: {prebuilt}")


def build(root, cli, work, name, attempt):
    with tempfile.TemporaryDirectory(prefix=f"cold {attempt} ", dir=work) as tmp:
        run = Path(tmp)
        project = Path("examples") / name
        shutil.copytree(root / project, run / project)
        env = desktop_env(run / "home")
        print(f"[{name}, attempt {attempt}] mtile build {project.as_posix()}", flush=True)
        subprocess.run([cli, "build", project.as_posix()], cwd=run, env=env, check=True, timeout=600)
        rom = (run / project / "game.gba").read_bytes()
        if len(rom) <= 192 or rom[0xB2] != 0x96 or (sum(rom[0xA0:0xBE]) + 0x19) % 256:
            raise ValueError(f"Invalid GBA header for {name} on attempt {attempt}")
        digest = hashlib.sha256(rom).hexdigest()
        print(f"[{name}, attempt {attempt}] {len(rom)} bytes, SHA-256 {digest}", flush=True)
        return {"example": name, "attempt": attempt, "bytes": len(rom), "sha256": digest}


def verify(archive, target, shard, output, block_windows_network):
    check_target(archive, target)
    with tempfile.TemporaryDirectory(prefix="mtile-catalog-") as tmp:
        work = Path(tmp)
        install = work / "Candidate Download"
        install.mkdir()
        launcher, tools, _ = unpack(archive.resolve(), install)
        # unpack has compared this checksum with the actual archive bytes.
        archive_sha256 = Path(str(archive) + ".sha256").read_text().split()[0]
        root = install if sys.platform == "darwin" else launcher.parent
        check_projects(root / "examples")
        cli = tools / ("mtile.exe" if os.name == "nt" else "mtile")
        records = []
        with windows_offline(install, block_windows_network):
            for name in EXAMPLES[shard::SHARDS]:
                for attempt in range(1, ATTEMPTS + 1):
                    records.append(build(root, cli, work, name, attempt))
    output.parent.mkdir(parents=True, exist_ok=True)
    report = {"target": target, "shard": shard, "archive_sha256": archive_sha256, "records": records}
    output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--target", choices=TARGETS, required=True)
    parser.add_argument("--shard", type=int, choices=range(SHARDS), required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--block-windows-network", action="store_true",
                        help="block the candidate executables with Windows Firewall (administrator)")
    args = parser.parse_args()
    verify(args.archive, args.target, args.shard, args.output, args.block_windows_network)
