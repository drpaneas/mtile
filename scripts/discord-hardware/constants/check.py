#!/usr/bin/env python3
"""Measure two ARM constant-loading sequences in the pinned native mGBA core."""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import re
import shlex
import shutil
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]


def output(command):
    return subprocess.check_output(command, text=True)


def run(args, directory):
    library = args.mgba_build / "mgba/libmgba-core.a"
    if not library.is_file():
        raise ValueError(f"missing {library}; build the headless mtile-player first")
    source = directory / "materialize.s"
    shutil.copy2(HERE / "materialize.s", source)
    obj, binary = directory / "materialize.o", directory / "materialize.bin"
    subprocess.run([args.tool_prefix + "as", "-mcpu=arm7tdmi", "-o", str(obj), str(source)], check=True)
    subprocess.run([args.tool_prefix + "objcopy", "-O", "binary", str(obj), str(binary)], check=True)
    code = binary.read_bytes()
    if len(code) != 24:
        raise ValueError(f"expected two 12-byte routines, got {len(code)} bytes")
    symbols = {}
    for line in output([args.tool_prefix + "nm", str(obj)]).splitlines():
        fields = line.split()
        if len(fields) == 3:
            symbols[fields[2]] = int(fields[0], 16)
    if symbols.get("literal") != 0 or symbols.get("immediate") != 12:
        raise ValueError("the native runner expects literal=0 and immediate=12")
    (directory / "materialize.asm").write_text(output([args.tool_prefix + "objdump", "-d", str(obj)]))
    rom = directory / "trial.gba"
    # The runner executes only the injected routines, not this dummy entrypoint.
    rom.write_bytes(bytes.fromhex("feffffea") + bytes(4092) + code + bytes(4096))
    executable = directory / "check"
    compiler = shlex.split(args.cc)
    command = compiler + [
        "-std=c11", "-O3", "-Wall", "-Wextra", "-Werror",
        "-I" + str(args.mgba_source / "include"),
        "-I" + str(args.mgba_build / "mgba/include"),
        str(HERE / "check.c"), str(library), "-lm", "-pthread",
        "-o", str(executable),
    ]
    if platform.system() == "Darwin":
        command += ["-framework", "Foundation"]
    subprocess.run(command, check=True)
    result = subprocess.run([str(executable), str(rom)], capture_output=True, text=True)
    report = result.stdout + result.stderr
    (directory / "report.txt").write_text(report)
    measurements = []
    for match in re.finditer(r"(ROM|IWRAM) (literal_load|two_immediates) n=(\d+) min=(\d+) max=(\d+) mean=([\d.]+) bytes=(\d+)", report):
        location, sequence, count, minimum, maximum, mean, size = match.groups()
        measurements.append({"location": location, "sequence": sequence, "executions": int(count), "min_cycles": int(minimum), "max_cycles": int(maximum), "mean_cycles": float(mean), "bytes": int(size)})
    metadata = {
        "environment": "native mGBA ARM7TDMI instruction execution; WAITCNT=0x4317; no physical-hardware measurement",
        "constant": "0x040000de",
        "checks": "value, r1-r12, stack pointer, condition flags, ARM return state",
        "measurements": measurements,
        "compiler": output(compiler + ["--version"]).splitlines()[0],
        "assembler": output([args.tool_prefix + "as", "--version"]).splitlines()[0],
        "source_sha256": {name: hashlib.sha256((HERE / name).read_bytes()).hexdigest() for name in ("materialize.s", "check.c")},
        "mgba_library": str(library),
        "mgba_sha256": hashlib.sha256(library.read_bytes()).hexdigest(),
        "exit_code": result.returncode,
    }
    (directory / "report.json").write_text(json.dumps(metadata, indent=2) + "\n")
    print(report, end="")
    print("Artifacts:", directory)
    if result.returncode:
        return result.returncode
    if len(measurements) != 4 or "PASS:" not in report:
        raise ValueError("native checker produced incomplete telemetry")
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mgba-source", type=Path, default=ROOT / "third_party/mgba")
    parser.add_argument("--mgba-build", type=Path, default=ROOT / "build/player")
    parser.add_argument("--tool-prefix", default="arm-none-eabi-")
    parser.add_argument("--cc", default="cc")
    parser.add_argument("--output-dir", type=Path, help="new directory outside the repository for retained artifacts")
    args = parser.parse_args()
    args.mgba_source = args.mgba_source.resolve()
    args.mgba_build = args.mgba_build.resolve()
    try:
        if args.output_dir:
            args.output_dir = args.output_dir.resolve()
            if args.output_dir == ROOT or ROOT in args.output_dir.parents:
                raise ValueError("--output-dir must be outside the repository")
            args.output_dir.mkdir(parents=True, exist_ok=False)
            return run(args, args.output_dir)
        with tempfile.TemporaryDirectory(prefix="mtile-constant-check-") as temporary:
            return run(args, Path(temporary))
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(2, f"constant check: {error}\n")


if __name__ == "__main__":
    raise SystemExit(main())
