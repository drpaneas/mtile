# 8bpp object bank + Mode 0 pose streaming Implementation Plan

> **For agentic workers:** Execute inline in this session (tasks are tightly coupled). Do not commit unless the user asks.

**Goal:** Add optional `assets/objsheet.json`, Sprite Lab BG/OBJ authoring, Mode 4 `pspr` from that bank, and Mode 0 streaming of proven `pspr` poses into OBJ VRAM 0-447.

**Architecture:** Object sheet is an optional second 8bpp warehouse. Pose `tile` IDs index it when present, else the spritesheet. Mode 4 blits compact object-sheet ROM. Mode 0 DMA unique 8x8 tiles of the current pose into a per-site window and writes remapped OAM. `pspr` becomes Mode 0 conditional.

**Tech Stack:** Go, assetsync JSON, TinyGo-generated cart, existing Mode 0 object runtime, `go test`.

**Spec:** `docs/superpowers/specs/2026-09-22-obj-bank-mode0-stream-design.md`

## Global Constraints

- Public Lua API change: `pspr` moves from `RendererMode4Only` to `RendererMode0Conditional` in the same merge as runtime, tests, and `docs/render-modes.md` (`AGENTS.md`).
- Do not classify `pspr` as `RendererBoth`.
- Missing `objsheet.json` keeps slice 1 pose-on-spritesheet behavior.
- `spr` / `sspr` / `map` / Celeste stay bit-identical.
- No 4bpp, Ken inheritance, pose editor, sheet resize, 16x16 pose packing, or `oset`.
- Never use the em dash character in new user-facing docs or diagnostics.
- Do not skip git hooks. Do not commit unless the user asks.

## File map

- Create: `internal/assetsync/objsheet.go`, `internal/assetsync/objsheet_test.go`
- Create: `runtime/pose_obj.go` (Mode 4 object-cell blit helpers if needed)
- Create: `runtime/mode0_pose.go` (`//go:build mtile_mode0 && mtile_mode0_obj`)
- Create: `internal/transpiler/mode0_pspr.go`, `mode0_pspr_test.go`
- Modify: `internal/assetsync/{types.go,assets.go,generate.go,poses.go}`
- Modify: `runtime/{cartmem.go,context.go,pose.go,pose_test.go}`
- Modify: `internal/transpiler/{transpiler.go,render_modes.go,render_modes_test.go,renderer_selection.go,renderer_selection_test.go,mode0_draw_walk.go,mode0_object_budget.go}`
- Modify: `internal/build/build.go`
- Modify: `internal/editor/model.go` plus editor tests
- Modify: `cmd/editor/{sprite_editor.go,sprite_tools.go,game.go}` plus tests
- Modify: `docs/render-modes.md`, `docs/graphics-model.md`, `docs/reference.md`
- Modify last: `examples/nerdo` assets + `nerdo_fixture_test.go`

## Tasks

### Task 1: Object sheet load/save + pose flatten against the active bank

Constants: `MaxObjSheetCells = MapStreamingThresholdCells`, `MaxObjSheetPx = 8 * 128`, `DefaultObjSheetW = 240 * 2`, `DefaultObjSheetH = 160 * 2`.

`LoadObjSheet`: missing file returns `(nil, nil)`. Invalid JSON/dims fail with `objsheet.json` in the message. `flattenPoses` takes `(poses, bankN, bankName)` and names the bank on tile-range errors.

### Task 2: Cart object-sheet ROM + Mode 4 `PSpr` bank switch

`Assets`/`AssetImage`/`AssetMemory`/`Options` grow `ObjSheetW/H`, `ObjSpriteCols/Rows/N`, `ObjSheet`, `ObjSheetIndex`. Object sheet is always compact ROM (no EWRAM pixel copy). `PSpr` blits object cells when `ObjSpriteN > 0`, else existing `Spr`.

### Task 3: Sprite Lab BG | OBJ

Model holds optional `ObjSheet`. Toggle paints the active backing store. Create writes default/asked size. Object tile 0 is paintable; print locks 480-511 do not apply to object IDs. Flags stay on the BG sheet.

### Task 4: Mode 0 proof + reclassify

`pspr` -> `RendererMode0Conditional`. New `M0_PSPR_SHAPE`. Finite pose IDs like `spr`. Mix with `spr`/`sspr` fails. More than two sites or window `> 448` is `M0_RESOURCE_LIMIT`. Spritesheet bank + reachable `sset` is `M0_MUTABLE_SHEET`. Object budget counts pose parts as 8x8 `spr` 1x1. Exhaustive classification counts: Mode 4 only 8->7, Mode 0 conditional 8->9.

### Task 5: Mode 0 stream runtime, parity, bench

Per-site windows in OBJ 0-447. Vblank DMA unique tiles from compact pose-bank ROM. OAM attr2 uses slot, not logical tile. Pixel-parity Mode 0 vs Mode 4 for nerdo idle and `flipX`. DMA/frame bench for one pose change.

### Task 6: nerdo fixture + docs

Move four idle tiles onto `objsheet.json`, remap poses. Update `render-modes.md`, `graphics-model.md` Stage 8 note.

### Task 7: AGENTS.md gate

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```
