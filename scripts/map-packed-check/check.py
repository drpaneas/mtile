#!/usr/bin/env python3
"""Optional native packed-map span correctness, memory bounds, ABI and cycle check."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shlex
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
ENTRY = "mtile_copy_map_packed_spans"


def check(args, directory):
    original = args.candidate.read_text()
    marker = '.section .iwram.map_packed,'
    start = original.index(marker)
    end_marker = '.size ' + ENTRY + ', .-' + ENTRY
    end = original.index(end_marker, start) + len(end_marker)
    assembly = original[start:end] + "\n"
    source = directory / "packed.s"
    source.write_text(assembly)
    script = directory / "packed.ld"
    script.write_text("SECTIONS { .text 0x03001000 : { *(.iwram.map_packed) } }\n")
    obj, elf, binary = [directory / ("packed." + extension) for extension in ("o", "elf", "bin")]
    subprocess.run([args.tool_prefix + "as", "-mcpu=arm7tdmi", "-o", str(obj), str(source)], check=True)
    subprocess.run([args.tool_prefix + "ld", "-T", str(script), "-o", str(elf), str(obj)], check=True)
    subprocess.run([args.tool_prefix + "objcopy", "-O", "binary", str(elf), str(binary)], check=True)
    symbols = subprocess.check_output([args.tool_prefix + "nm", str(elf)], text=True)
    if not any(line.split() == ["03001000", "T", ENTRY] for line in symbols.splitlines()):
        raise ValueError("packed helper entry must be at 0x03001000")
    if not 0 < binary.stat().st_size <= 800:
        raise ValueError("packed helper exceeds the 800-byte IWRAM budget")
    (directory / "packed.asm").write_text(subprocess.check_output([args.tool_prefix + "objdump", "-d", str(elf)], text=True))
    library = args.mgba_build / "mgba" / "libmgba-core.a"
    if not library.is_file():
        raise ValueError(f"missing {library}; build mtile-player as described in README.md")
    executable = directory / "map-packed-check"
    compiler = shlex.split(args.cc)
    command = compiler + ["-std=c11", "-O3", "-Wall", "-Wextra", "-Werror",
        "-I" + str(args.mgba_source / "include"),
        "-I" + str(args.mgba_build / "mgba" / "include"),
        str(HERE / "check.c"), str(library), "-lm", "-pthread", "-o", str(executable)]
    if platform.system() == "Darwin":
        command += ["-framework", "Foundation"]
    subprocess.run(command, check=True)
    rom = directory / "dummy.gba"
    rom.write_bytes(bytes.fromhex("feffffea") + bytes(16380))
    result = subprocess.run([str(executable), str(binary), str(rom)], capture_output=True, text=True)
    report = result.stdout + result.stderr
    (directory / "report.txt").write_text(report)
    metadata = {"source": str(args.candidate), "assembly_sha256": hashlib.sha256(assembly.encode()).hexdigest(),
        "bytes": binary.stat().st_size, "mgba_library": str(library),
        "mgba_library_sha256": hashlib.sha256(library.read_bytes()).hexdigest(),
        "compiler": subprocess.check_output(compiler + ["--version"], text=True).splitlines()[0],
        "assembler": subprocess.check_output([args.tool_prefix + "as", "--version"], text=True).splitlines()[0],
        "exit_code": result.returncode}
    (directory / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    print(report, end="")
    print(f"Helper bytes: {metadata['bytes']}")
    if args.output_dir:
        print(f"Artifacts: {directory}")
    return result.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate", type=Path, default=ROOT / "runtime" / "crt0" / "gba.s")
    parser.add_argument("--mgba-source", type=Path, default=ROOT / "third_party" / "mgba")
    parser.add_argument("--mgba-build", type=Path, default=ROOT / "build" / "player")
    parser.add_argument("--tool-prefix", default="arm-none-eabi-")
    parser.add_argument("--cc", default=os.environ.get("CC", "cc"))
    parser.add_argument("--output-dir", type=Path, help="retain binaries, disassembly and reports here")
    args = parser.parse_args()
    for name in ("candidate", "mgba_source", "mgba_build"):
        setattr(args, name, getattr(args, name).resolve())
    try:
        if args.output_dir:
            args.output_dir = args.output_dir.resolve()
            args.output_dir.mkdir(parents=True, exist_ok=True)
            return check(args, args.output_dir)
        with tempfile.TemporaryDirectory(prefix="mtile-map-packed-check-") as temporary:
            return check(args, Path(temporary))
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(2, f"packed-map check: {error}\n")


if __name__ == "__main__":
    raise SystemExit(main())
