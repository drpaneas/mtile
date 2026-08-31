# Graphics model

This document records Mtile's logical-background semantics. Version-0 map
assets remain supported as BG0; version 2 adds four independently authored
planes. The Lua API deliberately changed so the first argument of every
non-empty `map(...)` call is always the logical BG number.

## Logical assets

One project currently owns:

- one 8bpp spritesheet addressed as complete 8x8 cells;
- one logical map asset, either one legacy plane or four versioned BG planes,
  whose cells contain spritesheet IDs;
- one eight-bit flag value per spritesheet cell; and
- one logical palette with 16 or 256 authorable colors, expanded to 256
  runtime entries.

The four version-2 planes share dimensions but own independent cell arrays.
BG0 is the back plane and BG3 is the front plane. The Map Editor composites
visible planes in that order, paints only the selected plane, and provides a
session-only eye control for each plane. Opening and saving a legacy project
does not migrate it; the first edit to BG1-BG3 expands it, preserving BG0
byte-for-byte and initializing the other planes as transparent.

The spritesheet is also logical rather than hardware-specific. `map()` reads a
cell as background art and `spr()`/`sspr()` read a cell as object art. A cell
may be used by both. Mode 0 then stages the required pixels into the GBA's
separate BG and OBJ VRAM regions; Mode 4 software-composites both operations
into one framebuffer.

## `map()` contract

There are two forms:

```lua
map()
map(bg, cell_x, cell_y, screen_x, screen_y, cells_w, cells_h, mask)
```

`map()` draws every authored non-empty plane in BG0-to-BG3 order. In the
second form `bg` is always the first argument and must be a static integer
literal from 0 through 3. Every argument after `bg` is optional and keeps its
existing default, so `map(2)`, `map(2, 10, 4)`, and the complete eight-argument
call all select BG2. An old positional call migrates by adding the desired BG
at the front: `map(cell_x,cell_y,...)` becomes
`map(0,cell_x,cell_y,...)` for BG0.

`mask` is a sprite-flag mask, **not a hardware background number**. Zero draws
every non-empty map cell. A nonzero mask draws a cell only when its spritesheet
flags contain every requested bit.

The source cell `(cell_x + mx, cell_y + my)` is placed at world position:

```text
(screen_x + (cell_x + mx) * 8,
 screen_y + (cell_y + my) * 8)
```

The current camera is subtracted from that world position. Consequently a
source rectangle beginning at cell X 30 can be overlaid at screen/world X 0 by
passing `screen_x = -240`.

The selected plane uses BG0 as the back and BG3 as the front. A program may
either use one `map()` automatic call or unique explicit plane calls; it cannot
mix them. Explicit calls are unconditional direct `_draw` statements in
ascending BG0-to-BG3 order. Empty planes are transparent and omitted from the
build allocation. Violations fail every renderer build with the stable,
source-positioned `MAP_EXPLICIT_CONTRACT` diagnostic. This common contract is
what lets Mode 4 painter order and Mode 0 hardware priority remain identical
without four software framebuffers.

In Mode 4 every valid call is an immediate software draw in the required
back-to-front order. In Mode 0 logical planes are packed into consecutive
hardware slots while preserving their order:

| Dense plane position | Hardware BG | BG priority | Visual order |
| --- | --- | --- | --- |
| first | BG0 | `N-1` | back |
| second | BG1 | `N-2` | in front of the first |
| third | BG2 | `N-3` | in front of the previous calls |
| fourth | BG3 | `0` | front map pass |

Here `N` is the number of non-empty planes selected for drawing. Hardware BG
numbers are an implementation allocation, not the logical ID exposed to Lua.
For example, authored BG1 and BG3 retain those source identities but occupy
hardware BG0 and BG1, so they reserve two screen shadows rather than four.

The generated map payload uses the same dense idea. Only non-empty authored
planes are emitted to ROM/EWRAM, with a logical-plane mask translating BG IDs
to storage slots. `mset()` keeps BG0 allocated even if it starts empty because
runtime mutation can make it visible.

Each Mode 0 pass owns a 64x32 regular 8bpp hardware background window and two
screen blocks. Zero, one, two, three, and four passes therefore allow at most
1024, 960, 896, 832, and 768 declared spritesheet cells respectively. A
zero-pass scene presents only the backdrop and any admitted OBJ/text calls.

## Current parallax

Each map pass observes the camera active at its own call site. Different
cameras therefore already produce independent parallax:

```lua
function _draw()
  cls(225)

  camera(flr(cam_x / 4), 0)
  map(0, 0, 0, 0, 0, 10, 20, 0)

  camera(flr(cam_x / 2), 0)
  map(1, 10, 0, -80, 0, 10, 20, 0)

  camera(cam_x, cam_y)
  map(2, 20, 0, -160, 0, 10, 20, 0)

  -- Objects use whichever camera is active when they are drawn.
  spr(100, player_x, player_y)
  camera(0, 0)
end
```

Mode 4 applies those offsets during each software draw. Mode 0 stores a
separate scroll value for each hardware BG. Sub-tile movement normally updates
only scroll registers; crossing an eight-pixel boundary may refresh the
corresponding streamed tile window.

## Current mutations

The logical assets remain authoritative in both renderers:

- `mset(x, y, id)` and `mget(x,y)` retain their legacy BG0 meaning. Explicit
  mutation of BG1-BG3 is intentionally not added in this stage; those planes
  are static at runtime until a separately specified layer-aware mutation API
  is accepted.
- `sset(x, y, color)` changes one spritesheet pixel. If that cell is used by
  both a map and an object, both Mode 0 VRAM copies are synchronized.
- `fset(id, bit, value)` changes the flags for a tile ID, potentially changing
  every placement of that ID in every masked map pass.

A stateful block normally changes its own map cell:

```lua
if block_hit then
  mset(block_x, block_y, used_block_tile)
end
```

This does not require an entity or animation system. Shared animation of every
placement of one tile is less direct today: 64 `sset()` calls would replace an
8x8 frame, while the Mode 0 update proof currently permits only 16 calls.

## Current object order

The accepted Mode 0 subset requires every `spr()`, supported `sspr()`, and
`print()` after every map call. Hardware objects are emitted with OBJ priority
0, so they appear in front of all accepted backgrounds. Objects cannot
currently be placed between two map passes in Mode 0.

Mode 4 can software-draw arbitrary painter sequences, but a sequence outside
the proven subset makes AUTO fall back to Mode 4 and makes a forced Mode 0
build fail with a source-positioned diagnostic.

## Editor representation

The Sprite Editor exposes the shared sheet and eight generic flag bits. The
Map Editor exposes a fixed horizontal BG0-to-BG3 toolbar below the canvas,
per-layer visibility, active-layer selection, composite preview, and isolated
painting/erasing. It does not yet show:

- the camera captured by each pass;
- BG/OBJ VRAM residency; or
- object priority relative to backgrounds.

Those are derived from Lua and the selected renderer during the build.

## Graphics evolution roadmap

This is the canonical implementation order for evolving the graphics model.
It supersedes earlier informal lists that omitted the explicit-background API
or placed engine-level metadata before the smaller compatibility extensions.

Status meanings:

- **Done** means the behavior, characterization evidence, and documentation are
  present in this repository.
- **Partial** means a useful slice exists but the stated acceptance condition
  is not complete.
- **Pending** means no public behavior should be inferred from the proposal.
- **Absorbed** means a later stage was intentionally delivered as part of an
  earlier product slice and no separate migration remains.
- **Deferred** means do not start the work until its dependencies and a new
  product decision are complete.

| Stage | Status | Deliverable | Difficulty | Depends on |
| --- | --- | --- | --- | --- |
| 0 | Done | Characterization tests and exact documentation of the current single-map, shared-sheet, implicit-BG model. | Small | Runtime tests, renderer-selection tests, parity workloads, docs. |
| 1 | Done | Current-API examples with four BG passes, parallax, and a stateful animated coin block. | Small | Stage 0; examples and docs only. |
| 2 | Done | Unambiguous `map()`/`map(bg, ...)`, sparse build-time plane packing, versioned four-plane assets, and selectable/hideable BG0-BG3 Map Editor layers. | Large | Stage 0; schema expansion, editor/save/undo, public API migration, both runtimes, analyzer, parity, benchmarks. |
| 3 | Pending | `tcopy(dst_tile, src_tile)` for bounded whole-cell background animation without 64 handwritten `sset()` calls. | Medium | Stage 0; sheet mutability analysis and the complete public API contract. |
| 4 | Pending | Dynamic `spr(id, ...)` for a statically proven finite ID range that fits OBJ VRAM and the object/text budgets. | Small–medium | Stage 0; analyzer widening, diagnostics, parity, benchmarks. |
| 5 | Pending | `spr()`/`sspr()` between logical backgrounds with automatically derived OAM priority and identical Mode 4 painter order. | Medium–large | Stage 2; analyzer, Mode 0 OBJ runtime, parity, benchmarks. |
| 6 | Pending | Read-only editor preview of legacy and explicit map passes, captured cameras, derived BG slots, and object-depth lanes. | Medium | Stage 2 and static analysis exposed to the editor. |
| 7 | Absorbed | The multi-plane schema/editor portion was pulled into Stage 2 by product decision; derived camera/pass preview remains Stage 6. | — | Stage 2. |
| 8 | Pending | Separate logical BG/OBJ asset banks, 4bpp palette allocation, and native OBJ packing. | Very large | Stage 2 and a measured asset-allocation design. |
| 9 | Deferred | Tile types, named states, events, entity components, terrain brushes, and higher-level scene behavior. | Engine-level | Stage 8 plus a separate product decision. |

The dependency shape is intentionally not a single chain:

```text
Stage 0: characterize current semantics
├── Stage 1: current-API examples (done)
├── Stage 3: tcopy
├── Stage 4: bounded dynamic spr
└── Stage 2: explicit map(bg) + four-plane authoring (done)
    ├── Stage 5: object depth between backgrounds
    └── Stage 6: read-only runtime pass/camera preview
        └── Stage 8: separate BG/OBJ banks
            └── Stage 9: semantic engine features (deferred)
```

### Stage 0 evidence

The baseline includes:

- this document and [Render-mode compatibility](render-modes.md);
- host tests for independent two- and four-background state;
- checked-in `layers4`, `parallax_layers`, `map_arities_short`,
  `map_arities_long`, `map_mutation_isolation`, and `object_order_current`
  characterization workloads; and
- the [`background-layers`](../examples/background-layers/README.md) project,
  which supplies four editor-visible authored planes and draws them with four
  independent parallax cameras.

### Stage 1 evidence

The `parallax_layers` benchmark source over the background-layers assets proves
four backgrounds and independent cameras. The focused
[`coin-block`](../examples/coin-block/README.md) companion completes the
stateful-background slice: one direct `mset()` alternates one cell between two
active IDs, A (or its deterministic teaching demo) bumps the block, one coin is
awarded, and that same cell becomes a stable used tile. It adds no metadata,
new Lua builtin, or second map plane.

The paired 120-frame full gate observed the active animation, transition, and
used state with zero differing pixels and zero missed frames. Mode 0 reclaimed
30,952 EWRAM bytes and reduced median work by 66.36%, while paying a one-time
3,273,893-cycle cold upload. The exact evidence is recorded in
[`coin-block-v1.json`](benchmarks/render-modes/coin-block-v1.json).

### Stage 2 compatibility boundary

`map()` means all authored non-empty planes. Every call with arguments begins
with a logical background ID, followed by the previous optional arguments:

```lua
map(bg, cell_x, cell_y, screen_x, screen_y, cells_w, cells_h, mask)
```

`bg` is a logical back-to-front identity, not a physical register exposed for
arbitrary manipulation. Mode 0 densely assigns selected non-empty planes to
hardware BGs and Mode 4 draws the same authored planes in the shared required
order. Duplicate slots, mixed automatic/explicit calls, dynamic IDs,
conditional calls, and reversed order fail all builds with
`MAP_EXPLICIT_CONTRACT`; they cannot fall back because they are invalid common
API programs rather than renderer limitations. Missing/empty planes consume no
map payload or hardware screen shadow. The version-2 schema, editor/save/undo
path, generated compact ROM layout, both runtimes, analyzer, and example are
covered by Stage 2 tests.

The checked-in
[`background-layers-map-contract-v2.json`](benchmarks/render-modes/background-layers-map-contract-v2.json)
records 120 moving parallax frames with zero differing pixels after the API
migration. Compared with Mode 4, Mode 0 reduces median work by 83.09% and
returns 21,652 EWRAM bytes; its one-time cold upload costs 1,903,027 cycles and
records three misses. The parity gate passes. The generic full gate does not,
because four intentional hardware-screen shadows remain resident and cold
scene upload cost is real; that trade-off is documented rather than hidden.
The two map-arity workloads additionally exercise all accepted one-through-eight
argument forms in paired Mode 4/Mode 0 CI captures. Argument count changes only
which established defaults reach the same runtime map primitive, so it does not
define a separate performance profile.

[`map-plane-packing-host-v1.json`](benchmarks/render-modes/map-plane-packing-host-v1.json)
isolates sparse allocation: BG1+BG3 take 82,248 ns/op versus 162,213 ns/op for
four planes on the recorded host, with zero allocations. A 60x20 two-of-four
map saves 4,800 map bytes in ROM/EWRAM plus two 4 KiB Mode 0 screen shadows.

### Stage 3 compatibility boundary

`tcopy(dst_tile, src_tile)` copies one complete logical 8x8 source cell into a
complete destination cell. It copies pixels, not flags or map placements, and
the source is snapshotted before the destination changes. Both renderers update
their BG and OBJ resident copies before the next draw which observes the
destination. Merely aliasing VRAM tiles is not equivalent because `sget()` and
later `sset()` calls must observe the copied logical pixels.

Adding `tcopy` is a public Lua API change. It therefore requires the builtin
registry, renderer classification, render-mode documentation, parser,
transpiler, runtime, renderer-selection, pixel-parity, and benchmark coverage
in the same change.

### Stages 4–6 widening rules

- Dynamic `spr()` begins with a one-cell source and a statically provable
  finite ID set/range. Every possible ID must fit the sheet, OBJ tile range,
  optional text reservation, and OAM/scanline budgets. An arbitrary runtime ID
  remains a Mode 4 fallback.
- Object depth is defined relative to Stage 2 logical BG slots. It is not
  inferred from a physical register number exposed to Lua. Mode 0 uses OAM
  Attribute 2 priority; Mode 4 must retain the same visible order.
- Stage 6 adds read-only runtime pass/camera inspection on top of the authored
  Stage 2 planes; it does not add scene metadata or a second authoring model.

### Stages 8–9 hold point

Do not begin the asset-bank split or semantic tile/entity system while Stages
3–6 are unsettled. They multiply allocator and renderer work and would turn
the remaining background-tile problems into an engine architecture commitment.

The long-term semantic design remains recorded in
[Semantic tile types, scene layers, and background animation](design/tile-types-proposal.md),
but it is not the active implementation plan.
