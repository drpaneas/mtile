# Tile link benchmarks

Records behind the [tile links guide](../../tile-links.md). Measured on
October 25, 2026 with TinyGo 0.42.0 and the pinned headless mGBA helper,
using `cmd/perfsurvey` (3,600 frames per run; the first two seconds are
excluded) and `cmd/renderbench` (20 warm-up frames, 120 paired samples).

`generate_workloads.py` writes each workload twice under `/tmp/tlbench`:
`feature` (a plain `map()` with `assets/tilelinks.json`) and `oracle` (a Lua
scan that clears the marker tiles and draws `anim`/`pspr` in the same order).
A `plain` copy without links measures the map alone.

| Workload | Map | Linked cells | Scroll |
| --- | --- | ---: | --- |
| screen12 | 30x20 | 12 | none |
| dense120 | 30x20 | 120 | none |
| scroll200 | 256x20 | 200 | 2 px per tick |
| stream250 | 1024x20 (streamed) | 250 | 2 px per tick |
| level1500 | 1024x20 (streamed) | 1,500 | 2 px per tick |
| vscroll60 | 30x60 | 90 | 1 px per tick, vertical |

## Lookup strategies (Mode 4)

Each round's raw `perfsurvey` rows are in `round1.txt`..`round3.txt`; columns
are project, renderer, ticks, missed, median, p95, max, budget%, update p95,
draw p95, GC max, heap peak, IWRAM, and ROM bytes.

- Round 1: the three lookups as first written, loops in ROM.
- Round 2: every lookup loop in IWRAM; scan resolves streamed chunks per row.
- Round 3: list and row index also read streamed chunks directly. This round
  decided the implementation; level1500 was added to stress level size.

| Workload | Map alone | Scan | List | Row index |
| --- | ---: | ---: | ---: | ---: |
| screen12 | 44,646 | 117,564 | 103,988 | 105,457 |
| dense120 | 118,828 | 690,728 | 704,044 | 705,597 |
| scroll200 | 152,530 | 271,094 | 266,214 | 263,026 |
| stream250 | 197,224 | 260,355 | 231,894 | 226,249 |
| level1500 | 247,581 | 472,698 | 500,905 | 453,028 |

Scan, list, and row index matched the oracle frame for frame on every
oracle-valid workload. The oracle cannot represent level1500: a Lua table
array holds at most 256 entries, so the hand-written scan silently drops
linked cells beyond that; the three lookups agreed with each other there.

## Mode 4 and Mode 0

Shipped implementation (row index with scan fallback), paired `renderbench`
captures with zero differing pixels in every workload:

| Workload | Mode 4 median / p95 | Mode 0 median / p95 | Mode 0 objects | Missed (4 / 0) |
| --- | ---: | ---: | ---: | ---: |
| screen12 | 103,863 / 104,039 | 32,855 / 45,235 | 12 | 0 / 0 |
| dense120 | 689,205 / 690,965 | 249,261 / 372,305 | 120 | 140 / 28 |
| scroll200 | 221,769 / 254,283 | 58,324 / 201,342 | 32 | 0 / 0 |
| stream250 | 221,412 / 246,294 | 30,832 / 455,471 | 18 | 0 / 0 |
| vscroll60 | 244,303 / 250,192 | 78,710 / 185,625 | 37 | 0 / 0 |

These runs also exposed two existing Mode 0 problems, fixed alongside the
feature and covered by tests:

- A `map()` call without a size counted every cell scrolled off to the left;
  past about 272 pixels into a wide map the count exceeded the 64-cell window
  and Mode 0 stopped drawing the map entirely. The window now starts at the
  first visible cell when needed.
- Streamed maps rebuilt their screen shadows through one full cache lookup
  per cell. With per-row chunk reads, the plain stream250 map went from 192
  missed ticks out of 429 (worst tick 1,250,141 cycles) to none (worst
  451,324).

## Regression call shapes

Measured October 26, 2026 with TinyGo 0.42.0 and the same headless helper.
`TestTileLinkTargetDiagnostic` creates fixtures with linked poses and a clip,
negative part offsets, anchors outside the right and bottom screen edges,
alternating camera offsets, a changing flag mask, and an unconditional
`mset(5,5,0)` that removes a linked marker. One fixture omits map sizes; the
other uses an explicit 30x20 source rectangle. Each run discards 20 frames
and compares 120 synchronized frames, with no missed frames or differing
pixels in either fixture.

| Fixture | Mode 4 median / p95 cycles | Mode 0 median / p95 cycles | Record |
| --- | ---: | ---: | --- |
| Default extent, filtered | 49,113 / 58,508 | 93,024 / 98,627 | [JSON](regression-default-extent.json) |
| Explicit extent, filtered | 43,091 / 46,698 | 9,718 / 88,556 | [JSON](regression-explicit-extent.json) |

These measurements prove the repaired call shapes' parity and record their
cost; they do not establish a new AUTO performance threshold. Runtime tests
independently compare every pixel with an authored-pose oracle for the indexed
and scanning paths, including flag changes and explicit rectangle exclusion.
Compiler and runtime tests also cover row offsets beyond 65,535 linked cells.

Reproduce the native measurements from the repository root:

```sh
MTILE_TILE_LINKS_OUT=/tmp/mtile-tilelinks-evidence \
  go test -tags runtimeperfdiagnostic -run '^TestTileLinkTargetDiagnostic$' -v ./cmd/renderbench
```
