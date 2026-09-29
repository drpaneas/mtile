# OBJ pose and animation optimization — 2026-09-24

The generated GBA code had avoidable RAM and CPU costs. This pass removes them without changing the Lua API, pose pixels, clip timing, or the hardware renderer's admission limits. For the current Nerdo cart, linked EWRAM falls from **155,316 to 12,236 bytes**, and the heaviest measured Mode 0 frame falls from **78,929 to 48,144 cycles**. These are measured improvements, not a claim that the renderer is globally optimal.

## Changes and evidence

1. **Remove unused Lua heap machinery.** `pspr` and `anim` were missing from the compiler's nonallocating-call list. Even allocation-free carts retained two 57,344-byte heap semispaces and GC bookkeeping. Both calls now participate in the existing whole-program allocation proof. Allocating arguments and other allocating code still retain the heap.
2. **Resolve literal clip names during compilation.** `anim("idle", x, y)` now calls the runtime with the clip's numeric ID. This removes the name's initialization-time interning/state and its per-draw decoding and clip-name search. Dynamic names still work; proven heapless programs decode static names without bringing in the general heap-backed string path.
3. **Allow static names in global variables without forcing a heap.** Boxed globals holding static strings, booleans, or nil previously overrode the allocation proof. They no longer do. Programs that can allocate still register their global GC roots. Native link tests cover both cases.
4. **Align OBJ pixels to four bytes in ROM.** Nerdo's byte bank started at an address equal to `2 mod 4`, sending tile copies through ARM's byte-copy loop. A shared declaration generator now guarantees word alignment, with unchanged pixel bytes and no RAM copy of the library. An isolated experiment reduced Nerdo's p95 from 78,550 to 48,102 cycles without increasing its ROM or RAM. Actual TinyGo ELF tests verify alignment, ROM residency, and payload bytes; generated-asset tests also cover empty banks.
5. **Check reserved VRAM ranges in constant time.** Pose-window validation now uses range overlap checks instead of checking every tile against the text and packed-sprite reservations. Exhaustive tests cover every valid window in the 512-tile space, invalid bounds, and the relevant runtime feature combinations.

The existing good behavior remains: identical source tiles share ROM pixels; pose recipes and clip sequences live in ROM; only the active pose windows need staging RAM; unchanged poses do not upload tiles again. An animation clip stores pose references and timing, not another copy of the artwork.

## Measurement method

The benchmark builds actual GBA ROMs with TinyGo 0.42.0 / LLVM 22.1.4 using Go 1.27.1, then runs their ARM code and graphics hardware emulation in the headless mGBA player. This is emulated GBA evidence, not desktop Go timing or a physical-handheld measurement.

- Frozen baseline and final source copies use identical project assets, player binary, toolchain, and profiling settings. The baseline includes the existing uncommitted editor/animation work; comparing against the Git commit alone would be the wrong baseline.
- Each of six scenes runs both Mode 0 and Mode 4: two warmup game frames, followed by 120 sampled game frames per renderer.
- The current runtime runs at 30 game frames per second, with a budget of **561,792 cycles per game frame**. Its CPU measurement includes frame work and display commit, excluding deliberate pacing waits. Boot and cold initialization are outside the sample.
- The GBA LCD refresh takes 280,896 cycles at approximately 59.73 Hz. Its physical VBlank is 83,776 cycles; the runtime conservatively budgets 67 scanlines, or **82,544 cycles**, for its commit. See [Tonc's video timing reference](https://gbadev.net/tonc/video.html).
- RAM figures are **linked RAM reservations**, including runtime state, not just art buffers or observed allocations. IWRAM also includes 9,216 bytes of reserved stacks; stack high-water usage was not measured. Free-space counts exclude the reserved EWRAM debug mailbox and IWRAM tail.
- CPU runs enable identical debug telemetry before and after. ROM sizes in the scene tables therefore include telemetry. A separate release build below measures Nerdo without it.

All **720 game-frame pairs** match between renderers and before/after. All **1,440 Mode 0 LCD scans per phase** also match, including the first scan of each game frame. No Mode 0 scene misses a frame. Every measured Mode 0 commit fits the conservative VBlank budget. The JSON evidence includes individual cycle samples, frame hashes, ROM hashes, and source-project hashes.

## Mode 0 CPU results

Cycles per game frame; each cell is baseline → final. P95 is the 95th percentile. Maximum means the largest sample, not a proof of worst-case execution time.

| Scene | Median cycles | P95 cycles | Maximum cycles | Final p95 / game-frame budget |
| --- | ---: | ---: | ---: | ---: |
| Clip clock | 13,848 → 10,313 | 14,951 → 11,416 | 14,952 → 11,417 | 2.03% |
| Clip switching / helpers | 55,690 → 41,031 | 69,954 → 49,827 | 70,162 → 50,035 | 8.87% |
| Fighters | 29,093 → 24,274 | 219,546 → 123,033 | 219,546 → 123,033 | 21.90% |
| Full OBJ window changed every frame | 430,041 → 427,534 | 430,041 → 427,534 | 430,041 → 427,534 | 76.10% |
| Six heroes / 180 poses | 19,120 → 15,639 | 97,897 → 93,988 | 97,920 → 94,011 | 16.73% |
| Nerdo idle clip | 14,220 → 13,815 | 78,887 → 48,102 | 78,929 → 48,144 | 8.56% |

The largest final Mode 0 commit is **68,629 cycles**, in the full-window stress test. That leaves 13,915 cycles of the conservative VBlank budget. Nerdo's maximum commit is 8,887 cycles. Large gains on pose-changing frames come from preparing the next pose faster; unchanged-pose frames already avoided the upload.

## Mode 0 memory and ROM

All sizes are bytes. EWRAM and ROM columns show baseline → final; IWRAM is the final reservation including stacks.

| Scene | EWRAM | IWRAM | Debug ROM |
| --- | ---: | ---: | ---: |
| Clip clock | 151,492 → 8,412 | 10,416 | 20,524 → 17,664 |
| Clip switching / helpers | 153,492 → 10,412 | 10,416 | 27,324 → 24,884 |
| Fighters | 170,888 → 27,820 | 10,536 | 44,388 → 42,548 |
| Full OBJ window | 183,992 → 40,924 | 9,980 | 28,468 → 26,580 |
| Six heroes / 180 poses | 157,276 → 14,044 | 10,416 | 735,180 → 732,328 |
| Nerdo | 155,316 → 12,236 | 10,416 | 36,908 → 34,320 |

All six final fixtures omit the Lua heap. The baseline reserved it despite allocating zero Lua heap bytes during measurement. This distinction matters: zero allocations did not previously mean zero heap RAM. Games that create tables or strings can still need that heap.

### Nerdo release build

The actual cart containing `anim("idle", 100, 60)`, built without debug telemetry, has these linked sizes:

| Resource | Baseline | Final |
| --- | ---: | ---: |
| ROM | 35,408 | 32,848 |
| EWRAM used | 155,316 | 12,236 |
| EWRAM available | 106,316 | 249,396 |
| IWRAM used, including reserved stacks | 10,416 | 10,416 |

The **143,080-byte EWRAM saving (92.12%)** consists of 114,688 bytes of unused heap semispaces, 28,380 bytes of unnecessary context/GC bookkeeping, and 12 bytes of static-name state. Pose staging remains 2,816 bytes. The final context is 7,272 bytes; shared-sheet indices and flags use 1,800 bytes; cart metadata uses 348 bytes.

Nerdo still has exactly 10,752 bytes of OBJ pixels in ROM: 168 unique 8×8, 8-bit tiles. Its clip adds a 20-byte record and 8 bytes of pose IDs, plus name bytes; it does not duplicate the pixels. Exact linker symbols and ROM hashes are recorded in [the release memory evidence](obj-optimization-v1/nerdo-release-memory.json).

## Six heroes with 30 poses each

This fixture contains **180 poses**, each 40×80 pixels, and six named 8 FPS clips. All **9,000 nonempty source tiles are unique**, independently counted, so the capacity result does not depend on repetitive test art deduplicating away. Pixels alone occupy **576,000 bytes of ROM**.

With two heroes visible simultaneously, the complete instrumented cart occupies **732,328 bytes of ROM (about 715 KiB)**, **14,044 bytes of EWRAM**, and **10,416 bytes of IWRAM including stacks**. It uses ten hardware OBJ entries and 80 OBJ cycles on the busiest scanline. Its heaviest sampled frame costs 94,011 cycles, within the current game-frame budget.

That is how the library fits: all poses stay in cartridge ROM, while the renderer stages and uploads the poses currently being displayed. This test is not drawing all 180 poses at once. Its synthetic 480×1200 source layout is a compiler storage fixture, independent of the editor's page dimensions. Other pose sizes, more simultaneous actors, game logic, and audio will change the budget.

## Remaining costs and limits

Mode 0 is the efficient hardware-sprite path for these admitted scenes. Mode 4 pays for software drawing plus a framebuffer copy/commit costing roughly 77,000 cycles per frame. Software drawing dominates the heavier scenes, reaching a median 1,080,772 cycles in the full-window test. Its p95 results are:

| Scene | Mode 4 p95 cycles, baseline → final | Final missed game frames / 120 |
| --- | ---: | ---: |
| Clip clock | 151,768 → 151,029 | 0 |
| Clip switching / helpers | 229,120 → 223,587 | 0 |
| Fighters | 731,719 → 731,665 | 120 |
| Full OBJ window | 1,158,587 → 1,158,553 | 120 |
| Six heroes / 180 poses | 416,663 → 408,159 | 0 |
| Nerdo | 224,404 → 224,615 | 0 |

Nerdo's Mode 4 median regressed slightly, from 218,329 to 218,480 cycles (+0.07%); this pass does not improve every renderer metric. The two heavy Mode 4 scenes exceed the current frame budget both before and after. The full-window Mode 0 stress case also exceeds a single 60 Hz refresh budget, although it fits the current 30 Hz loop. These results are not a blanket 60 FPS guarantee.

The remaining fixed pose-site bookkeeping is about 4,368 bytes for 128 sites. Sizing it to the compiled call sites could save more RAM. Dynamic clip-name lookup remains linear, while literal names bypass it. Exact animation time calculations retain the runtime's wide-integer fallback; the ARM helper already has a narrow-operand fast path. Changing timing math without preserving overflow and playback behavior would be premature.

Sharing already resident tiles across different active actors could reduce VRAM or transfers further, but requires a different allocation/cache strategy. That is separate from the existing ROM deduplication. The compiler's OAM, scanline, VRAM, and upload-budget checks remain necessary: editor library capacity does not remove physical GBA limits.

## Verification and reproduction

Passed after the production changes:

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile ./internal/assetsync ./cmd/renderbench
cd runtime && go test ./...
```

Runtime suites also passed with OBJ streaming, packed OBJ, and text combinations. Regression tests check clip timing/name behavior, global static-name choices, allocating arguments and GC roots, real TinyGo heap inclusion/exclusion, actual ROM alignment, unchanged OBJ bytes, empty banks, large-bank compaction, and VRAM reservation equivalence.

The opt-in native harness is `cmd/renderbench/obj_optimization_diagnostic_test.go`. From the desired source checkout, with the native player and TinyGo installed, use a fresh output directory:

```sh
MTILE_OBJ_OPT_OUT=/tmp/mtile-obj-baseline \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestOBJOptimizationTargetDiagnostic$' -count=1 -v

MTILE_OBJ_OPT_OUT=/tmp/mtile-obj-final \
MTILE_OBJ_OPT_COMPARE=/tmp/mtile-obj-baseline \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestOBJOptimizationTargetDiagnostic$' -count=1 -v
```

Run the first command against the baseline source and the second against the optimized source. The ordinary `cmd/renderbench` test suite also checks the recorded evidence without requiring native tooling.

[Summary measurements](obj-optimization-v1/summary.json), [source/toolchain provenance](obj-optimization-v1/provenance.json), and the per-frame reports under `obj-optimization-v1/{baseline,phase1,phase2}/` preserve the comparison. `phase2` is the final result; `phase1` isolates the earlier compiler/runtime changes before ROM alignment and the boxed-global fix. Generated Go, ROMs, ELFs, linker maps, and final images from this run are additionally preserved locally under `/private/tmp/mtile-obj-optimization-20260924/`.
