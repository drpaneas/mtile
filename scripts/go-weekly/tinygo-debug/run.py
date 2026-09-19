#!/usr/bin/env python3
"""Reproduce TinyGo 0.42 GBA logging in a private copy; never patch installation."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import statistics
import struct
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--tinygo", default=shutil.which("tinygo"))
    parser.add_argument("--work", type=Path)
    parser.add_argument("--samples", type=int, default=300)
    args = parser.parse_args()
    if args.samples <= 0:
        parser.error("--samples must be greater than zero")
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    work = (args.work or Path(tempfile.mkdtemp(prefix="mtile-tinygo-debug-"))).resolve()
    work.mkdir(parents=True, exist_ok=True)
    print(f"work directory: {work}", flush=True)
    events = []

    def run(command, label, cwd=ROOT, expect_failure=False):
        started = time.monotonic()
        result = subprocess.run(list(map(str, command)), cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        events.append({"label": label, "command": list(map(str, command)), "cwd": str(cwd), "seconds": time.monotonic()-started, "exitCode": result.returncode})
        (out / (label + ".log")).write_text(result.stdout + result.stderr)
        (out / "commands.json").write_text(json.dumps(events, indent=2) + "\n")
        if (result.returncode == 0) == expect_failure:
            raise RuntimeError(f"{label}: unexpected exit {result.returncode}; see {out / (label + '.log')}")
        return result

    original = Path(run([args.tinygo, "env", "TINYGOROOT"], "toolchain-root").stdout.strip()).resolve()
    # Homebrew's bin/tinygo script unconditionally sets TINYGOROOT. Bypass that
    # script, not the compiler, when selecting the private resource copy.
    compiler = original / "libexec" / "tinygo"
    if not compiler.is_file():
        compiler = Path(args.tinygo).resolve()
    watched = ["src/runtime/runtime_mgba.go", "src/runtime/runtime_arm7tdmi.go", "src/runtime/gc.go", "targets/gameboy-advance.json"]
    installed_before = {name: sha(original / name) for name in watched}
    clone = work / "tinygo"
    if clone.exists():
        raise RuntimeError("work/tinygo already exists; use a fresh work directory")
    run(["cp", "-cR", original, clone], "clone-toolchain")
    patchfile = clone / "src/runtime/runtime_mgba.go"
    source = patchfile.read_text()
    marker = "\nvar (\n"
    if source.count(marker) != 1 or "const zeroSizeAllocPtr" in source:
        raise RuntimeError("unexpected TinyGo runtime source; this reproducer targets 0.42.0")
    replacement = "\nconst zeroSizeAllocPtr uintptr = 16 // points somewhere in the BIOS which is not readable\n\nvar (\n"
    patchfile.write_text(source.replace(marker, replacement, 1))
    import difflib
    patch = "".join(difflib.unified_diff(source.splitlines(True), patchfile.read_text().splitlines(True), fromfile="a/src/runtime/runtime_mgba.go", tofile="b/src/runtime/runtime_mgba.go"))
    (out / "upstream.patch").write_text(patch)
    (out / "metadata.json").write_text(json.dumps({
        "originalRoot": str(original), "compiler": str(compiler), "compilerSHA256": sha(compiler),
        "cloneRoot": str(clone), "workDir": str(work), "installedBefore": installed_before,
        "tinygo": run([args.tinygo, "version"], "tinygo-version").stdout.strip(),
        "go": run(["go", "version"], "go-version").stdout.strip(),
        "playerSHA256": sha(ROOT / "build/player/mtile-player"),
        "gitRevision": run(["git", "rev-parse", "HEAD"], "revision").stdout.strip(),
        "gitStatus": run(["git", "status", "--porcelain"], "working-tree").stdout,
        "sourceHashes": {str(p.relative_to(ROOT)): sha(p) for tree in [ROOT/"internal/build", ROOT/"internal/transpiler", ROOT/"runtime"] for p in sorted(tree.rglob("*")) if p.is_file() and p.suffix in {".go", ".s", ".S", ".ld"}},
    }, indent=2)+"\n")
    wrappers = {}
    for name, resource_root, add_debug in [("original", original, False), ("original-debug", original, True), ("patched", clone, False), ("patched-debug", clone, True)]:
        path = work / name
        path.write_text("#!/usr/bin/env python3\nimport os, sys\n"
            + f"os.environ['TINYGOROOT'] = {str(resource_root)!r}\n"
            + "args = sys.argv[1:]\n"
            + ("if args and args[0] == 'build':\n    for i, value in enumerate(args):\n        if value.startswith('-tags='):\n            args[i] += ',mgbadebug'\n            break\n    else:\n        args.insert(1, '-tags=mgbadebug')\n" if add_debug else "")
            + f"os.execv({str(compiler)!r}, [{str(compiler)!r}] + args)\n")
        path.chmod(0o755)
        actual = run([path, "env", "TINYGOROOT"], name+"-root").stdout.strip()
        if Path(actual).resolve() != resource_root.resolve():
            raise RuntimeError("wrapper failed to select its resource root")
        wrappers[name] = path
    harness = work / "harness"
    run(["go", "build", "-o", harness, "./scripts/go-weekly/tinygo-debug"], "build-harness")
    allocchecker = work / "checkallocs"
    run(["go", "build", "-o", allocchecker, "./internal/cmd/checkallocs"], "build-allocation-checker", cwd=ROOT/"runtime")
    player = ROOT / "build/player/mtile-player"
    results = {"diagnostics": {}, "release": {}, "preview": {}}

    def play(rom, label):
        started = time.monotonic()
        proc = subprocess.run([str(player), str(rom)], input=bytes([1,0,0,0])*4+bytes([3,0,0,0]), stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=20)
        (out/(label+"-player.log")).write_bytes(proc.stderr)
        if proc.returncode != 0 or proc.stdout[:4] != b"GLUP":
            raise RuntimeError(f"invalid player run for {label}")
        offset = 16
        frames = []
        for number in range(4):
            magic, tick, audio, overflow, telemetry = struct.unpack_from("<4sIIII", proc.stdout, offset)
            if magic != b"FRAM" or tick != number or audio > 4096 or overflow or telemetry != 552:
                raise RuntimeError(f"invalid frame {number} for {label}")
            offset += 20
            pixels = proc.stdout[offset:offset+240*160*4]
            frames.append(hashlib.sha256(pixels).hexdigest())
            offset += 240*160*4+audio*4+telemetry
        if offset != len(proc.stdout):
            raise RuntimeError(f"invalid player output length for {label}")
        return {"logs": proc.stderr.decode(), "frames": frames, "hostSeconds": time.monotonic()-started}

    for case, expected in [("panic", "panic: MTILE_ARBITRARY_PANIC"), ("bounds", "index out of range")]:
        directory = work/case
        directory.mkdir()
        shutil.copy2(HERE/(case+".go.in"), directory/"main.go")
        (directory/"go.mod").write_text("module tinygo-debug-diagnostic\n\ngo 1.27.1\n")
        for variant in ["original", "original-debug", "patched-debug"]:
            target = directory/(variant+".json")
            run([harness,"-compiler",wrappers[variant],"-runtime",ROOT/"runtime","-target",target],case+"-"+variant+"-target")
            rom = directory/(variant+".gba")
            label = case+"-"+variant
            command = [wrappers[variant],"build","-gc=none","-opt=2","-panic=print","-print-allocs-cover=.","-target="+str(target),"-o",rom,"."]
            result = run(command,label,cwd=directory,expect_failure=variant=="original-debug")
            if variant == "original-debug":
                if "undefined: zeroSizeAllocPtr" not in result.stderr:
                    raise RuntimeError("failed for an unexpected reason")
                results["diagnostics"][label] = {"compileFailure": "undefined: zeroSizeAllocPtr"}
                continue
            run([allocchecker,out/(label+".log")],label+"-allocations")
            capture = play(rom,label)
            capture.update({"romSHA256":sha(rom),"romBytes":rom.stat().st_size})
            if variant == "patched-debug":
                if expected not in capture["logs"] or f"MTILE_{case.upper()}_READY" not in capture["logs"]:
                    raise RuntimeError(f"missing expected diagnostic: {capture['logs']}")
            elif "MTILE_" in capture["logs"] or "panic:" in capture["logs"]:
                raise RuntimeError("normal TinyGo unexpectedly emitted diagnostics")
            results["diagnostics"][label] = capture
            print(label+": expected logging behavior verified",flush=True)

    for mode in ["mode0","mode4"]:
        for variant,debug in [("original",False),("patched",False),("original",True),("patched-debug",True)]:
            label = mode+"-"+("preview-" if debug else "release-")+variant
            rom = work/(label+".gba")
            command = [harness,"-compiler",wrappers[variant],"-runtime",ROOT/"runtime","-project",ROOT/"examples/testio","-out",rom,"-renderer",mode]
            if debug: command += ["-debug","-player",player,"-samples",args.samples]
            result = json.loads(run(command,label).stdout)
            if not debug: result["capture"] = play(rom,label)
            results["preview" if debug else "release"][label] = result
            print(label+f": {result['romBytes']} ROM bytes, {result['iwramBytes']} IWRAM bytes",flush=True)
        release_before=results["release"][mode+"-release-original"]
        release_after=results["release"][mode+"-release-patched"]
        for field in ["romSHA256","romBytes","iwramBytes","ewramBytes","ewramFreeBytes","generatedGoSHA256"]:
            if release_before[field] != release_after[field]: raise RuntimeError(f"release {mode} differs: {field}")
        if release_before["capture"]["frames"] != release_after["capture"]["frames"]: raise RuntimeError("release frame mismatch")
        before=results["preview"][mode+"-preview-original"]
        after=results["preview"][mode+"-preview-patched-debug"]
        if before["generatedGoSHA256"] != after["generatedGoSHA256"]: raise RuntimeError("preview source changed")
        if before["ewramBytes"] != after["ewramBytes"]: raise RuntimeError("preview EWRAM changed")
        for left,right in zip(before["rows"],after["rows"],strict=True):
            for field in ["tick","rgbaSHA256","watches","audioOverflow"]:
                if left[field] != right[field]: raise RuntimeError(f"preview {mode} {field} mismatch at {left['tick']}")
            for field in ["GCCount","HeapAllocated","HeapLive","HeapPeakLive","HeapHandles","HeapError","MissedFrames"]:
                if left["stats"][field] != right["stats"][field]: raise RuntimeError(f"preview {mode} {field} mismatch")
        def timing(record):
            values=sorted(row["stats"]["LastFrame"] for row in record["rows"])
            return {"median":statistics.median(values),"p95":values[(len(values)*95+99)//100-1],"maximum":values[-1]}
        results.setdefault("comparison",{})[mode]={"releaseByteIdentical":True,"previewPixelsAndStateIdentical":True,"previewSamples":args.samples,"baselineFrameCycles":timing(before),"debugFrameCycles":timing(after),"debugROMGrowth":after["romBytes"]-before["romBytes"],"debugIWRAMGrowth":after["iwramBytes"]-before["iwramBytes"]}
        print(mode+": release bytes and preview pixels/state identical",flush=True)
    installed_after={name:sha(original/name) for name in watched}
    if installed_before != installed_after: raise RuntimeError("installed toolchain changed")
    results["installedToolchainUnchanged"] = installed_before == installed_after
    (out/"results.json").write_text(json.dumps(results,indent=2)+"\n")
    print(json.dumps(results["comparison"],indent=2),flush=True)


if __name__ == "__main__":
    main()
