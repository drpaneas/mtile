# Differential expression experiment — 2026-09-19

**Decision: retain the bounded test oracle and the signed-minimum negation fix.**
The first generated execution corpus exposed valid Lua that transpiled into an
overflowing Go constant. The tests also detect three deliberately introduced
semantic defects. This is a correctness and coverage improvement; no runtime
speedup is claimed.

The independent test model covers nil, booleans, signed 16:16 numbers, addition,
subtraction, multiplication, comparisons, unary negation/not, operand-returning
and/or, and ordered side effects. It uses int64 intermediates and explicit int32
wrapping, with no runtime arithmetic/truthiness helper or parser precedence-table
reuse. Expressions are serialized with necessary parentheses rather than fully
parenthesizing every operation. No new dependency or public Lua API was added.

- `FuzzDifferentialParser` compares the original model's value and complete
  side-effect trace with an independently traversed parsed AST. Inputs are capped
  at 64 bytes; the generated model has depth three and at most fifteen nodes.
  This fast fuzz target checks parsing, **not generated-code execution**.
- `TestDifferentialExecution` batches 14 explicit expression witnesses and 128
  deterministic generated expressions (seed `0x6d74696c65`) into one generated
  package per configuration. Each expression runs in both value and condition
  contexts. Five additional witnesses cover compound-index evaluation,
  parenthesized/inlined minimum values, numeric-call sequencing and skipped
  numeric calls. Both optimized and generic configurations compare against
  independently expected values/traces: 289 checks each, 578 total.
- The generic configuration disables monomorphic numbers/functions, numeric
  constants, and condition specialization. It is compared against the oracle,
  not merely against the optimized configuration.

The discovered defect is exemplified by `return -(-32768)`, `return -(32768)`,
and `minimum=-32768; function probe() return -minimum end`. Negating an emitted
`rt.Number(-2147483648)` Go constant overflows int32 even though the dialect's
16:16 result wraps to the same value. The previous direct-literal guard missed
parentheses, nested negation, and inlined numeric globals. The narrow fix in
`emitNumberExpr` binds the numeric operand to a temporary before negation, so Go
uses variable wrapping semantics and evaluates the operand exactly once.
The [isolated production diff](unary-negation-fix.patch) excludes pre-existing
working-tree changes in the same file.

The mutation runner copies the current `internal`, `runtime`, and module files
into a disposable directory. It restores each mutation before applying the next.
Its three semantic mutation checks explicitly reject compile failures as proof:

| Compiling mutant | Observed semantic failure |
| --- | --- |
| Multiplication precedence lowered to addition | `1 + 2 * 3` produces 9 instead of 7. |
| Condition RHS always evaluated | `true or mark(1,9)` returns the expected truth value but incorrectly records `[1]`. |
| Operand evaluation reversed | `mark(1,1) + mark(2,2)` returns 3 but records `[2,1]` instead of `[1,2]`. |

[Raw mutation results](mutations.json), individual `.patch`/`.log` files, and the
separate [minimum-negation regression log](minimum-negation-regression.log)
preserve the proof. The latter intentionally reproduces the real compile
failure and is **not** counted as a killed semantic mutant.

The 30-second single-worker parser fuzz run passed **436,272 executions**, adding
67 interesting inputs beyond 33 seeds, with no failure ([fuzz.log](fuzz.log)).
That execution count is a bounded coverage exercise, not a claim of exhaustive
Lua correctness. Strings, division/modulo/power, arbitrary programs, GC stress,
and actual GBA execution are outside this oracle's scope.

On the final narrow check, optimized/generic transpilation took 6.10/4.73 ms;
formatting, building, and executing each generated package took 1.35/1.88 s.
The full new deterministic tests took 3.24 s inside the package
([final log](final-differential-tests.log)). These are one-run test costs,
separate from the steady compiler benchmark below.

Six paired, alternating benchmark samples compare the previous unary emitter
against the fix, holding the corpus and all test code constant. Each sample uses
`-cpu=1 -benchtime=300ms -count=1`, with corpus/oracle construction outside timing.
The host was Apple M2 Pro, darwin/arm64, using Go 1.27.1.
Only `Transpile` and retaining its output are timed; generated Go is not compiled
inside the benchmark. Consequently the pre-fix compiler can be measured even
though its output fails to compile for the boundary witnesses.

| Median metric | Before fix | After fix | Change |
| --- | ---: | ---: | ---: |
| Host transpile time | 5.374 ms | 5.481 ms | +1.98% |
| Allocated bytes/op | 6,010,611 | 6,066,817.5 | +0.94% |
| Allocations/op | 63,305 | 65,506 | +3.48% |

The timing ranges overlap; this small sample does not establish a significant
timing regression. The extra host allocations are a measured cost of emitting
temporaries in this dense expression corpus. Raw
[before](transpile-before.log)/[after](transpile-after.log) logs,
[samples](transpile-samples.json), and [medians](transpile-summary.json) are kept.
The added test helpers are absent from production binaries.

Independent TinyGo builds of the existing `testio` cartridge are byte-identical
to the pre-fix baseline: Mode 0 **24,376 bytes**, Mode 4 **26,356 bytes**
([native hashes and results](native/results.json)). This controls ordinary
generated-code cost on those two fixtures; it does not claim every program's
binary is unchanged.

Validation is a composite gate: the full transpiler suite passed in 304.5 s;
the final two test-only numeric-call witnesses then passed in the final narrow
run. All other root packages passed with compiler-cache/CoreMIDI access
([root package log](root-other-tests.log)). Both
[default runtime](runtime-tests.log) and [Mode 0 runtime](runtime-mode0-tests.log)
suites passed. The first full root invocation hit sandbox restrictions in
TinyGo cache writes and CoreMIDI; [its raw log](root-tests-sandbox.log) preserves
those environmental failures and the successful full transpiler run. An
unnecessarily repeated full transpiler invocation was deliberately stopped
after its sibling packages passed
([stopped repeat](root-tests-redundant-stopped.log)); no production code changed
after the successful full transpiler run.

Reproduce from the repository root:

```sh
go test ./internal/transpiler -run '^TestDifferential' -count=1 -v
go test ./internal/transpiler -run '^$' -fuzz '^FuzzDifferentialParser$' -fuzztime=30s -parallel=1
python3 scripts/go-weekly/differential_mutations.py --benchmark
```

The mutation script uses a writable temporary Go cache and rewrites only its
disposable copies and this evidence directory. Normal `go test` runs the fixed
corpus and fuzz seeds; it does not invoke the mutation script or a timed fuzz run.
