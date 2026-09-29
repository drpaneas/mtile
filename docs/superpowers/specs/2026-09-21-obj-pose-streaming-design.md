# OBJ poses, 4bpp banks, and VRAM streaming

Status: slice 1 shipped. The 8bpp object canvas plus Mode 0 pose streaming
slice is specified in
[2026-09-22-obj-bank-mode0-stream-design.md](2026-09-22-obj-bank-mode0-stream-design.md).
4bpp banks (slice 2 as originally written) and inheritance (slice 4) remain
later. This file still records the program-level model.

Date: 2026-09-21

## Goal

Mtile must author and draw fighting-game characters the way CPS-1 and GBA
actually stored them: unique tiles in ROM, poses as recipes, VRAM as a cache
of the poses on screen. The Sprite Lab filmstrip is not the warehouse.

`examples/nerdo` is the first fixture, not the product. Existing `spr`,
`sspr`, `map`, and Celeste semantics stay unchanged.

## Why this, not a taller sheet

A 40x80 `sspr` rectangle reserves fifty consecutive 8x8 cells, including
empty air. CPS-1 stored 16x16 tiles packed like a puzzle and composed a pose
from a scatter list ([Sanglard, Street Fighter II paper trails](https://fabiensanglard.net/sf2_sheets/)).
GBA Revival then kept that archive on the 8 MB cart and streamed the current
poses into 32 KB of OBJ VRAM.

Mtile today has one shared 8bpp sheet, resident unique tiles, and no pose
object. Growing the sheet cannot hold a roster and cannot enter Mode 0 once
unique object tiles exceed 512.

## Non-goals for slice 1

- Separate BG vs OBJ sheets
- 4bpp authored object tiles
- OBJ VRAM working-set streaming
- Character inheritance (Ken = Ryu tiles + patches + pal)
- Pose editor UI or PNG-sequence packer
- `tcopy`, Stage 6 map-preview, or tile-type metadata

Those are slices 2-4 and parallel tracks. They must not leak into the first
PR.

## Roadmap exception

`docs/graphics-model.md` currently holds Stage 8 until Stages 3 and 6 settle.
`tcopy` animates background cells. Stage 6 visualizes map cameras. Neither
unlocks object poses. Slice 1 is an explicit, narrow pull-forward of Stage 8:
pose assets plus a Mode 4 draw builtin. The hold remains for banks, 4bpp, and
streaming.

## Architecture

```text
assets/poses.json  -->  compact PoseIndex + PoseParts in ROM
                              |
                              v
Lua pspr(id, x, y, flipx, flipy)
                              |
              +---------------+---------------+
              |                               |
        Slice 1: Mode 4                 Later: Mode 0
        blit each 8x8 part              upload pose tiles
        via existing Spr()              into OBJ VRAM slots
```

The logical spritesheet remains the pixel source. A pose never copies pixels
at load time. `sset` on a referenced cell is visible on the next `pspr`, the
same contract as `spr`.

## Slice 1: pose assets + `pspr` (Mode 4)

### Asset file

Optional `assets/poses.json`. Missing file means zero poses and is not an
error. Existing projects keep building.

```json
{
  "version": 1,
  "poses": [
    {
      "name": "ryu_idle_0",
      "parts": [
        {"tile": 31, "x": 8, "y": 0, "flipX": false, "flipY": false}
      ]
    }
  ]
}
```

Rules:

- `version` must be `1`.
- Pose IDs are 0-based array indices. `name` is documentation only; it must
  be unique when non-empty.
- `tile` is a spritesheet cell ID in `0 .. SpriteN-1`.
- `x` and `y` are signed pixel offsets from the `pspr` origin to the
  unflipped 8x8 cell's top-left.
- `flipX` / `flipY` default false.
- At most 128 parts per pose, 4096 poses, 65535 parts total.
- Empty `parts` is a valid no-op pose.
- Authors omit fully transparent cells. The runtime still draws listed
  cells, including all-zero ones, so a later `sset` cannot disappear.

### Runtime records

```go
type PosePart struct {
    Tile  uint16
    X, Y  int16
    Flags uint8 // bit 0 = flipX, bit 1 = flipY
}

type PoseRange struct {
    Start uint16
    Count uint16
}
```

Pose tables live in ROM. `Init` copies slice headers onto `AssetMemory`.
There is no EWRAM pixel copy for poses. `assetLensExact` accepts empty pose
tables so current fixtures keep compiling.

### Lua API

```lua
pspr(id, x, y)
pspr(id, x, y, flip_x)
pspr(id, x, y, flip_x, flip_y)
```

- Arity 3..5, zero results.
- `id` is an integer pose index. Out of range is a no-op, like `spr`.
- `x`, `y` are world pixels. Camera subtraction matches `spr`.
- Whole-pose flip XORs each part's flip flags and mirrors offsets around the
  origin:

```text
ox, oy = part.X, part.Y
if flip_x then ox = -part.X - 8
if flip_y then oy = -part.Y - 8
part_flip_x = part.flipX xor flip_x
part_flip_y = part.flipY xor flip_y
spr(part.tile, x + ox, y + oy, 1, 1, part_flip_x, part_flip_y)
```

- Part order is painter order (later parts on top).
- Clip, pal, and palt are whatever `Spr` already does.
- No scaling. No non-8x8 parts in slice 1.

### Renderer contract

`pspr` is **Mode 4 only**.

- Forced Mode 0: fail with the existing `M0_MODE4_BUILTIN` path
  (`name+"() requires the Mode 4 framebuffer"`).
- AUTO: fall back to Mode 4 with the usual source-positioned diagnostic.
- Do not classify it `RendererBoth` or `RendererMode0Conditional` until a
  later slice has pixel-parity fixtures and a VRAM working-set proof.

This is a public Lua API. The same change must update:

1. `Builtins` in `internal/transpiler/transpiler.go`
2. `BuiltinRendererCompatibility` in `internal/transpiler/render_modes.go`
3. `docs/render-modes.md`
4. Parser/transpiler/runtime/renderer-selection tests
5. Exhaustive counts in `render_modes_test.go` (Mode 4 only 7 -> 8)
6. Zero-result classification in `semantic_regressions_test.go`

### Compatibility

- No `poses.json` and no `pspr` call: bit-identical to today.
- `spr` / `sspr` rectangles keep working, including `nerdo`'s current idle
  tables, until that example is migrated.
- Mode 0 games that never call `pspr` are unaffected.

### Tests (slice 1)

- Missing file loads as empty.
- Invalid version, duplicate name, out-of-range tile, too many parts: load
  error with the file name in the message.
- `PSpr` of a two-part pose matches an equivalent `Spr` sequence, including
  camera, whole-pose flip, and per-part flip.
- Out-of-range id and empty pose: framebuffer unchanged.
- `sset` on a posed tile is visible on the next `PSpr`.
- Transpile `pspr(0,1,2)` to `c.PSprV(...)`.
- Forced Mode 0 + `pspr` fails. AUTO + `pspr` selects Mode 4.
- `go test ./internal/transpiler ./internal/build ./cmd/mtile` and
  `cd runtime && go test ./...` stay green.

### Fixture

After the engine path is green, `examples/nerdo` switches the four idle
frames from `sspr` rectangles to `pspr` poses. Each pose lists only
non-empty 8x8 cells from the existing sheet. Project renderer stays `auto`
and therefore Mode 4.

## Slice 2: 4bpp OBJ bank (later)

The 8bpp object sheet itself is specified in the 2026-09-22 document. What
remains of this slice is 16-color authored object tiles, packed 4bpp at
**build time**. Do not convert 8bpp at runtime; that candidate was measured
and dropped (`docs/benchmarks/discord-hardware/tiles.md`, E05). Logical
256-color `spr`/`sset` for old projects does not change. GBA OBJ palettes
become the hardware mapping for object banks.

## Slice 3: stream the working set (later)

The 8bpp, two-site, single-buffer stream is specified in the 2026-09-22
document. What remains of this slice is 4bpp working-set streaming,
double-buffering, cross-character tile sharing, and audio-vs-DMA cadence
once 4bpp banks exist.

Reclassify `pspr` to Mode 0 conditional only after paired pixel-parity
fixtures and the vblank upload proof (that reclassify is part of 2026-09-22,
still 8bpp). Unsupported shapes keep failing forced Mode 0 / AUTO Mode 4.

## Slice 4: inheritance (later)

A character may name a base pose set, a patch tile list, and a palette bank.
Ken is Ryu plus patches plus pal, not a second full sheet.

## Success bar for the whole program

Thirty or more Ryu poses, two characters on screen, Mode 0 pixel-identical
to Mode 4, proven OAM / scanline / vblank upload, Celeste and current
`spr`/`sspr` untouched.

Slice 1 succeeds when a Mode 4 cart can draw packed poses through `pspr`
without changing any other public API.
