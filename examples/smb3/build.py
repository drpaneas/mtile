#!/usr/bin/env python3
"""Build the GBA compatibility cartridge from a local SMB3 NES ROM."""

import argparse
import hashlib
import io
import os
from pathlib import Path
import struct
import tempfile
from urllib.request import urlopen
import zipfile


HERE = Path(__file__).resolve().parent
RELEASE = "https://github.com/Dwedit/PocketNES/releases/download/1-4-2020-B/PocketNES.1-4-2020.zip"
ARCHIVE_SHA256 = "900c30a2d8a610c1a97840c5d82f04876c02bd62dddf24f7919e17db9e9074a7"
CORE_SHA256 = "86eb6e690b0ff4ab089a6cb78b0bf7cab1f4e96dd8e62df2ae9325582c3673e2"
NES_SHA256 = "4377a7f5e6eb50bdd2ac6f249bf1a7085500aca8eb41f38545c3a2731c51a579"
NES_SIZE = 393232
CART_SHA256 = "8df99dded49916e382814272833d947c513040bd244084d5f3193033255491ff"


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def load_core(local_path):
    if local_path is None:
        print("Downloading the pinned PocketNES release; your NES ROM stays local.", flush=True)
        with urlopen(RELEASE, timeout=30) as response:
            data = response.read(1024 * 1024)
    else:
        data = local_path.read_bytes()
    fingerprint = sha256(data)
    if fingerprint == ARCHIVE_SHA256:
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            data = archive.read("pocketnes.gba")
        fingerprint = sha256(data)
    if fingerprint != CORE_SHA256:
        raise ValueError("PocketNES SHA-256 mismatch: use the pinned release ZIP or its pocketnes.gba.")
    return data


def write_atomic(path, data):
    with tempfile.NamedTemporaryFile(dir=path.parent, prefix=".smb3-", delete=False) as output:
        staging = Path(output.name)
    try:
        staging.write_bytes(data)
        os.replace(staging, path)
    finally:
        staging.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("rom", type=Path, help="path to your own, matching mario3.nes")
    parser.add_argument("--core", type=Path, help="local pinned PocketNES release ZIP or pocketnes.gba (no network)")
    parser.add_argument("-o", "--output", type=Path, default=HERE / "game.gba", help="output path (default: game.gba beside this script)")
    args = parser.parse_args()
    try:
        if args.output.resolve() == args.rom.resolve() or (args.core and args.output.resolve() == args.core.resolve()):
            raise ValueError("choose an output path separate from the ROM and PocketNES inputs.")
        if args.rom.stat().st_size != NES_SIZE:
            raise ValueError(f"unsupported NES ROM: expected {NES_SIZE} bytes.")
        nes = args.rom.read_bytes()
        if sha256(nes) != NES_SHA256:
            raise ValueError(f"unsupported NES ROM: expected SHA-256 {NES_SHA256}.")
        core = load_core(args.core)
        # PocketNES RomHeader: title[32], filesize, flags, sprite follow and reserved.
        # A single appended entry autoloads on boot. Keep all iNES bytes intact.
        header = struct.pack("<32sIIII", b"Super Mario Bros. 3", len(nes), 0, 0, 0)
        cartridge = core + header + nes
        fingerprint = sha256(cartridge)
        if fingerprint != CART_SHA256:
            raise ValueError("unexpected output fingerprint; the cartridge was not written.")
        write_atomic(args.output, cartridge)
        print(f"Built {args.output.resolve()} ({len(cartridge):,} bytes)")
        print(f"SHA-256 {fingerprint}")
    except (OSError, ValueError, zipfile.BadZipFile) as error:
        parser.exit(1, f"build: {error}\n")


if __name__ == "__main__":
    main()
