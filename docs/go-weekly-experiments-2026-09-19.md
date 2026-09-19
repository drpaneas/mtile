# Go Weekly engineering experiments — 2026-09-19

This loop evaluates the eight primary recommendations from the Go Weekly title
survey. Each candidate is implemented against the existing working tree, tested,
measured, and accepted or removed before the next candidate changes product code.
Existing uncommitted work is the baseline and must be preserved.

## Decision rules

Correctness gates take precedence over microbenchmarks. Changes to public Lua
semantics require the complete renderer contract, documentation and selection
tests. None of these experiments intends to change the public Lua API.

Cartridge experiments use fixed-input native mGBA runs with exact pixel/state
comparisons, timer cycles, ROM/IWRAM/EWRAM use, heap traces and missed frames.
These are emulator measurements, not physical GBA measurements. Host wall-clock
measurements use repeated fixed workloads; timing noise is reported, not called
deterministic. A retained speed optimization needs a repeatable useful gain and
no material control regression. A testing/debugging feature needs demonstrable
new capability and bounded overhead; it need not improve game FPS.

Candidates that fail a required capability or correctness gate are rejected
without spending time on broader performance tests that cannot reverse that
decision. Rejected source is retained only as an explicitly labeled experiment
patch or fixture, not shipped in ordinary builds.

For experiment 7, before implementation or timing, the optional recorder's host
overhead gate is a median paired increase of at most 5% in each of Sprite, Map
and Code Update/Draw/readback workloads across at least six alternating pairs.
Added steady post-GC live heap must stay within 32 MiB on those fixtures; RSS and
sample spread are also reported. Samples that trigger and stop recording are
invalid for enabled-overhead measurement. The intended five-second/eight-MiB
recorder settings are retention targets, not a universal memory bound. Correct
pre-stall event ordering, lifecycle/race tests and an unchanged disabled dispatch
path are required independently of those timing results.

## Experiment sequence

| # | Candidate | Acceptance question | Status |
| --- | --- | --- | --- |
| 1 | Ebitengine 2.10.2 / Oto 3.5.0 and VM editor testing | Does real editor input/render testing work, with passing editor/audio tests and no material repeated-workload regression? | **Kept** |
| 2 | Lua differential fuzzing | Does an independent oracle catch seeded semantic faults, cover real generated programs, and fit a bounded test/fuzz budget without production overhead? | **Kept, with bug fix** |
| 3 | TinyGo mGBA panic logging | Can the missing-symbol problem be fixed and arbitrary Go panics captured without a production ROM cost or an unjustified toolchain fork? | **Kept as isolated diagnostics; no default integration** |
| 4 | Palette dithering | Does a bounded import prototype improve low-frequency color fidelity while preserving exact palette colors/transparency at acceptable import and tile-reuse cost? | **Kept, optional and off by default** |
| 5 | Lua-table hash fingerprints | Can collision-heavy lookups improve at least 5% in target frames with exact semantics, no greater than 1% p95 regression in controls, and justified code/heap cost? | **Dropped; patch and evidence retained** |
| 6 | ELF size attribution | Can reproducible per-symbol/section comparisons explain existing ROM/IWRAM totals without new runtime cost or excessive tooling dependencies? | **Kept** |
| 7 | Host flight recording | Does opt-in recording produce a useful pre-stall trace with bounded CPU/memory overhead and zero recording cost when disabled? | **Dropped; prototype and trace evidence retained** |
| 8 | Host PGO | Does representative profiling yield a repeatable at least 3% end-to-end Go-workload gain on held-out inputs, without greater than 2% control regression or unjustified profile maintenance? | **Kept for the CLI only** |

## Baseline

Toolchain: Go 1.27.1, TinyGo 0.42.0, LLVM 22.1.4, macOS arm64.
The initial source manifest and binary Git diff were captured before edits.
The first full root suite passed every real test package, but failed discovery
because archived Go snippets beside standalone C programs looked like a package.
The snippets now use `.go.in` filenames; their runner still copies identical Go
contents to the same experiment destinations. The C programs are unchanged.
After that repair, `go test ./...` passed. The separate runtime suite also passed.
[Baseline metadata, source fingerprints and logs](benchmarks/go-weekly/baseline/)
record the existing dirty working tree used for all comparisons.

## 1. Ebitengine / Oto — kept

The tested dependency upgrade, opt-in render benchmark and real-editor VM smoke
are retained. The VM navigated, edited, saved, undid and restored exact source
bytes; all three workspaces had exact RGBA parity. Required repository gates and
editor/emulator/audio/MIDI race tests passed. Linux/amd64 editor compilation with
cgo disabled now succeeds (the former dependencies fail that build).

Eight alternating Sprite benchmark pairs measured 6.875 → 6.442 ms median
(−6.29%, p=.010), resolving an initial non-interleaved slowdown. Map and Code
also improved in their repeated runs. Code's allocation count rose about
711/frame (+58%), though bytes rose 1.5% and rendering improved. Audio synthesis
controls were statistically unchanged with zero allocations. These host timings
are noisy; physical audio/MIDI latency and Linux runtime behavior were not tested.
The useful VM capability and passing performance/correctness gates justify the
upgrade despite the recorded Code allocation tradeoff.

[Full evidence and reproduction](benchmarks/go-weekly/ebitengine/README.md).

## 2. Lua differential testing — kept, with a compiler fix

An independent, bounded nil/bool/16:16 model checks parser output and a fixed
corpus of compiled programs, including exact side-effect order. It caught a real
optimized-code bug: nested/parenthesized negation and inlined globals at the
signed minimum could generate a Go constant overflow. Materializing the operand
as a numeric temporary restores the existing wrapping semantics. No Lua API or
renderer eligibility was changed.

All three compiling semantic mutants were detected (precedence, eager
short-circuit RHS, reversed operand order). Parser fuzzing passed 436,272 cases
in 30 seconds; generated execution passed 578 fixed-corpus checks across two
configurations. These are distinct coverage sets: fuzz mutations are not each
compiled and executed. The full transpiler suite, final added witnesses, other
root packages and both runtime modes passed; sandbox-only cache/CoreMIDI
failures were rerun with access.

Six alternating host-transpilation pairs measured 5.374 → 5.481 ms median
(+1.98%), +0.94% bytes and +3.48% allocations on this deliberately expression-heavy
corpus. That modest compiler cost is justified by corrected semantics and useful
regression coverage. The tests add no production runtime work. Rebuilt ordinary
`testio` Mode 0 and Mode 4 ROMs are byte-identical to the frozen baseline.

[Full evidence and reproduction](benchmarks/go-weekly/differential/README.md).

## 3. TinyGo mGBA panic logging — isolated diagnostic recipe kept

The unmodified 0.42.0 debug-tag build fails with the missing `zeroSizeAllocPtr`.
Adding the same constant used by the ordinary ARM7 runtime to a temporary
toolchain copy fixes linking. Both an explicit Go panic and a volatile-index
bounds panic reached the native player's log without calling mTile's logger.
The installed toolchain is unchanged; the runner verifies its selected resource
root and retains a minimal upstream patch. No upstream submission was made.

Healthy release Mode 0/4 builds are byte-identical, including memory totals.
With diagnostic logging enabled, 300 logical frames per renderer have identical
pixels, watched state, heap/GC and missed-frame counts. Mode 0 adds 1,580 ROM
bytes and 20 IWRAM bytes; Mode 4 adds 1,252 and 72 respectively. EWRAM is unchanged.
Median/p95/max cycles increase by eight in each mode (1,567 → 1,575;
229,270 → 229,278). These are fixed-input emulator measurements.

Target-generation, build, emulator, live-logger and runtime tests passed.
The reproducible diagnostic repair is useful, but maintaining a private default
toolchain fork is not justified. Default builds and the existing preview logger
are therefore unchanged.

[Full evidence and reproduction](benchmarks/go-weekly/tinygo-debug/README.md).

## 4. Ordered palette dithering — kept as an optional preview

Image paste still starts with the unchanged nearest-color quantizer. Pressing
**D** during placement previews a cached 4×4 ordered mixture of existing palette
colors; pressing D again restores nearest output exactly. The pattern aligns
with 8×8 tiles. Exact palette artwork, transparent masks and repeated source-tile
identity are preserved. Undo/redo now cancels pending paste placement, fixing a
stale-palette preview found during review. Preview/cancel does not mutate assets;
commit remains one undo step.

Across twelve predefined smooth fixtures, median 4×4 RGB15 mean-error reduction
was 75.98% (each improved 38.4–93.6%). Full first-toggle p95 at 128×128 was 1.649 ms
for PICO-8 and 15.330 ms for Aurora. A synthetic 256-color high-variation case
took 28.119 ms p95. Cached full toggles allocate nothing. Nearest-color benchmark
medians remained effectively unchanged (448.9 → 447.2 µs and 3.990 → 3.986 ms).
Editor, build, CLI, runtime and focused race checks passed; real-editor captures
were visually inspected.

This improves averaged color fidelity, not every pixel or every artistic choice.
Zoomed output has pronounced speckling; raw per-pixel error increases. The
PICO-8 sphere used 96 → 160 distinct tiles, and an Aurora diagonal used 20 → 31.
These counts measure possible reuse, not current ROM size: sheets use fixed
slots. The useful preview, bounded cost and unchanged default justify keeping
the option with these tradeoffs explicit.

[Full evidence and reproduction](benchmarks/go-weekly/ordered-dither/README.md).

## 5. Lua-table hash fingerprints — dropped from the runtime

The prototype stored an eight-bit string-hash fingerprint in existing key
padding, avoiding full comparisons for unequal fingerprints. All twelve native
workload/renderer gates passed: collision-frame medians improved 74.27% in Mode 0
and 51.93% in Mode 4; the largest control p95 regression was 0.583%. Generated
source, pixels, independently checked state, heap and EWRAM matched, with no
warmup or steady missed frames. Baseline-only workload calibration and its
rejected, overloaded inputs are preserved in the evidence.

Passing those synthetic gates does not justify this as a default optimization.
Fresh ordinary twelve-key insertion measured 1.467 → 2.105 µs (+43.49%); dynamic
image construction measured 23.24 → 658.9 ns and static images 22.12 → 33.87 ns.
These are host measurements, not cartridge-cycle estimates. Dynamic workloads
also gained 288 ROM bytes and 104 IWRAM bytes. Existing example code showed no
clear dynamically constructed-key workload that would benefit, while generated
static image constructors would pay the new scan.

Celeste could not supply an additional comparison: its unchanged baseline fails
Mode 0 eligibility and overflows IWRAM in the attempted Mode 4 build. These are
baseline limitations, not candidate regressions. With demonstrated construction
costs and no demonstrated current-game benefit, the default runtime change is
rejected. The exact original runtime files are restored; the prototype remains
an explicitly labeled patch with fixtures and reproducible measurements.
Candidate runtime, Mode 0 and focused race tests passed. After restoration,
focused transpiler table/renderer checks, build/CLI and both runtime suites also
passed. A source-manifest audit confirms the rollback preserved existing work.

[Full evidence and reproduction](benchmarks/go-weekly/hash-fingerprint/README.md).

## 6. ELF size attribution — kept

The standalone `cmd/elfsize` command uses Go's standard `debug/elf` package to
report sections and symbol deltas in text or JSON. ROM load bytes, ROM address
span, IWRAM, application EWRAM and the separate debug mailbox have distinct
totals. Interval unions prevent overlapping ranges from inflating totals;
identical symbol aliases are grouped. Initialized IWRAM code correctly costs
both ROM and RAM, while NOBITS stack/BSS costs only RAM. Unsized symbols and
padding remain explicitly unattributed rather than being assigned invented costs.

Independent LLVM/linker-map/cartridge checks and four saved ELF fixtures agree.
The rejected fingerprint pair's complete 288-byte ROM increase is explained by
`TableSet` (+104), table growth (+80), and `hashFind` (+104). The last function
also explains all 104 added IWRAM bytes. Synthetic checks cover Thumb addresses,
aliases, overlaps, stripped files, malformed inputs and truncated file backing.
The scoped unit/reference suite and race run both passed.

Five parse samples per ELF measured about 51 µs per file, approximately 103 KiB
and 999 host allocations. Twenty compiled CLI comparisons, including startup,
file I/O and JSON output, measured 10.279 ms median and 12.065 ms p95. That modest
diagnostic cost and exact attribution justify retaining the command. It adds no
dependency and is not linked into the editor or cartridge builds.

[Full evidence and reproduction](benchmarks/go-weekly/elf-size/README.md).

## 7. Editor flight recording — dropped from the product

The opt-in prototype captured one trace after a completed editor Update took at
least 100 ms. With recording disabled it passed the exact original game to
Ebitengine, adding no per-frame checks. The real trace decoded completely with
Go's trace reader: a pre-stall sentinel preceded a 122.023 ms blocked region,
which preceded the capture trigger. Threshold/error propagation, exclusive file
creation, partial-file cleanup, replacement preservation and shutdown races
passed. The shutdown-waits-for-writer check uses deterministic `synctest`.

The frozen six-pair Update/Draw/readback comparison failed its performance gate.
Median paired changes were −3.496% for Sprites, +1.899% for Map, and **+9.059% for
Code**, exceeding the predeclared 5% limit. Code's six changes were +25.974%,
+5.796%, +4.672%, +20.127%, +12.321% and −3.416%. All measured frames had exact
RGBA equality, and no sample stopped recording prematurely. These noisy host
measurements establish rejection under this experiment's rule, not a universal
slowdown percentage or a Sprite speedup.

Separate active-memory runs added 4,106,200 live heap bytes in Sprites and
3,008,128 in Map, below the 32 MiB fixture limit. RSS is recorded separately and
is not a hard memory bound. Code's memory phase was deliberately not run after
the timing failure. The five-second/eight-MiB recorder settings are targets;
asynchronous capture can still cause global trace flushing. Permanent hangs and
Draw-only stalls were outside this prototype's trigger scope.

The useful trace capability did not meet the agreed host overhead budget. No
retuning or replacement measurement followed the failure. The editor/CLI
integration is restored to its pre-experiment state; the candidate survives only
as an explicit patch and archived fixtures with the complete measurements.
Both existing entry-point files match their pre-experiment hashes, the four new
Go files are absent from ordinary builds, and restored editor/CLI tests passed.

[Full evidence and reproduction](benchmarks/go-weekly/flight-recorder/README.md).

## 8. Host PGO — kept for `cmd/mtile` only

One 30.14-second profile covered 1,509 equal training rounds over Celeste AUTO,
multi-file and audio-gb-song. Held-out inputs were platformer, Pong and mapdemo;
controls were hello and noalloc-call-stress. These sets were fixed before
profiling. Existing baseline build guards excluded the initially proposed
celeste-mode0 and loop configurations. Platformer is a small starter, so Pong
and mapdemo dominate the held-out workload. The ignored local Celeste source is
snapshotted outside the repository, with input hashes retained for reproduction.

Ten alternating comparisons of the Go preparation adapter improved the held-out
corpus from 15.309 to 14.617 ms (4.52%); every pair improved. Both binaries matched
the actual build pipeline and produced byte-identical source, asset, global and
renderer manifests across all eight projects. To check that PGO had not merely
favored the test adapter's call sites, a separate validation timed the actual
`Service.Build` method up to its pre-workspace boundary. It reused the same
profile and inputs, including identical callback/recovery overhead in both
binaries. No second profile or tuning was performed.

All six actual-build pairs improved: 14.872 → 13.972 ms per held-out corpus,
**6.05% (0.900 ms)**. Hello improved 4.38%; noalloc regressed 0.079%, within the
2% control limit. These measurements cover the Go preparation prefix, excluding
workspace writing, external TinyGo, process startup and cartridge execution.
They are not a claim about complete ROM build latency or game FPS.

Actual CLI builds for hello, Pong and mapdemo produced identical generated Go
and ROM hashes with PGO enabled and disabled. The CLI grew 231,408 bytes (3.14%),
from 7,374,434 to 7,605,842; the profile is 48,300 bytes. The repeated useful
prefix gain, passing controls and small isolated artifact justify retaining
`cmd/mtile/default.pgo`. Editor builds receive no default profile. Profile
refreshing and the mirrored training adapter require maintenance; the recipe
records inputs and validates the actual build path. `go build -pgo=off
./cmd/mtile` remains available for comparisons or disabling the profile.

Scoped build/CLI and both runtime suites passed. Existing production source
files are unchanged by this experiment.

[Full evidence and reproduction](benchmarks/go-weekly/pgo/README.md).

## Final verification

All eight experiments are complete. On the final source tree, `go test ./...`
passed in the root module (233.3 seconds), and both `go test ./...` and
`go test -tags=mtile_mode0 ./...` passed in the runtime module. These final checks
complement the per-experiment benchmarks, parity checks and focused race tests
linked above. [Commands, exit codes and timings](benchmarks/go-weekly/final/tests.json)
and [full root log](benchmarks/go-weekly/final/root.log) are retained.

The final preservation audit compared all 1,134 original files: 1,125 remain
byte-identical, seven have the intended changes described above, and two archived
Go snippets were renamed with identical contents. There were no unexpected
changes. All 19 checked Go files were formatted, and `git diff --check` passed.
Rejected runtime and recorder implementations are absent from ordinary builds.
[Source hashes, expected changes and formatting results](benchmarks/go-weekly/final/source-preservation.json)
document preservation of the pre-existing work. No commits were created.
