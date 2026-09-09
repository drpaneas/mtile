#!/usr/bin/env python3
"""Optional ARM7TDMI division correctness, ABI, and cycle regression gate."""

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
ENTRY_NAMES = (
    "__aeabi_uidivmod",
    "__aeabi_idivmod",
    "__aeabi_uldivmod",
    "__aeabi_ldivmod",
)


def output(command):
    return subprocess.check_output(command, text=True)


def assemble(source, destination, base, prefix):
    destination.mkdir(parents=True, exist_ok=True)
    original = source.read_text()
    marker = ".section .iwram.uidiv"
    if marker not in original:
        raise ValueError(f"{source} does not contain the IWRAM division helpers")
    assembly = original[original.index(marker):]
    source_path = destination / "division.s"
    source_path.write_text(assembly)
    script = destination / "division.ld"
    script.write_text(f"SECTIONS {{ .text {base:#x} : {{ *(.iwram.*) }} }}\n")
    obj = destination / "division.o"
    elf = destination / "division.elf"
    binary = destination / "division.bin"
    subprocess.run([prefix + "as", "-mcpu=arm7tdmi", "-o", str(obj), str(source_path)], check=True)
    subprocess.run([prefix + "ld", "-T", str(script), "-o", str(elf), str(obj)], check=True)
    subprocess.run([prefix + "objcopy", "-O", "binary", str(elf), str(binary)], check=True)
    symbols = {}
    for line in output([prefix + "nm", str(elf)]).splitlines():
        fields = line.split()
        if len(fields) == 3:
            symbols[fields[2]] = int(fields[0], 16)
    for alias, entry in (
        ("__aeabi_uidiv", "__aeabi_uidivmod"),
        ("__udivsi3", "__aeabi_uidivmod"),
        ("__aeabi_idiv", "__aeabi_idivmod"),
        ("__divsi3", "__aeabi_idivmod"),
    ):
        if symbols.get(alias) != symbols.get(entry):
            raise ValueError(f"{source}: {alias} must alias {entry}")
    size = binary.stat().st_size
    if not 0 < size < 8192:
        raise ValueError(f"{source}: {size} bytes do not fit the isolated helper region")
    for name in ENTRY_NAMES:
        if name not in symbols or not base <= symbols[name] < base + size:
            raise ValueError(f"{source}: invalid helper entry {name}")
    (destination / "division.asm").write_text(output([prefix + "objdump", "-d", str(elf)]))
    return [str(binary)] + [f"{symbols[name]:x}" for name in ENTRY_NAMES], {
        "source": str(source),
        "assembly_sha256": hashlib.sha256(assembly.encode()).hexdigest(),
        "bytes": size,
        "symbols": {name: f"{address:#x}" for name, address in symbols.items()},
    }


def check(args, directory):
    compiler = shlex.split(args.cc)
    library = args.mgba_build / "mgba" / "libmgba-core.a"
    if not library.is_file():
        raise ValueError(f"missing {library}; build mtile-player as described in README.md")
    executable = directory / "division-check"
    command = compiler + [
        "-std=c11", "-O3", "-Wall", "-Wextra", "-Werror",
        "-I" + str(args.mgba_source / "include"),
        "-I" + str(args.mgba_build / "mgba" / "include"),
        str(HERE / "check.c"), str(library), "-lm", "-pthread",
        "-o", str(executable),
    ]
    if platform.system() == "Darwin":
        command += ["-framework", "Foundation"]
    subprocess.run(command, check=True)
    invocation = [str(executable)]
    metadata = {
        "compiler": output(compiler + ["--version"]).splitlines()[0],
        "assembler": output([args.tool_prefix + "as", "--version"]).splitlines()[0],
        "mgba_library": str(library),
        "mgba_library_sha256": hashlib.sha256(library.read_bytes()).hexdigest(),
    }
    for index, (name, source) in enumerate((
        ("baseline", args.baseline), ("candidate", args.candidate),
    )):
        operands, metadata[name] = assemble(
            source, directory / name, 0x03001000 + index * 0x2000, args.tool_prefix,
        )
        invocation += operands
    # A tiny ROM establishes mGBA's memory map; no ROM instructions are executed.
    rom = directory / "dummy.gba"
    rom.write_bytes(bytes.fromhex("feffffea") + bytes(16380))
    invocation.append(str(rom))
    result = subprocess.run(invocation, capture_output=True, text=True)
    report = result.stdout + result.stderr
    metadata["exit_code"] = result.returncode
    (directory / "report.txt").write_text(report)
    (directory / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    print(report, end="")
    print(f"Helper bytes: {metadata['baseline']['bytes']} -> {metadata['candidate']['bytes']}")
    if args.output_dir:
        print(f"Artifacts: {directory}")
    return result.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", type=Path, default=HERE / "fixtures" / "restoring.s")
    parser.add_argument("--candidate", type=Path, default=ROOT / "runtime" / "crt0" / "gba.s")
    parser.add_argument("--mgba-source", type=Path, default=ROOT / "third_party" / "mgba")
    parser.add_argument("--mgba-build", type=Path, default=ROOT / "build" / "player")
    parser.add_argument("--tool-prefix", default="arm-none-eabi-")
    parser.add_argument("--cc", default=os.environ.get("CC", "cc"))
    parser.add_argument("--output-dir", type=Path, help="retain binaries, disassembly, and reports here")
    args = parser.parse_args()
    for name in ("baseline", "candidate", "mgba_source", "mgba_build"):
        setattr(args, name, getattr(args, name).resolve())
    try:
        if args.output_dir:
            args.output_dir = args.output_dir.resolve()
            args.output_dir.mkdir(parents=True, exist_ok=True)
            return check(args, args.output_dir)
        with tempfile.TemporaryDirectory(prefix="mtile-division-check-") as temporary:
            return check(args, Path(temporary))
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(2, f"division check: {error}\n")


if __name__ == "__main__":
    raise SystemExit(main())
