#!/usr/bin/env python3
"""Opt-in mGBA idle, blend, and HBlank trials. Mutates only an isolated output copy."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent


def run(command, cwd, log=None, env=None, check=True):
    result = subprocess.run(command, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if log:
        log.write_text(result.stdout)
    if check and result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {command}\n{result.stdout[-5000:]}")
    return result


def compile_c(root, source, binary):
    command = [os.environ.get("CC", "cc"), "-std=c11", "-O3", "-Wall", "-Wextra", "-Werror",
               "-I" + str(root / "third_party/mgba/include"),
               "-I" + str(root / "build/player/mgba/include"),
               str(HERE / source), str(root / "build/player/mgba/libmgba-core.a"),
               "-lm", "-pthread", "-o", str(binary)]
    if platform.system() == "Darwin":
        command += ["-framework", "Foundation"]
    run(command, root)


def video(root, out):
    binary = out / "video-controls"
    compile_c(root, "video_controls.c", binary)
    dummy = out / "dummy.gba"
    dummy.write_bytes(bytes.fromhex("feffffea") + bytes(16380))
    result = run([str(binary), str(dummy)], root, out / "video-controls.log")
    print(result.stdout)


def capture(root, out, variant, kind):
    name = f"{variant}-{kind}"
    symbols = {}
    for line in (out / (name + ".map")).read_text().splitlines():
        fields = line.split()
        if len(fields) >= 4 and fields[-1].startswith("main."):
            symbols[fields[-1]] = int(fields[0], 16)
    if kind == "idle":
        addresses = [symbols["main.probe"] + i * 4 for i in range(2048)]
    else:
        addresses = [addr for symbol, addr in symbols.items() if symbol.startswith("main.psgCoexist")]
    result = run([str(out / "capture"), str(out / (name + ".gba")),
                  "650" if kind == "idle" else "1200"] + [hex(a) for a in addresses],
                 root, out / (name + ".log"))
    line = next(line for line in result.stdout.splitlines() if line.startswith("S "))
    counts = {key: int(value) for key, value in re.findall(r"(\w+)=(\d+)", line)}
    counts["haltedPercent"] = 100 * counts["halted"] / counts["samples"]
    counts["symbols"] = symbols
    counts["words"] = {line.split()[1]: line.split()[2] for line in result.stdout.splitlines() if line.startswith("W ")}
    return counts


def sleep(root, out):
    work = out / "isolated"
    # Do not copy build products or Git metadata. Shared dependencies are read-only
    # symlinks; every target, generated cartridge, source edit, and log is private.
    excluded = {"build", "third_party", "trial-package", "experiments", "trial-artifacts"}
    def ignore(path, names):
        # Only the repository-level build directory is generated. internal/build
        # is production source and its templates are required by package tests.
        skip = {".git", "node_modules"}
        if Path(path).resolve() == root:
            skip |= excluded
        return [name for name in names if name in skip]
    shutil.copytree(root, work, symlinks=True, ignore=ignore)
    for name in ("build", "third_party"):
        (work / name).symlink_to(root / name, target_is_directory=True)
    runtime = work / "runtime"
    probe = runtime / "cmd/idleprobe"
    probe.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(HERE / "idleprobe.go", probe / "main.go")
    shutil.copyfile(HERE / "idle_trial_clock_test.go.in", runtime / "idle_trial_clock_test.go")
    run(["go", "test", "./..."], runtime, out / "baseline-runtime-tests.log")
    run(["go", "run", "./internal/cmd/gentarget"], runtime, out / "target.log")
    compile_c(root, "capture.c", out / "capture")
    report = {}
    for variant in ("baseline", "candidate"):
        if variant == "candidate":
            run(["patch", "-p1", "-i", str(HERE / "idle-candidate.patch")], work, out / "patch.log")
        for kind, entry in (("idle", "idleprobe"), ("audio", "audiopsgcoexist")):
            name = f"{variant}-{kind}"
            run(["tinygo", "build", "-gc=none", "-opt=2", "-target=target-gba.json",
                 "-ldflags=-extldflags=-Map=" + str(out / (name + ".map")),
                 "-o", str(out / (name + ".gba")), "./cmd/" + entry],
                runtime, out / (name + "-build.log"))
            report[name] = capture(root, out, variant, kind)
    run(["go", "test", "./..."], runtime, out / "candidate-runtime-tests.log")
    run(["go", "test", "./internal/transpiler", "./internal/build", "./cmd/mtile"],
        work, out / "candidate-root-tests.log")
    shutil.copyfile(HERE / "idle_trial_diagnostic_test.go.in", work / "cmd/renderbench/idle_trial_diagnostic_test.go")
    env = dict(os.environ, MTILE_PERF_BASELINE_RUNTIME=str(root / "runtime"), MTILE_IDLE_OUT=str(out / "games"))
    result = run(["go", "test", "-tags=runtimeperfdiagnostic", "./cmd/renderbench",
                  "-run", "^TestIdleTrialDiagnostic$", "-count=1", "-v"],
                 work, out / "games.log", env=env, check=False)
    report["pairedGameTestsPass"] = result.returncode == 0
    report["keep"] = False
    report["decision"] = "Research-only: evaluate all native gates. Historical candidate failed Mode4 background-layers."
    (out / "sleep-report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(result.stdout)
    # The native diagnostic's historical failure is expected experimental output,
    # not authorization to install the candidate in the source repository.


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("trial", choices=("video", "sleep", "both"))
    parser.add_argument("--root", type=Path, default=HERE.parents[2])
    parser.add_argument("--out", type=Path)
    args = parser.parse_args()
    root = args.root.resolve()
    out = args.out.resolve() if args.out else Path(tempfile.mkdtemp(prefix="mtile-idle-video-"))
    out.mkdir(parents=True, exist_ok=True)
    library = root / "build/player/mgba/libmgba-core.a"
    metadata = {"root": str(root), "librarySHA256": hashlib.sha256(library.read_bytes()).hexdigest(),
                "tinygo": run(["tinygo", "version"], root).stdout.strip(), "trial": args.trial}
    (out / "environment.json").write_text(json.dumps(metadata, indent=2) + "\n")
    if args.trial in ("video", "both"):
        video(root, out)
    if args.trial in ("sleep", "both"):
        sleep(root, out)
    print("Artifacts:", out)


if __name__ == "__main__":
    main()
