# Scalar table hash fingerprint — rejected as a default optimization

The candidate passed correctness and all twelve native microbenchmark gates,
but its benefit is confined to deliberately colliding heap-string keys. No
existing example workload demonstrated that gain. Ordinary construction became
slower, and linked code consumed additional ROM and scarce IWRAM. The default
runtime was therefore restored exactly; this directory retains the experiment.
The independent reviewer and parent review found no implementation blocker.

## Candidate and correctness

The high byte of FNV-1a is stored in byte 1 of each dynamic string key's existing
twelve-byte Value slot. This uses padding, so table capacity, heap use and iteration
order are unchanged. String lookups reject differing tags before full string
comparison; equal tags, including zero, still compare complete contents. Static
stored keys bypass the filter. TableSet and growth write tags after copying the
Value; raw constructor images normalize copied heap-string keys without allocating.

The [patch](candidate.patch) touches only `runtime/table.go` and
`runtime/table_literal_image.go`. The archived
[tests and cold benchmark](hash_fingerprint_test.go.in) independently check FNV
tags, static/heap equality, collisions, equal/zero tags, empty and embedded-NUL
strings, deletion/refill/growth, iteration order across moving GC, and poisoned
raw-image padding across allocation-triggered and explicit collections.
Embedded image references are rooted before allocation, respecting the existing
constructor contract. Full runtime suites in both render modes and the focused
race suite passed before performance testing.

## Native result

Go 1.27.1, TinyGo 0.42.0; native mGBA, not physical GBA hardware. Each variant has
20 warmup updates followed by 600 synchronized updates. All twelve pairs had
identical generated Go, every pixel, independent numeric watches, heap and EWRAM.
There were zero missed updates during warmup or measurement, and no GC in the
three dynamic-string fixtures. The complete arrays are in `native/*/*.json.gz`;
[summary and artifact hashes](native-summary.json) and [run log](native.log)
record the measurements.

| Workload | Mode 0 median cycles, baseline → candidate | Mode 0 p95 change | Mode 4 median cycles, baseline → candidate | Mode 4 p95 change |
| --- | ---: | ---: | ---: | ---: |
| Colliding dynamic strings | 317,905 → 81,803 | −76.61% | 459,820 → 221,030 | −55.19% |
| Numeric control | 174,031 → 174,031 | 0% | 315,861 → 315,861 | 0% |
| Static-string control | 61,661 → 61,885 | +0.36% | 197,725 → 197,949 | +0.11% |
| Distributed dynamic strings | 77,089 → 77,435 | +0.45% | 216,316 → 216,662 | +0.16% |
| Equal-tag collision fallback | 311,155 → 312,957 | +0.58% | 453,070 → 454,872 | +0.41% |
| Camera control | 1,242 → 1,242 | 0% | 187,106 → 187,106 | 0% |

The collision median improved 74.27% and 51.93%, exceeding the predeclared 5%
target. Every control remained below the 1% p95 regression limit. Dynamic-string
ROMs grew 288 bytes and IWRAM 104 bytes; static-string ROMs grew 556 bytes and
IWRAM 88 bytes. Numeric ROMs grew 348 bytes with unchanged IWRAM; camera binaries'
sizes were unchanged. EWRAM and heap were identical in every pair.

## Workload calibration before candidate timing

All three string fixtures use 12 stored and four absent 64-byte keys with a long
common prefix, final capacity 16, and separately allocated equal-content query
strings. `sub(text,1,#text)` forces heap strings during initialization; measurement
does not allocate strings. The collision set has one bucket and distinct tags
(including zero); the distributed set covers every bucket; the fallback set has
one bucket and the same tag. [Frozen keys and source hashes](workloads.json)
fully specify the final workload.

The original full sweep performed 12 hits, 12 updates and four misses per update.
Its baseline alone took 1,001,563 cycles and missed every update. No candidate
timing was collected for that overloaded workload. The approved baseline-only
calibration reduced work to four interleaved hits/updates and one rotating miss,
preserving keys, capacity and a full sweep every three updates. All twelve
baseline-only cases then passed before the final source hashes were frozen and
the candidate ran. The [original baseline](overloaded-baseline.json),
[original fixture](overloaded-fixture.lua.in),
[calibration summary](baseline-validation-summary.json) and logs preserve this
measurement-validity correction. An earlier unsupported conditional `cls` shape
was also corrected before successful baseline measurement; no renderer code changed.

## Construction cost and present-day relevance

Five alternating host sample pairs measured each fresh 12-key table with 16 hash
slots, normal allocation and GC included. Reusable keys and image preparation
were outside timing. Each sample ran at least 300 ms, with one Go benchmark CPU.
These are host timings, not GBA cycle estimates. [Raw samples](cold-samples.json)
and [medians](cold-summary.json) retain all measurements.

| Operation | Baseline ns/table | Candidate ns/table | Change |
| --- | ---: | ---: | ---: |
| Insert colliding dynamic keys | 6,860 | 5,141 | −25.06% |
| Insert distributed dynamic keys | 1,467 | 2,105 | +43.49% |
| Copy dynamic-key image | 23.24 | 658.90 | +2,735.20% |
| Copy static-key image | 22.12 | 33.87 | +53.12% |

Ordinary insertion costs an extra 638 ns per table; dynamic image normalization
adds 635.66 ns; static images add 11.75 ns. Go allocation counts remained zero
because these objects live in mTile's own heap. The very large dynamic-image
percentage is a comparison against a cheap copy, not a similarly large frame-rate
claim. Compiler-generated images currently require static string keys
(`internal/transpiler/table_literals.go:169`), so dynamic images are primarily a
runtime correctness case. Static images are used by real generated code and still
pay the new per-slot scan.

A read-only search of example Lua found concatenated table keys only in three
host parity-test sites, with no obvious dynamic-string-key construction in shipped
game sources. This is source inspection, not a complete dynamic-type analysis.
The current Celeste generated source contains four image constructor call sites,
in hair creation, player reset, death particles and effect initialization; their
[recorded call sites](celeste-image-call-sites.json) show existing code paying the
scan without using the new filter. This asymmetry, the cold costs and added IWRAM
make the optimization unjustified as a default today. A future demonstrated game
using long, colliding dynamic keys could justify revisiting the preserved patch.

## Celeste limit, restoration and reproduction

Optional real-game validation was attempted only after all early gates passed.
The unchanged baseline rejected current `celeste-mode0` with its native-audio,
four-BG, per-frame palette-update Mode 0 eligibility guard. The attempted Mode 4
fallback then failed baseline linking: `.data` exceeded IWRAM by 880 bytes and
`.bss` by 1,252 bytes. No Celeste candidate was built or timed. These are baseline
limitations, not fingerprint regressions; the two preflight logs are retained.

Both production files were restored byte-for-byte from the initial frozen dirty
runtime, and the candidate-only padding test was removed from ordinary test
discovery. [Restoration hashes](restoration.json) prove preservation. Rejected
implementation and tests remain only as the patch and `.go.in` archive. The
opt-in native driver and Lua fixtures remain for reproduction.
After restoration, focused transpiler table-literal/renderer-contract tests,
the full build and CLI packages, and full runtime suites in both render modes
passed (`restored-*.log`). The final broad transpiler run belongs to the parent
experiment loop; it was not repeated here. The optional Celeste diagnostic now
requires its own `MTILE_HASH_CELESTE=1` switch in addition to runtime paths, so
setting the early-gate environment does not automatically include it.

To reproduce, make independent copies of the current baseline runtime, apply
`candidate.patch` to the candidate copy from its parent directory, and copy
`hash_fingerprint_test.go.in` to the candidate as `hash_fingerprint_test.go`.
Use `scripts/go-weekly/hash-fingerprint/cold_bench.py BASELINE CANDIDATE OUTPUT`
for the alternating cold benchmark. Set absolute `MTILE_HASH_BASELINE`,
`MTILE_HASH_CANDIDATE`, and `MTILE_HASH_OUT`, then run:

```sh
go test ./cmd/renderbench -tags=runtimeperfdiagnostic -run '^TestHashFingerprintDiagnostic$' -count=1 -v
```

The frozen runtime copies, linked ROMs, ELF files and maps are retained under
`/tmp/mtile-go-weekly-loop/hash-fingerprint/`; large binaries are not checked in.
In particular, `native/collision-mode0/{baseline,candidate}/game.{elf,map}` is
the real code-change pair reserved for the subsequent ELF attribution experiment.

The camera timings and frame checksums describe the original test fixture. Its screenshots are omitted from the public record. The current test fixture uses original geometric tiles.
