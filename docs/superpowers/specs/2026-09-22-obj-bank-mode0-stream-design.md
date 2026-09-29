# 8bpp object bank and Mode 0 pose streaming

Status: draft for review. Implements a narrowed Stage 8 pull-forward: a
second 8bpp object sheet plus Mode 0 streaming of proven `pspr` poses.
4bpp banks and Ken inheritance stay later.

Date: 2026-09-22

Parent: [2026-09-21-obj-pose-streaming-design.md](2026-09-21-obj-pose-streaming-design.md)
(slice 1 shipped; this document replaces the "later" 8bpp reading of slices
2-3, not the 4bpp or inheritance slices).

## Goal

Authors paint fighter frames on a Sprite Lab object canvas that is not the
240x160 GBA screen. `pspr` draws those poses in Mode 4 from the object
sheet, and in Mode 0 by DMA of the poses actually on screen into OBJ VRAM
slots 0-447. Forced Mode 0 is legal for proven call shapes. AUTO may select
Mode 0 for the same shapes.

`examples/nerdo` is the Mode 0 + object-sheet fixture. Celeste, `spr`,
`sspr`, and `map` stay bit-identical.

## Non-goals

- 4bpp object tiles, OBJ palette banks, or build-time 8bpp-to-4bpp conversion
- Character inheritance (Ken = Ryu + patches + pal)
- Pose editor UI or PNG-sequence packer
- In-Lab object-sheet resize after create
- Copy/paste between the BG sheet and the object sheet
- 16x16 packing of pose parts into one OAM entry
- `spr`/`sspr` and `pspr` in the same Mode 0 `_draw`
- Cross-character sharing of unique tiles in OBJ VRAM
- Double-buffered pose windows
- `oset` / mutating the object sheet at runtime
- `tcopy`, Stage 6 map preview, tile-type metadata

## Roadmap exception

`docs/graphics-model.md` still holds the rest of Stage 8 (4bpp allocation,
native OBJ packing as a general sheet feature) until Stages 3 and 6 settle.
This spec is an explicit pull-forward of two Stage 8 pieces: a separate
logical object sheet, and an OBJ VRAM working set for `pspr` only.

## Architecture

```text
assets/spritesheet.json  -->  map / spr / sspr / sget / sset / fget / fset
assets/objsheet.json     -->  optional object warehouse (8bpp cells)
assets/poses.json        -->  recipes; tile IDs index the active pose bank

Lua pspr(id, x, y, flipx, flipy)
        |
        +-- Mode 4: blit each 8x8 part from the active pose bank
        |
        +-- Mode 0: vblank DMA unique part tiles into a per-site slot
            window in OBJ VRAM 0-447; OAM 8x8 with remapped attr2
```

Active pose bank:

- If `assets/objsheet.json` is absent, pose `tile` IDs index the spritesheet
  (slice 1 behavior).
- If the file is present, pose `tile` IDs index only the object sheet.
  Spritesheet IDs are invalid in that project.

The Lua `pspr` arity, flip math, painter order, and out-of-range no-op do
not change. Pixel source and Mode 0 lowering change.

Mode 0 never identity-maps logical pose tiles onto OBJ VRAM. Slot 31 is not
object-sheet cell 31. That is why `spr`/`sspr` cannot share `_draw` with
`pspr` in this slice: they still use identity mapping into the same 0-447
range.

## Editor and assets

### Object sheet file

Optional `assets/objsheet.json`. Same JSON schema as `spritesheet.json`
(`width`, `height`, sparse `sprites` records, 8bpp palette indices). Missing
file is not an error.

Load rules:

- Width and height must be positive multiples of 8.
- Cell count must stay at or under `MapStreamingThresholdCells` (16384).
  That is the existing "too large to fully materialize in EWRAM" budget,
  reused here so a dense object sheet cannot exceed the map stack's copy
  threshold. `MaxSpriteN` (65536) remains a harder upper bound but does not
  bind.
- Each axis must stay at or under 1024 pixels. That is not a new invented
  size: 16384 cells pack as a 128x128 cell square, and 128*8 = 1024. Name
  the implementation constants `MaxObjSheetCells = MapStreamingThresholdCells`
  and `MaxObjSheetPx = 8 * 128`.
- Invalid JSON, overlap, duplicate IDs, and out-of-range IDs fail the load
  with `objsheet.json` in the message, using the same checks as the
  spritesheet.

A 1024x1024 8bpp buffer is 1 MB dense. Generated carts must keep the
object sheet in compact ROM (unique 8x8 cells, same idea as the spritesheet
sampler). Mode 4 and Mode 0 both sample that ROM. Do not copy the full
pixel rectangle into EWRAM.

There is no object-sheet flags array. `fget`/`fset` stay on the BG sheet.
`SpriteRecord.Flags` on object-sheet entries is ignored if present.

### Sprite Lab

The Sprite Lab header gains a **BG | OBJ** toggle.

- **BG** edits `spritesheet.json`. Map flags, tile 0 lock (map eraser), and
  current tools stay as they are.
- **OBJ** edits `objsheet.json`. Map flags are hidden. Object tile 0 is
  paintable. Hardware print reservation 480-511 does not lock object-sheet
  cell IDs.

If `objsheet.json` is missing, OBJ mode shows an empty state and a Create
Object Sheet action. Create asks width and height (multiples of 8), default
480x320 (two GBA frames: `2*240` by `2*160`), maximum 1024x1024. Create
writes the file. This spec does not resize an existing object sheet in the
Lab.

Creating the file switches the active pose bank. Existing `poses.json`
entries that still name spritesheet cell IDs fail flatten until they are
remapped to object-sheet IDs. The editor does not rewrite poses on create.

Pencil, pan, zoom, selection, and the sheet picker reuse the current
implementation against the active backing store. Copy/paste stays on the
active sheet. The 256-color project palette is shared.

`poses.json` stays hand-authored. No pose editor.

New projects do not create an object sheet.

### Pose flatten

`flattenPoses` rejects a part whose `tile` is outside `0 .. BankN-1`, where
`BankN` is object-sheet cell count when the object sheet exists, otherwise
spritesheet cell count. The error names the bank and the pose index.

`poses.json` version remains 1. No per-pose bank field.

## Mode 4 runtime

`PSpr` keeps the slice 1 flip and no-op contract.

When no object sheet is loaded, each part still calls `Spr` on the
spritesheet. `sset` on a referenced spritesheet cell is visible on the next
`PSpr`.

When an object sheet is loaded, each part blits that object-sheet 8x8 cell
with the same camera, clip, pal, and palt behavior `Spr` uses for a 1x1
sprite. It must not read the spritesheet. There is no `oset`; the object
sheet is immutable.

Mode 4 still honors slice 1 when the pose bank is the spritesheet: `sset`
on a referenced cell is visible on the next `PSpr`. Mode 0 does not take
that coupling (see Exclusion).

## Mode 0 proof

`pspr` is **Mode 0 conditional**, not Mode 4 only, not `RendererBoth`.

The compile-time selector applies the same proof to forced Mode 0 and AUTO.
Forced Mode 0 fails closed. AUTO records the diagnostic and selects Mode 4.

A `pspr` call is Mode 0 eligible only when every condition below holds.

### Draw shape

- The call is reachable from `_draw` and follows `cls`, using the same draw
  walk as `spr`.
- Arity 3..5.
- `id` is a statically proven finite integer set, using the same analysis as
  `spr` IDs (literals, finite assigned scalars, bounded counters, the numeric
  builtins already admitted for `spr`). `x`, `y`, `flip_x`, and `flip_y` may
  be dynamic.
- Every proven `id` is in range for `PoseIndex`, or is a runtime no-op id
  (negative / too large). Out-of-range proven values do not add tiles or OAM.
- No proven in-range pose has more than 128 parts (already an asset max;
  the analyzer still treats part count as the OAM cost).

`examples/nerdo`'s `frame` if-chain that yields 0, 1, 2, or 3 is in the
finite-integer proof.

### Exclusion

- The same `_draw` walk contains no `spr` or `sspr` call.
- `map` and `print` may coexist. Text still owns OBJ tiles 480-511.
- `sset` mutates the BG spritesheet only. It does not disqualify Mode 0
  `pspr` when the active pose bank is the object sheet: pose pixels are
  immutable ROM, and `sset` keeps its existing Mode 0 rules (`M0_MUTABLE_SHEET`
  placement, `maxMode0SsetPerUpdate = 16` VBlank cap, four-BG plus native
  audio restriction).
- When the active pose bank is the spritesheet and any `sset` is reachable,
  Mode 0 `pspr` fails with `M0_MUTABLE_SHEET`. This slice does not add a
  second dirty-tile pipeline that re-uploads streamed pose windows after
  `sset`. Mode 4 still shows those mutations. A spritesheet-backed pose
  project with no `sset` is read-only and may stream from the same compact
  sheet sampler `spr` already uses.

### Sites and windows

- Each reachable `pspr` call expression in the `_draw` walk is one site.
  Loop copies use the same `copies` multiplier as `spr`. If/else branches
  that each contain `pspr` are separate sites; this slice does not prove
  mutual exclusion.
- At most two sites after copies. A third site fails the proof.
- For site `i`, let `S_i` be the proven in-range pose ID set. Unique-tile
  count of a pose is the number of distinct `part.tile` values, first
  occurrence in part order. `U_i` is the maximum unique-tile count over
  `S_i`, or 0 if `S_i` is empty.
- Slot windows are contiguous in walk order: site 0 uses `[0, U_0)`, site 1
  uses `[U_0, U_0+U_1)`.
- `U_0 + U_1 <= 448`. Tiles 448-479 stay the packed `spr` cache (unused in
  this slice because `spr` is forbidden). Tiles 480-511 stay text.

There is no runtime slot allocator and no sharing of unique tiles across
sites.

### OAM and scanline

- Each pose part is one 8x8 OAM entry. This slice does not pack 16x16.
- Site `i` budgets `C_i` entries, the maximum part count over `S_i`.
- Total object entries are `C_0 + C_1` plus whatever `print` already budgets.
  The total must be `<= 128`.
- Scanline cycles use the existing 8x8 `spr` 1x1 unpacked profile for each
  budgeted part, including dynamic Y. The total must stay under the current
  1226-cycle cap (`M0_OBJ_BUDGET`).

### Diagnostics

All failures are source-positioned.

| Condition | Tag | Message shape |
| --- | --- | --- |
| Unproven `id`, bad arity, id proof failure | `M0_PSPR_SHAPE` | `Mode 0 pspr() id must be a proven finite integer set` (or arity text) |
| `spr`/`sspr` in the same `_draw` | `M0_PSPR_SHAPE` | `Mode 0 pspr() cannot share _draw with spr()/sspr()` |
| More than two sites | `M0_RESOURCE_LIMIT` | names the site cap |
| Window `U_0+U_1 > 448` | `M0_RESOURCE_LIMIT` | names the 0-447 window |
| OAM or scanline miss | `M0_OBJ_BUDGET` | same budget text family as `spr` |
| Pose tables missing while `pspr` is proven in-range | `M0_PSPR_SHAPE` | `Mode 0 pspr() requires pose tables` |
| Spritesheet pose bank plus reachable `sset` | `M0_MUTABLE_SHEET` | `Mode 0 pspr() cannot stream a spritesheet pose bank that sset() mutates` |

`M0_PSPR_SHAPE` is new and follows `M0_SPR_SHAPE` / `M0_PRINT_SHAPE`.
`M0_RESOURCE_LIMIT` is the existing tag for sheet-alignment, `sset` caps,
and tile-budget overflows; two-site and window overflow use it for the
same reason. Do not keep `M0_MODE4_BUILTIN` for `pspr`. That tag is for
APIs that can never be Mode 0.

## Mode 0 runtime

Generated carts that pass the proof link a pose-stream object path.

Per site, keep the last presented pose id. On change, during vblank, DMA
each unique 8x8 tile of the new pose from the compact pose-bank ROM sampler
into that site's window, in first-occurrence part order. Unused bytes in a
window larger than the current pose may stay stale; they are not referenced
by OAM.

Do not re-sample a mutable spritesheet into the window. That combination is
rejected at proof time. Immutable banks (object sheet, or spritesheet with
no `sset`) upload from ROM.

The VBlank cost is `64 * (tiles actually copied this frame)` bytes of DMA32,
worst case `64 * (U_0 + U_1)` if both sites change. Classification may ship
only after a paired measurement of one pose change (nerdo's worst idle) in
the same bench family as `maxMode0SsetPerUpdate`. If that upload plus the
existing 16-`sset` profile cannot be shown to fit, keep `pspr` out of Mode 0
rather than inventing an unmeasured tile cap. Nerdo's idle unique-tile count
is the evidence target, not a 448-tile every-frame upload.

Then write one OAM 8x8 entry per part:

- Screen position is `pspr` origin plus the slice 1 offset math, minus
  camera, matching Mode 4.
- Flip bits are the composed part flags, matching Mode 4.
- `attr2` tile index is `windowBase + localIndex(part.tile)`, not
  `part.tile`.

Out-of-range or empty poses hide that site's OAM entries and skip upload.

Clip, pal, palt, and OBJ depth vs `map` follow the existing Mode 0 object
rules for `spr` (painter order, Stage 5 depth lanes). `pspr` is an object
draw op in `mode0WalkDraw`.

Do not call identity-mapped `Spr` from Mode 0 `PSpr`.

## Renderer contract

Same change must update:

1. `Builtins` in `internal/transpiler/transpiler.go` (arity unchanged; keep
   `"pspr": {"PSprV", 3, 5, 0}`).
2. `BuiltinRendererCompatibility`: `pspr` moves from `RendererMode4Only` to
   `RendererMode0Conditional`.
3. `docs/render-modes.md`: classification paragraph and the graphics table
   row. Exact reason: a proven finite pose-id set whose worst unique tiles
   and parts fit the per-site 0-447 windows and OAM/scanline budgets, with
   no `spr`/`sspr` in `_draw` and without a spritesheet pose bank that
   `sset` mutates, is representable as hardware OBJ. Unproven IDs, mixed
   sheet objects, a mutable spritesheet pose bank, or a working set that
   misses the window fail forced Mode 0 and make AUTO select Mode 4.
4. Parser/transpiler/runtime/renderer-selection tests, including pixel-parity
   fixtures and a DMA/frame benchmark for one pose change.
5. `render_modes_test.go` exhaustive counts: Mode 4 only 8 to 7; Mode 0
   conditional 8 to 9. Remove `pspr` from the Mode-4-only name list; add it
   to the conditional list.
6. `semantic_regressions_test.go` keeps `pspr` as a zero-result graphics
   builtin.

Mode 0 support must preserve exact `pspr` semantics. Do not classify this
`RendererBoth`. A similar picture is not enough.

## Compatibility

- No object sheet and no `pspr`: bit-identical to today.
- No object sheet, Mode 4 `pspr`: bit-identical to slice 1.
- Object sheet present, Mode 4 `pspr`: pixels come from the object sheet.
  Callers that still point poses at spritesheet IDs fail at flatten, not at
  runtime.
- Mode 0 games that never call `pspr` are unaffected.
- Celeste must keep building and rendering as it does today.

## Errors (assets)

- Missing `objsheet.json`: empty object bank, poses index the spritesheet.
- Present but invalid `objsheet.json`: load fails, file name in the message.
- Dimensions not a multiple of 8, non-positive, above 1024 on an axis, or
  above 16384 cells: load fails.
- Pose tile outside the active bank: flatten fails with bank name, pose
  index, tile, and `BankN`.

## Tests

Asset and editor:

- Missing object sheet loads as empty.
- Invalid object sheet fails with `objsheet.json` in the message.
- Create-default 480x320 persists; 1024x1024 is accepted; 1025 on an axis
  or more than 16384 cells is rejected.
- Pose flatten uses spritesheet `SpriteN` without an object sheet, and object
  `SpriteN` when the sheet exists.
- Editor BG/OBJ toggle paints and saves the matching file; flags stay on BG.

Runtime Mode 4:

- `PSpr` from object-sheet cells matches an equivalent `Spr` sequence whose
  pixels were copied onto a spritesheet of the same cells (camera, clip,
  whole-pose flip, per-part flip).
- Without an object sheet, existing `runtime/pose_test.go` parity remains.

Renderer selection:

- Forced Mode 0 + unproven `pspr` id fails `[M0_PSPR_SHAPE]`.
- Forced Mode 0 + `pspr` and `spr` in `_draw` fails `[M0_PSPR_SHAPE]`.
- Forced Mode 0 + three `pspr` sites fails `[M0_RESOURCE_LIMIT]`.
- Forced Mode 0 + spritesheet pose bank + reachable `sset` fails
  `[M0_MUTABLE_SHEET]`.
- Forced Mode 0 + object-sheet poses + legal `sset` on the BG sheet still
  admits `pspr` (subject to the existing 16-call `sset` cap).
- Forced Mode 0 + nerdo's 0-3 if-chain succeeds (`M0_FORCED_PROVEN` or the
  current forced-success reason).
- AUTO + unproven `pspr` selects Mode 4 with a source-positioned diagnostic.
- AUTO + proven nerdo shape may select Mode 0.

Pixel parity and budget:

- Host or cart fixture: Mode 0 vs Mode 4 pixel-identical for nerdo idle
  unflipped and whole-pose `flipX`.
- Object budget includes `pspr` parts in OAM and scanline totals.
- Benchmark: DMA cost of one pose change (window upload of the worst nerdo
  idle), recorded with the other Mode 0 object benches.

AGENTS.md suite stays green:

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

## Fixture

Migrate `examples/nerdo` so the four idle unique tiles live on
`assets/objsheet.json` and `poses.json` indexes that sheet. The spritesheet
may be a minimal BG sheet. Project renderer stays `auto`. The proven
if-chain remains the Mode 0 admission test; pixel-parity compares Mode 0 and
Mode 4 builds of that cart.

The object sheet for nerdo may stay 240x160 if that holds the four idles.
Create-default 480x320 is for new sheets, not a requirement on this fixture.

## Success bar

- Extra frames have a Sprite Lab OBJ canvas that can be 480x320 (and up to
  1024x1024) without growing the BG filmstrip.
- `mtile build -renderer=mode0` succeeds for nerdo.
- Mode 0 nerdo idle matches Mode 4 pixels for unflipped and `flipX`.
- Celeste and current `spr`/`sspr`/`map` paths are unchanged.
- Unproven, mixed, or spritesheet-`sset` `pspr` still cannot silently enter
  Mode 0.

## Implementation order (for the later plan)

1. Object sheet load/save/flatten and Mode 4 `PSpr` bank switch, with nerdo
   migrated, still Mode 4 only in the classifier.
2. Analyzer proof, diagnostics, object-budget walk, renderer reclassify.
3. Mode 0 stream runtime, pixel-parity, benchmark, docs.

Step 1 is not shippable as a public renderer-contract change by itself.
The merge includes steps 1-3 so `pspr`'s classification and semantics land
together, per `AGENTS.md`.
