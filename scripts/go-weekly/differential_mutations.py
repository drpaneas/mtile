#!/usr/bin/env python3
"""Prove semantic detection in disposable source copies, never the worktree."""

import difflib
import json
import os
import re
from pathlib import Path
import shutil
import statistics
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "docs/benchmarks/go-weekly/differential"
OUTPUT.mkdir(parents=True, exist_ok=True)


def replace_once(source, old, new):
    if source.count(old) != 1:
        raise RuntimeError(f"mutation anchor count: {source.count(old)} for {old!r}")
    return source.replace(old, new, 1)


def reverse_sequence(source):
    start = source.index("func (g *generator) sequenceExprs(")
    end = source.index("\nfunc ", start + 1)
    body = replace_once(
        source[start:end],
        "for i, e := range exprs {",
        "for reverse := range exprs {\n\t\ti := len(exprs) - 1 - reverse\n\t\te := exprs[i]",
    )
    return source[:start] + body + source[end:]


mutations = [
    ("precedence", "internal/luaparser/parser.go", "^TestDifferentialParserCorpus$",
     lambda s: replace_once(s, 'return OpMultiply, 10, false', 'return OpMultiply, 9, false')),
    ("eager-condition", "internal/transpiler/conditions.go", "^TestDifferentialExecution$",
     lambda s: replace_once(s,
                           '\t\t\tg.pending = append(g.pending, "if "+guard+" {")',
                           '\t\t\tguard = "true"\n\t\t\tg.pending = append(g.pending, "if "+guard+" {")')),
    ("reversed-evaluation", "internal/transpiler/transpiler.go", "^TestDifferentialExecution$", reverse_sequence),
]

results = []
with tempfile.TemporaryDirectory(prefix="mtile-differential-mutants-") as directory:
    work = Path(directory)
    for name in ["internal", "runtime"]:
        shutil.copytree(ROOT / name, work / name)
    for name in ["go.mod", "go.sum"]:
        shutil.copy2(ROOT / name, work / name)

    def run(name, pattern):
        command = ["go", "test", "./internal/transpiler", "-run", pattern, "-count=1", "-v"]
        result = subprocess.run(command, cwd=work, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, text=True, timeout=180,
                                env={**os.environ, "GOCACHE": str(Path(tempfile.gettempdir()) / "mtile-generated-gocache")})
        (OUTPUT / f"{name}.log").write_text("$ " + " ".join(command) + "\n" + result.stdout)
        return result

    control = run("mutation-control", "^TestDifferential")
    if control.returncode:
        raise RuntimeError("unmutated control failed; inspect mutation-control.log")
    for name, filename, pattern, mutate in mutations:
        path = work / filename
        original = path.read_text()
        mutated = mutate(original)
        path.write_text(mutated)
        (OUTPUT / f"{name}.patch").write_text("".join(difflib.unified_diff(
            original.splitlines(True), mutated.splitlines(True), filename, filename)))
        try:
            result = run(name, pattern)
        finally:
            path.write_text(original)
        semantic_failure = (result.returncode != 0 and "semantic mismatch:" in result.stdout
                            and "[build failed]" not in result.stdout
                            and "not syntactically valid" not in result.stdout)
        results.append({"mutation": name, "exit_code": result.returncode,
                        "semantic_mismatch": semantic_failure})
        print(f"{name}: semantic detection={semantic_failure}", flush=True)
        if not semantic_failure:
            raise RuntimeError(f"{name} was not killed by a semantic mismatch")

    # This separately reproduces the real compile failure found by the oracle.
    # Compile rejection does NOT count as a killed semantic mutant above.
    path = work / "internal/transpiler/transpiler.go"
    original = path.read_text()
    regressed = replace_once(original,
        '\t\t\ttmp := g.next("neg")\n\t\t\tg.pending = append(g.pending, tmp+" := "+value)\n\t\t\treturn "-(" + tmp + ")"',
        '\t\t\tif literal, ok := x.Expr.(*luaparser.NumberExpr); ok && fixedNumberLiteral(literal.Value) == -2147483648 {\n\t\t\t\treturn "rt.Number(-2147483648)"\n\t\t\t}\n\t\t\treturn "-(" + value + ")"')
    path.write_text(regressed)
    (OUTPUT / "minimum-negation-regression.patch").write_text("".join(difflib.unified_diff(
        original.splitlines(True), regressed.splitlines(True), str(path.relative_to(work)), str(path.relative_to(work)))))
    result = run("minimum-negation-regression", "^TestDifferentialExecution$")
    reproduced = result.returncode != 0 and "overflows int32" in result.stdout
    results.append({"mutation": "minimum-negation-regression", "exit_code": result.returncode,
                    "known_compile_failure_reproduced": reproduced})
    if not reproduced:
        raise RuntimeError("minimum negation regression did not reproduce")
    path.write_text(original)

    if "--benchmark" in sys.argv:
        # Same corpus/test code in both states; generated Go is not compiled by
        # this benchmark. Alternate six paired samples to reduce time drift.
        samples = {"before": [], "after": []}
        for state in samples:
            (OUTPUT / f"transpile-{state}.log").write_text("")
        for pair in range(6):
            for state in (["before", "after"] if pair % 2 == 0 else ["after", "before"]):
                path.write_text(regressed if state == "before" else original)
                command = ["go", "test", "./internal/transpiler", "-run", "^$",
                           "-bench", "^BenchmarkDifferentialCorpusTranspile$",
                           "-benchtime=300ms", "-count=1", "-cpu=1"]
                result = subprocess.run(command, cwd=work, stdout=subprocess.PIPE,
                                        stderr=subprocess.STDOUT, text=True, timeout=180,
                                        env={**os.environ, "GOCACHE": str(Path(tempfile.gettempdir()) / "mtile-generated-gocache")})
                with (OUTPUT / f"transpile-{state}.log").open("a") as log:
                    log.write("$ " + " ".join(command) + "\n" + result.stdout)
                match = re.search(r"BenchmarkDifferentialCorpusTranspile\s+\d+\s+(\d+) ns/op\s+[\d.]+ MB/s\s+(\d+) B/op\s+(\d+) allocs/op", result.stdout)
                if result.returncode or not match:
                    raise RuntimeError(f"benchmark failed for {state}: {result.stdout}")
                samples[state].append(dict(zip(["ns/op", "B/op", "allocs/op"], map(int, match.groups()))))
                print(f"pair {pair + 1} {state}: {match.group(1)} ns/op", flush=True)
        (OUTPUT / "transpile-samples.json").write_text(json.dumps(samples, indent=2) + "\n")
        summary = {state: {key: statistics.median(row[key] for row in rows)
                           for key in rows[0]} for state, rows in samples.items()}
        summary["change_percent"] = {key: (summary["after"][key] / summary["before"][key] - 1) * 100
                                     for key in summary["before"]}
        (OUTPUT / "transpile-summary.json").write_text(json.dumps(summary, indent=2) + "\n")

(OUTPUT / "mutations.json").write_text(json.dumps(results, indent=2) + "\n")
print("All three semantic mutants detected; independent compile regression reproduced.")
