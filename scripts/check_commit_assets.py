#!/usr/bin/env python3
"""Check the files in the Git index, including archives, before publishing."""

import bz2
import gzip
import hashlib
import io
import lzma
import os
import subprocess
import sys
import tarfile
import tempfile
import zipfile


CARTRIDGE_SUFFIXES = (".gba", ".gb", ".gbc", ".nes", ".sfc", ".smc", ".nds",
                      ".rom", ".p8", ".p8.png", ".bios", "_bios.bin", ".elf")
GBA_LOGO_SHA256 = "08a0153cfd6b0ea54b938f7d209933fa849da0d56f5a34c481060c9ff2fad818"
GB_LOGO_SHA256 = "daf4cabdc852baa0291849203f0b41fd0b4ecd58e0d7aff4a509f5de4d7f9a2e"
ARCHIVE_LIMIT = 512 * 1024 * 1024
INSPECTION_ERRORS = (ValueError, OSError, EOFError, RuntimeError, tarfile.TarError,
                     zipfile.BadZipFile, lzma.LZMAError)


def check_name(name):
    lower = name.lower()
    if lower.endswith((".7z", ".rar")):
        raise ValueError(f"unsupported archive format {name!r}")
    if lower.endswith(CARTRIDGE_SUFFIXES) or lower.rsplit("/", 1)[-1] == "bios.bin":
        raise ValueError(f"cartridge, BIOS or executable filename {name!r}")


def is_tar(header):
    if len(header) < 512:
        return False
    try:
        tarfile.TarInfo.frombuf(header, "utf-8", "surrogateescape")
    except tarfile.HeaderError:
        return False
    return True


def restricted_blobs():
    by_size = {}
    for row in git("cat-file", "blob", ":scripts/restricted_asset_blobs.txt").decode().splitlines():
        if row and not row.startswith("#"):
            oid, size, former_path = row.split(" ", 2)
            by_size.setdefault(int(size), {})[oid] = former_path
    return by_size


def check_fingerprint(data, restricted):
    matches = restricted.get(len(data))
    if matches:
        digest = hashlib.sha1(f"blob {len(data)}\0".encode(), usedforsecurity=False)
        digest.update(data)
        former_path = matches.get(digest.hexdigest())
        if former_path:
            raise ValueError(f"matches retired asset {former_path!r}")


def inspect_file(stream, name, restricted, size=None, depth=0):
    """Inspect file headers and identify exact copies of retired content."""
    if depth > 8:
        raise ValueError("archive nesting exceeds inspection limit")
    if size in restricted:
        # File size avoids hashing unrelated ZIP/TAR entries. Use Git's blob
        # framing to identify the same bytes in an archive or at a new path.
        data = stream.read(size)
        check_fingerprint(data, restricted)
        stream = io.BytesIO(data)
    reader = io.BufferedReader(stream)
    header = reader.peek(512)[:512]
    if header.startswith((b"7z\xbc\xaf\x27\x1c", b"Rar!\x1a\x07")):
        raise ValueError("unsupported archive signature")
    if header.startswith((b"NES\x1a", b"\x7fELF", b"pico-8 cartridge //")):
        raise ValueError("cartridge or ELF executable signature")
    if len(header) >= 192 and (
        hashlib.sha256(header[4:160]).hexdigest() == GBA_LOGO_SHA256
        or (header[3] == 0xea and header[0xb2] == 0x96
            and (sum(header[0xa0:0xbe]) + 0x19) % 256 == 0)
    ):
        raise ValueError("GBA cartridge header")
    if len(header) >= 0x150 and hashlib.sha256(header[0x104:0x134]).hexdigest() == GB_LOGO_SHA256:
        raise ValueError("Game Boy cartridge header")

    for magic, opener in ((b"\x1f\x8b", lambda r: gzip.GzipFile(fileobj=r)),
                          (b"BZh", bz2.BZ2File), (b"\xfd7zXZ\x00", lzma.LZMAFile)):
        if header.startswith(magic):
            inner = name.rsplit(".", 1)[0]
            check_name(inner)
            with opener(reader) as unpacked:
                inspect_file(unpacked, inner, restricted, depth=depth + 1)
            return

    is_zip = header.startswith((b"PK\x03\x04", b"PK\x05\x06"))
    if is_zip:
        # ZIP directories require seeking. A TAR member may be a forward-only stream.
        data = reader.read(ARCHIVE_LIMIT + 1)
        if len(data) > ARCHIVE_LIMIT:
            raise ValueError("ZIP exceeds inspection size limit")
        archive = zipfile.ZipFile(io.BytesIO(data))
        members = ((m.filename, m.file_size, m.is_dir(), m) for m in archive.infolist())
    elif is_tar(header):
        archive = tarfile.open(fileobj=reader, mode="r|")
        members = ((m.name, m.size, not m.isfile(), m) for m in archive)
    elif name.lower().endswith((".zip", ".tar", ".tgz", ".tbz2", ".txz")):
        # A TAR with only its zeroed end marker is a valid empty archive.
        if name.lower().endswith(".tar") and header == bytes(512):
            return
        raise ValueError("unrecognized archive contents")
    else:
        # Single compressed files have no reliable uncompressed size (gzip
        # can join members). Stop reading as soon as no fingerprint can match.
        if size is None and restricted:
            check_fingerprint(reader.read(max(restricted) + 1), restricted)
        return

    total = 0
    with archive:
        for count, (path, size, skip, member) in enumerate(members, 1):
            total += size
            if count > 20_000 or total > ARCHIVE_LIMIT:
                raise ValueError("archive exceeds inspection limits")
            if skip:
                continue
            try:
                check_name(path)
                with (archive.open(member) if is_zip else archive.extractfile(member)) as child:
                    inspect_file(child, path, restricted, size, depth + 1)
            except INSPECTION_ERRORS as exc:
                raise ValueError(f"{path!r}: {exc}") from exc


def git(*args, **kwargs):
    return subprocess.check_output(["git", *args], **kwargs)


def ignored_in_index(entries):
    # Git's ignore matcher reads worktree files. Give it precisely the staged
    # .gitignore files; personal excludes and unstaged edits cannot affect CI.
    ignores = [path for mode, _, path in entries
               if mode.startswith(b"100") and path.split(b"/")[-1] == b".gitignore"]
    git_dir = os.fsdecode(git("rev-parse", "--absolute-git-dir").rstrip(b"\n"))
    with tempfile.TemporaryDirectory() as tmp:
        if ignores:
            git("checkout-index", "-z", "--stdin", "--prefix=" + tmp + "/",
                input=b"\0".join(ignores) + b"\0")
        found = git("--git-dir=" + git_dir, "--work-tree=" + tmp, "ls-files", "-zci",
                    "--exclude-per-directory=.gitignore", cwd=tmp)
    return set(found.split(b"\0")[:-1])


def check_index():
    entries = []
    for record in git("ls-files", "--stage", "-z").split(b"\0")[:-1]:
        metadata, path = record.split(b"\t", 1)
        mode, oid, stage = metadata.split()
        if stage != b"0":
            raise ValueError(f"unmerged path {os.fsdecode(path)!r}")
        entries.append((mode, oid, path))

    ignored = ignored_in_index(entries)
    restricted = restricted_blobs()
    failed = False
    with subprocess.Popen(["git", "cat-file", "--batch"], stdin=subprocess.PIPE,
                          stdout=subprocess.PIPE) as objects:
        try:
            for mode, oid, raw_path in entries:
                path = os.fsdecode(raw_path)
                try:
                    if raw_path in ignored:
                        raise ValueError("excluded by staged .gitignore")
                    check_name(path)
                    if not mode.startswith(b"100"):
                        continue  # Symlink targets and submodule commits contain no file payload.
                    objects.stdin.write(oid + b"\n")
                    objects.stdin.flush()
                    info = objects.stdout.readline().split()
                    if len(info) != 3 or info[1] != b"blob":
                        raise ValueError(f"unavailable Git blob {oid.decode()}")
                    data = objects.stdout.read(int(info[2]))
                    if objects.stdout.read(1) != b"\n":
                        raise ValueError("incomplete Git object")
                    inspect_file(io.BytesIO(data), path, restricted, len(data))
                except INSPECTION_ERRORS as exc:
                    failed = True
                    print(f"{path!r}: {exc}", file=sys.stderr)
        finally:
            objects.stdin.close()
    return int(failed)


if __name__ == "__main__":
    try:
        if os.environ.get("GIT_INDEX_FILE"):
            os.environ["GIT_INDEX_FILE"] = os.path.abspath(os.environ["GIT_INDEX_FILE"])
        os.chdir(os.fsdecode(git("rev-parse", "--show-toplevel").rstrip(b"\n")))
        sys.exit(check_index())
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
