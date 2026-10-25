# Reusable backgrounds and the hardware OBJ experiment

**Historical follow-up, superseded for actual Celeste by
[the grouped-parallax/OBJ-canvas implementation](celeste-parallax-2026-09-06.md).**
The ordinary `parallax-rectangles` example and its map-loop measurements remain
relevant. The old scene-specific canvas and separate sprite-promotion path
described below have been replaced, and the earlier statement that Celeste's
source/assets are unchanged no longer describes the current project.

## What was changed in mtile versus Celeste

The original tile-canvas experiment changed **mtile's runtime and build path**;
it retained the Celeste Lua source. However, it is not a general renderer.
Its runtime explicitly reserves palette banks 4/5, maps rear colors 206/11/14,
encodes opaque black as index 255, and admits one immutable 30x20 map with at
most 19 nonzero terrain tiles, 15 terrain colors, and small camera offsets.
Those are scene-specific assumptions, even though the source lives in mtile.
The opt-in build compiles through the Mode 4 contract and does not establish
general tile-canvas eligibility. Composed `pget`, arbitrary palette use, map
mutation, and general clear/drawing sequences remain outside its proven scope.

The earlier compiler arithmetic/condition/table-write optimizations and the
pixel-packing/dirty-run algorithms are reusable mechanisms. They should not
be confused with the scene-specific allocation and composition plan around
the private canvas. Generalizing that renderer still requires deriving the
plan from assets and source analysis, rejecting unsupported semantics, and
proving its memory and timing bounds. The public Mode 0/AUTO contract remains
unchanged.

This follow-up changes no original Celeste Lua or assets. It adds:

* A generic optimization to the ordinary Mode 0 map materialization loop.
* An editable, independent `testdata/projects/parallax-rectangles` demonstration using
  the existing public renderer and APIs.
* A separately enabled hardware OBJ experiment on top of the private canvas.

## Four backgrounds: the useful result

Mtile already supports independent `camera()` state for successive `map(bg)`
calls and hardware OBJs for compatible `spr()` calls. The new example uses
BG0/BG1/BG2 for rectangle groups moving at 1/2/4 pixels per update, BG3 for
stationary terrain, and OBJs for its actor and HUD. It authors the rectangles
once as repeated tiles and moves their source windows through BG scroll.

Logical BG0-to-BG3 in mtile is back-to-front order. Hardware depth is assigned
through priority bits; BG3 is not inherently frontmost. See the primary
[Tonc background documentation](https://gbadev.net/tonc/regbg.html) and
[OBJ documentation](https://gbadev.net/tonc/regobj.html).

The benchmark exposed a real runtime problem at simultaneous tile crossings:
each layer rebuilt its map shadow using repeated per-cell source/storage
checks and a byte-wise clear. `runtime/mode0.go` now resolves compact storage
once per pass, computes clipped row bounds once, reads compact cells directly,
uses equivalent power-of-two screen indexing, and clears the aligned shadow
with the existing DMA fill. Streaming retains its checked lookup path.
Map/flag mutation invalidation and renderer admission rules are unchanged.

| Same full-height rectangle scene | Median cycles | p95 cycles | Missed updates |
| --- | ---: | ---: | ---: |
| Mode 4 reference | 566,179 | 575,391 | 190 |
| Mode 0 before this runtime change | 66,191 | 900,512 | 71 |
| Mode 0 after this runtime change | 66,111 | 231,345 | 0 |

All 260 paired updates matched exactly. Before/after retained the same project
hash and Mode 4 ROM. All three moving layers still cover 20 rows and can cross
tile boundaries together. The p95 reduction is 74.3%; no staggering or reduced
map bounds conceals the original expensive case. A separate native probe
confirmed four enabled BGs, priorities 3/2/1/0, and 43 active hardware OBJs,
including the actor. No scroll-register readback is claimed: those registers
are write-only, so the pixel comparison verifies the visible result.

These measurements characterize the small demo, not the full Celeste port.
The comparison uses the existing pixel-parity gate: its 20,560-byte EWRAM
saving is below the broader promotion gate's 24 KiB requirement, and that gate
also requires no misses in the Mode 4 control. No gate threshold was changed.

Open and run the example normally:

```sh
go run ./cmd/mtile edit testdata/projects/parallax-rectangles
```

Celeste currently moves 17 clouds independently, with random speeds and new
vertical positions on wrap. Replacing them with three shared motions is a
deliberate visual adaptation. The example demonstrates that option without
silently changing the port's appearance or physics. Its README explains the
asset layout, regeneration, controls, and benchmark commands.

## Hardware OBJs: exact output, slower in this implementation

The new `mtile_tilecanvas_obj` path contains no Celeste IDs, colors, names, or
movement constants. It promotes eligible calls from any compatible immutable
sheet: up to eight normal 8x8 OBJ cells, identity draw palette, default
source-zero transparency, and a full-screen clip. Arbitrary logical sprite
IDs resolve through the existing compact sheet index. Other calls continue
through software drawing.

The sheet uploads once into OBJ VRAM. Each frame uploads only the active OAM
prefix plus entries that must be disabled after disappearance. A 256-bit set
identifies palette entries used by the sheet. The eight-cell cap keeps the
additional ARM state to 184 bytes, preserving the existing 4 KiB EWRAM reserve;
the first larger staging design was rejected by the linker, not accommodated
by reducing that reserve.

Later sprites use lower OAM indices to retain painter order. If a later
software primitive overlaps queued sprites, the required prefix is rendered
back into the software canvas using its saved draw-time state. This preserves
hair, body, particles, clipping, camera changes, and opaque UI. A frozen draw
retains committed OAM along with the backgrounds.

The native comparison uses the unchanged 162-update Celeste completion route:

| Celeste runtime | Median cycles | p95 cycles | Maximum cycles | Updates over 561,792 cycles | Observed update cadence |
| --- | ---: | ---: | ---: | ---: | ---: |
| Mode 4 | 587,900 | 743,270 | 819,260 | 111/162 | 21.99 Hz |
| Existing private canvas | 542,782 | 680,351 | 734,765 | 58/162 | 24.13 Hz |
| Private canvas + OBJs | 578,557 | 732,749 | 815,551 | 100/162 | 22.45 Hz |

The OBJ candidate is **6.6% more expensive at the median** than the existing
canvas. Queue/order checks, fallback rasterization and OAM/palette preparation
outweigh the saved sprite rasterization in this mixed workload. It is therefore
kept as a separately enabled experiment, not added to the existing opt-in or
recommended as a performance improvement. This result does not imply that
OBJs are slow in an ordinary tile/sprite game: the parallax demo uses them
successfully through the existing Mode 0 renderer.

Both candidates matched Mode 4 across all 280 compared updates, including
spawn, route, freeze, shake, completion, restart, death and respawn. Hardware
OBJs were active in 254 updates, with up to eight entries. All candidate commits
finished inside VBlank. The original-source playable OBJ ROM also passed boot
and 120 LCD frames of normal input with active sound and no heap/audio errors.
Its EWRAM headroom remains tight: 4,248 bytes, including the 4 KiB reserve.
These are mGBA results; physical GBA verification remains outstanding.

To inspect the optional candidate separately:

```sh
go run ./cmd/mtile edit -experimental-tilecanvas-objects examples/celeste
```

The original `-experimental-tilecanvas` still selects the faster canvas-only
experiment. The new flag is scoped to the chosen editor project and does not
modify `project.json` or public AUTO eligibility.

## Reproduction and retained evidence

```sh
MTILE_TILECANVAS_DIAGNOSTIC=1 MTILE_TILECANVAS_OBJECTS=1 \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestCelesteTileCanvasDiagnostic$' -count=1 -timeout=15m -v

go run ./cmd/renderbench -project testdata/projects/parallax-rectangles \
  -samples 260 -gate parity -out /tmp/mtile-parallax-rectangles
```

Selected ROMs, traces, reports, generated code and PNGs are saved locally under
`build/experiments/hardware-layers/`; `manifest.json` records their hashes and
original evidence directories. The final Celeste native evidence directory is
`$TMPDIR/mtile-celeste-tilecanvas-385628194`.

Host verification covers the old map-loop oracle versus the optimized loop,
four-layer composition, sparse planes, source changes, masks, negative origins,
tile boundaries, and edits before/after drawing, including compact and streaming
builds. OBJ tests independently decode OAM and BG pixels, and exercise palette,
clip, camera, flips, capacity fallback, freeze, erasure, DMA alignment, and zero
allocation. Required transpiler/build/CLI and default runtime suites pass.
