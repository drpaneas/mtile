# Celeste with authored parallax and a generic OBJ canvas

This supersedes the earlier scene-specific tile-canvas and optional sprite
promotion experiments. The actual `examples/celeste` project now contains
three authored cloud planes and its original terrain on BG3. Mtile's private
renderer has been replaced with ordinary tiled backgrounds plus a sparse
foreground OBJ canvas. The palette, sprite IDs, room dimensions, and cloud
speeds belong to the game assets/Lua rather than renderer conditionals.

Open the real project from the repository root:

```sh
go run ./cmd/mtile edit -experimental-tilecanvas examples/celeste
```

Cmd/Ctrl+R builds and runs it. The opt-in is scoped to that editor process and
selected project. Without it, the same source and assets retain the ordinary
renderer policy. Forced Mode 4 is the reference for the adapted scene.
This is not a public `project.json` renderer value or a new AUTO profile.

## Changes in the game

The four-plane map is now 60×20 cells. BG0–2 each contain a 30-column cloud
motif repeated once horizontally. A new unflagged solid tile, logical ID 226
and color 206, constructs 17 rectangles across the three planes. All 127
previously authored sprites are unchanged.

The original 600 terrain cells moved unchanged from BG1 into BG3's left
30 columns; the right half is empty. Collision and entity `mget`/`mset` calls
now select BG3. Their 30×20 room bounds, tile flags, marker interpretation,
movement rules, and physics constants are unchanged.

`cloud_phase` advances on each nonfrozen draw. Cameras produce rightward
movement at 1, 2, and 4 pixels per game update, or 30/60/120 pixels per second
at the target 30 Hz. All four `map` calls are direct statements in `_draw`, as
required by the existing explicit-background contract. The cloud calls request
their complete 60×20 rectangles. The terrain call requests its original 30×20
room. Camera shake is included in all layers and restored before gameplay
drawing; completion can still reset the camera for its overlay.

This intentionally changes the cloud artwork and motion. Three shared
offsets cannot preserve 17 independent randomized cloud trajectories. The old
cloud table continues its original initialization, update, and wrap RNG calls
without raster drawing, retaining the random sequence used by other effects.
The new group phase resets with the same `initialize_effects()` lifecycle.

The educational walkthrough and asset-regeneration instructions are in
[`examples/celeste/PARALLAX.md`](../examples/celeste/PARALLAX.md).

## Changes in mtile

The private runtime shares the ordinary tiled-map materializer. A generic
full-rectangle cache can retain up to 64×32 cells in each 512×256 hardware
background when the camera bounds prove that hardware wrap cannot expose
unrequested repeated pixels. Stable map contents then need only scroll writes.
When that proof does not hold, the existing viewport materializer handles the
map call. This optimization depends on geometry and state, not a game name.

Foreground raster calls still compose into the ordinary 240×160 software
buffer in painter order. An exact coverage mask distinguishes untouched pixels
from explicitly drawn opaque color zero. Occupied 16×16 blocks become
nonoverlapping hardware OBJs with 8 bits per pixel. The canvas stores up to 96 such blocks, using
24 KiB of OBJ tile staging and 768 bytes of OAM staging. Only changed physical
blocks are uploaded; OAM entries for disappeared blocks are disabled.

If opaque logical color zero is present, the packer chooses an unused nonzero
physical OBJ palette index for it. It rejects an exhausted palette rather than
reserving or stealing a particular source color. BG and OBJ palette RAM are
separate, so foreground black can cover the backgrounds without changing their
palette. No Celeste palette banks, cloud colors, sprite IDs, or limit of
19 terrain tiles remain in this renderer.

This differs from promoting individual Lua sprites: an OBJ canvas block can
contain the final mixture of body, hair, smoke, text, and primitives. Their
ordering has already been resolved by software, so the renderer need not
reconstruct an arbitrary interleaving of sprite and primitive calls.

The conversion uses aligned 32-bit reads and writes, a four-pixel coverage
mask, and exact per-byte zero replacement in IWRAM. Its palette preflight
uses byte lookups instead of counting trailing zeros for every covered pixel.
DMA clears the coverage mask each drawn frame; stale framebuffer pixels need
no clear because uncovered pixels are invisible. A combined IWRAM pass writes
the two hardware palettes. These changes address measured conversion/clearing
cost without changing logical colors or pixel coverage.

Tile and map DMA runs are prepared before presentation. A conservative upload
estimate must fit VBlank; presentation waits for a sufficient window and checks
the actual commit afterward. Freeze retains both the previous maps and OAM.
There is no scanline mode switching: the display remains Mode 0.

## Deliberate bounds

- One frame-initial `cls`; skipped drawing retains the previous image.
- Up to four explicitly ordered map planes before any foreground raster
  pixels. Map draws require full clip, identity draw palette, and key-zero
  transparency. A future room with a raster platform between map passes needs
  a separately supported composition; the current room has no active markers
  using that path.
- Immutable spritesheet, at most 768 logical BG tiles, and map rectangles
  supported by the shared 64×32 materializer. Full-rectangle caching is used
  only while its geometry/wrap proof holds.
- At most 96 occupied 16×16 foreground blocks. The canvas is deliberately sparse;
  arbitrary full-screen raster content is outside that capacity.
- Opaque black requires a spare nonzero OBJ palette entry. Composed `pget`
  readback remains unsupported.
- Upload capacity and actual VBlank timing must pass. Unsupported private
  behavior is rejected; users can select the ordinary Mode 4 path. There is no
  automatic live switch between renderers.

The mechanism is reusable, but these bounds do not establish full public Lua
renderer compatibility. Public Mode 0/AUTO classifications and source-positioned
rejection/fallback rules remain unchanged in [`render-modes.md`](render-modes.md).

## Verification and measurements

The final native mGBA run passed two separate comparisons. Across 280 game
updates, the adapted Mode 4 and hybrid renderers produced zero differing RGB
pixels. Across the same scenario, the original and adapted games retained
280 exact gameplay watch snapshots. Coverage includes the 162-update completion
route, spawning, dash freeze, camera shake, completion, restart, death, and
respawn. All checks in `examples/celeste/tests/verify_all.py` also passed.

The required transpiler/build/CLI suites and the complete default and Mode 0
runtime suites passed. Private `TestObjectCanvas` and `TestTileCanvas` tests
also passed with Go's strict pointer checks, covering pixel coverage, palette
capacity, hardware map decoding, mutation snapshots, and lifecycle rejection.

Hardware OBJs were active in all 280 hybrid snapshots, reaching at most
58 committed blocks. The hybrid reported zero VBlank commit violations.
The separate playable editor build passed startup followed by 120 LCD frames
of physical input with active sound and no heap errors.

These results do **not** establish sustained 30 Hz. The complete route gives:

| Scene and renderer | Median cycles | p95 cycles | Maximum cycles | Updates over 561,792 cycles | Observed update cadence |
| --- | ---: | ---: | ---: | ---: | ---: |
| Original clouds, Mode 4 | 587,900 | 743,270 | 819,260 | 111/162 | 21.99 Hz |
| Original clouds, old private canvas | 542,782 | 680,351 | 734,765 | 58/162 | 24.13 Hz |
| Grouped clouds, Mode 4 reference | 1,250,209 | 1,458,647 | 1,550,179 | 148/162 | 12.25 Hz |
| Grouped clouds, new hybrid | 542,957 | 697,083 | 1,069,611 | 62/162 | 23.54 Hz |

The hybrid is substantially faster than rasterizing the new four-map scene in
Mode 4. Compared with the previous scene-specific private renderer, its median
cost is effectively unchanged and its expensive updates are slower. The result
is a more reusable implementation with authored parallax and fewer palette/room
assumptions, not a large overall CPU improvement over the old private path.
The completion overlay remains especially expensive: median 972,672 cycles
and 12.30 observed updates per second. Raster composition and packing the
foreground still cost CPU even though the final display uses hardware OBJs.

| Instrumented build | Free EWRAM |
| --- | ---: |
| Original Mode 4 | 16,264 bytes |
| Old private canvas | 4,312 bytes |
| Adapted Mode 4 | 54,392 bytes |
| New hybrid | 7,344 bytes |

The hybrid gains 3,032 free EWRAM bytes over the old private renderer. Its
instrumented ROM is 405,068 bytes and uses 30,688 of 32,672 available IWRAM
bytes. The actual playable ROM is 389,168 bytes, with 30,696 IWRAM bytes used
and 7,472 EWRAM bytes free. Instrumented and playable figures are kept separate
because the diagnostic watches change the build.

These are emulator measurements of the tested scene and route. A correct
VBlank upload window does not imply that the entire game update fits its
two-LCD-frame budget; the route and completion numbers show the remaining work.

The frozen original project/runtime and SHA256 manifest are retained under
`build/experiments/celeste-parallax/before/`. The final candidate, software
oracle, traces, generated code, screenshots, and playable ROM are under
`build/experiments/celeste-parallax/after/`. The surrounding directory contains
`native-verification.log`, `asset-verification.log`, and
`evidence-locations.json`; the latter records the original temporary paths.

Structural checks confirm all 600 terrain cells
and all 127 original sprites were preserved, and asset regeneration is
idempotent. The ordinary Mode 4 source build also passes after retaining every
explicit `map` call directly inside `_draw`.

The retained asset check reads the real projects but runs regeneration only in
a temporary copy, so it does not overwrite authored assets:

```sh
python3 examples/celeste/tests/verify_parallax_assets.py \
  --baseline build/experiments/celeste-parallax/before
```

`MTILE_PARALLAX_BASELINE` can supply the same baseline path instead. The check
compares original BG1 terrain with new BG3, every original sprite, the one new
cloud tile, all three duplicated motifs, and generator idempotence.

Reproduce the native original/adapted gameplay and adapted-renderer pixel
comparison from the repository root:

```sh
MTILE_CELESTE_PARALLAX_BASELINE="$PWD/build/experiments/celeste-parallax/before" \
MTILE_CELESTE_PARALLAX_NATIVE=1 \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestCelesteParallaxNativeDiagnostic$' -count=1 -timeout=15m -v

python3 examples/celeste/tests/verify_all.py
```

The retained `before/verified-before-evidence.txt` points to the saved original
native traces. To rebuild that original comparison from its frozen runtime:

```sh
MTILE_CELESTE_PARALLAX_BASELINE="$PWD/build/experiments/celeste-parallax/before" \
  go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^TestCelesteParallaxBaselineDiagnostic$' -count=1 -timeout=15m -v
```

Original-versus-adapted cloud pixels intentionally differ. Only the two
renderers of the same adapted scene are required to match every pixel; these
two verification claims must remain separate.
