# Size-specialized allocation experiment — 2026-09-19

Decision: **drop both allocator changes**. The original runtime has been
restored byte for byte. Retain the diagnostic, candidate patches and measurements
so the experiment can be reproduced without putting either candidate in games.

[Go's size-specialized allocation article](https://go.dev/blog/size-specialized-allocations)
describes a Go 1.27 compiler/runtime optimization that is enabled automatically.
mTile already requires and uses Go 1.27.1, with no disabling `GOEXPERIMENT`
override in this run. Its desktop tools already receive that optimization.
Cartridges are compiled by TinyGo with `-gc=none`; Lua objects use mTile's own
moving heap. They do not use Go's specialized malloc functions.

The transferable idea is to replace small, dynamically sized clearing operations
with constant-offset stores. `Heap.allocObject` already has shared validation,
an IWRAM common path, and a ROM collection fallback. Many table literals use
`allocObjectFilled`, which copies a complete image without first zeroing it.
Duplicating Go's size classes or adding another general allocator would not fit
these existing paths.

Two narrow candidates specialized aligned 4-, 8-, 12- and 16-byte clears while
preserving allocation sizes, zeroed padding, handles, collection and error
semantics. This covers strings of up to 14 characters, including their two-byte
header. Larger and unaligned requests kept the generic clearing path.

- **Switch:** fixed stores with fallthrough, followed by the existing loops.
- **Conditional:** fixed stores selected by comparisons, with generic loops in
  an `else` branch.

The linked ARM disassembly confirms the switch candidate emitted fixed stores
and a jump table. It still carries dispatch and generic-path setup costs.

Measured with TinyGo 0.42.0 / LLVM 22.1.4 and the repository's headless mGBA
player: 20 warmup updates, then 300 synchronized updates per comparison. These
are native ROM emulator measurements, not physical GBA measurements.

| Candidate / workload | Renderer | Baseline median cycles | Candidate median cycles | Saving | Additional IWRAM bytes |
| --- | --- | ---: | ---: | ---: | ---: |
| Switch / mixed small allocations | Mode 0 | 143,555 | 143,146 | 0.285% | 48 |
| Switch / mixed small allocations | Mode 4 | 283,526 | 283,024 | 0.177% | 32 |
| Switch / large strings | Mode 0 | 158,414 | 158,388 | 0.016% | 48 |
| Switch / large strings | Mode 4 | 299,326 | 299,254 | 0.024% | 32 |
| Switch / numeric hash control | Mode 0 | 174,031 | 174,031 | 0% | 116 |
| Switch / numeric hash control | Mode 4 | 315,861 | 315,861 | 0% | 116 |
| Switch / camera control | Mode 0 | 1,242 | 1,242 | 0% | 0 |
| Switch / camera control | Mode 4 | 187,106 | 187,106 | 0% | 0 |
| Conditional / mixed small allocations | Mode 0 | 143,555 | 143,329 | 0.157% | 64 |

ROM growth equals IWRAM growth in these comparisons; EWRAM use is unchanged.
The conditional candidate also worsened p95 from 149,437 to 149,731 cycles and
maximum frame time from 312,684 to 312,972 cycles, so it was not expanded to the
remaining cases. The first switch/small/Mode 0 trial and its later suite rerun
produced identical timing results.

The small workload allocates twelve variable substrings of 1–30 characters and
twelve dynamic-key 44-byte tables per update. Twelve retained nested roots
survive repeated collections. It exercises both specialized and generic paths;
the measured gain is not an upper bound for a short-string-only workload. The
large-string control uses 65–128 characters. Numeric-hash initialization is
outside the measured steady frame loop; its unchanged timing does not establish
that initialization is unchanged. Camera is a heapless control.

All nine comparisons had identical generated Go, pixels, watched values and
per-update heap traces, including allocation/live/peak bytes, handle counts and
collection counts. All had zero missed frames and no reported heap errors.
The mixed and large workloads ran through five and nine collections respectively.
The switch prototype also passed runtime tests in both renderer configurations,
including expanded size-boundary, unaligned, poisoned-neighbor and exact-end-of-
heap checks. Those temporary test additions are archived as a patch.

The pros are a small, repeatable speedup and preserved behavior with no extra
data memory. The cons are additional hot code in the GBA's limited internal RAM,
code growth even for games with no steady allocation benefit, and another clear
path to maintain. The measured frame savings do not justify those costs here.
This rejects these two implementations on the tested workloads; it does not
rule out a future specialization supported by stronger workload evidence.

Evidence is in [benchmarks/size-specialized-allocations](benchmarks/size-specialized-allocations/):
[summary](benchmarks/size-specialized-allocations/summary.json),
[toolchain and source fingerprints](benchmarks/size-specialized-allocations/metadata.json),
compressed full per-frame results, logs, ARM disassembly, and both candidate
patches. Source fingerprints record the existing dirty workspace baseline;
unrelated work was preserved. No public Lua API or renderer eligibility changed.

To repeat the switch experiment from the repository root, with the native player
already built:

```sh
trial=$(mktemp -d)
cp -R runtime "$trial/baseline"
cp -R runtime "$trial/candidate"
patch -d "$trial/candidate" -p2 < docs/benchmarks/size-specialized-allocations/switch.patch
MTILE_SIZE_ALLOC_BASELINE_RUNTIME="$trial/baseline" \
MTILE_SIZE_ALLOC_CANDIDATE_RUNTIME="$trial/candidate" \
MTILE_SIZE_ALLOC_OUT="$trial/results" \
go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestSizeSpecializedAllocationDiagnostic$' -count=1 -timeout=10m -v
```

Use `conditional.patch` with a fresh candidate copy and select
`^TestSizeSpecializedAllocationDiagnostic/small-mode0$` to repeat the second
experiment. Apply `boundary-tests.patch` with the same `patch -p2` command to a
candidate copy to run the additional allocator checks. Full raw evidence can be
read with `gzip -dc docs/benchmarks/size-specialized-allocations/raw-results.json.gz`.

Final validation after restoring the original allocator passed in full; the
`final-*-tests.log` files alongside the measurements retain the results:

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
(cd runtime && go test ./...)
(cd runtime && go test -tags=mtile_mode0 ./...)
go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestSizeSpecializedAllocationWorkloads$' -count=1
```
