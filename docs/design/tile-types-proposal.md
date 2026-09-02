# Semantic tile types, scene layers, and background animation

Status: long-term product proposal; deferred and not yet implemented. The
active implementation order is the
[graphics evolution roadmap](../graphics-model.md#graphics-evolution-roadmap).
Do not treat the delivery slices below as the next engineering milestones.

![Sprite Lab concept](tile-types-sprite-lab-concept.png)

![Map Lab concept](tile-types-map-lab-concept.png)

![Scene Layers and parallax concept](scene-layers-parallax-concept-v2.png)

## Goal

Let an author work with named concepts such as `Grass Top`, `Coin Block`,
`Mountains`, and `Before Foreground` instead of coordinating raw sprite IDs,
flag masks, and hardware priorities in Lua. Fixed, synchronized animation must
remain a background concern; independently positioned actors remain objects.

The feature should preserve the current PICO-8-style logical sheet and the
exact semantics of existing projects. No metadata file means no behavior
change.

## Current renderer reality

Mode 0 already derives one hardware background from each direct `map()` call.
One to four calls become BG0 through BG3 in call order. The first call is the
back layer and the last call is the front layer. With four calls, the current
allocation is:

| Authoring order | Hardware layer | BG priority |
| --- | --- | --- |
| first / back | BG0 | 3 |
| second | BG1 | 2 |
| third | BG2 | 1 |
| fourth / front | BG3 | 0 |

Each call captures the `camera()` state that exists when it runs, so raw
parallax is technically possible today by changing the camera between map
calls. Fine movement becomes a BG scroll-register update. This capability is
not represented in the Map Lab and the last `map()` argument remains a sprite
flag mask, not a background-layer ID.

The immediate-mode compatibility path now supports object depth: a direct
`spr()`/`sspr()` placed between explicit `map()` calls derives the exact OBJ
priority from painter order. What remains missing from this long-term proposal
is a named, editor-visible lane model and runtime-variable depth that does not
require restructuring draw statements. A complete semantic design must expose
both the existing multi-BG allocation and those object slots in authoring UI.

## Authoring model

The editor adds three concepts above the existing spritesheet:

1. **Animation clip**: an ordered list of arbitrary sheet cells and durations.
2. **Tile type**: a stable name, renderer role, collision/tags, and one or more
   visual states.
3. **Tile state**: a named visual such as `active` or `used`, optionally linked
   to an animation clip.

The Sprite Lab owns definitions. The Map Lab paints definitions and inspects
instances. Raw IDs remain available in a secondary compatibility view.

The Map Lab adds a fourth concept above the existing single cell grid:

4. **Scene layer**: one named map plane with its own cells, depth, parallax,
   repeat policy, visibility, and collision participation.

Example optional asset file:

```json
{
  "version": 1,
  "animations": [
    {
      "name": "grass_sway",
      "target": "bg",
      "loop": true,
      "frames": [
        {"sprite": 41, "ticks": 8},
        {"sprite": 48, "ticks": 8},
        {"sprite": 52, "ticks": 8},
        {"sprite": 48, "ticks": 8}
      ]
    },
    {
      "name": "coin_flash",
      "target": "bg",
      "loop": true,
      "frames": [
        {"sprite": 60, "ticks": 6},
        {"sprite": 61, "ticks": 6}
      ]
    }
  ],
  "types": [
    {
      "name": "grass_top",
      "displayName": "Grass Top",
      "role": "bg",
      "tags": ["grass", "ground"],
      "collision": "solid",
      "states": {
        "default": {"anchorSprite": 40, "animation": "grass_sway"}
      }
    },
    {
      "name": "coin_block",
      "displayName": "Coin Block",
      "role": "bg",
      "tags": ["solid", "bumpable", "coin"],
      "collision": "solid",
      "initialState": "active",
      "states": {
        "active": {"anchorSprite": 50, "animation": "coin_flash"},
        "used": {"anchorSprite": 51}
      }
    }
  ]
}
```

`anchorSprite` is the compatibility bridge. The map continues to store a
normal sprite ID, `mget()` continues to return one, and `mset()` continues to
write one. The registry gives the editor and build a reverse mapping from an
anchor ID to a named type and state.

For the first implementation, changing a state is still an ordinary map edit:

```lua
-- Generated symbolic constants replace handwritten numeric IDs.
mset(tx, ty, tile_coin_block_used)
```

This provides the authoring improvement without adding or changing a public
Lua builtin.

## Scene-layer model

The editor stores layers back-to-front using stable semantic IDs. It does not
persist BG0-BG3 as author-owned truth; the build derives the hardware
allocation from order and resource pressure. This keeps a map meaningful in
Mode 4 and lets the Mode 0 allocator reject or repack it without rewriting the
asset.

An abbreviated versioned map asset could be:

```json
{
  "version": 2,
  "width": 120,
  "height": 20,
  "layers": [
    {
      "id": "sky",
      "displayName": "Sky",
      "cells": [0, 0, 0],
      "parallax": {"x": {"num": 0, "den": 1}, "y": {"num": 0, "den": 1}},
      "repeatX": true,
      "collision": false
    },
    {
      "id": "mountains",
      "displayName": "Mountains",
      "cells": [0, 71, 72],
      "parallax": {"x": {"num": 7, "den": 20}, "y": {"num": 3, "den": 20}},
      "repeatX": true,
      "collision": false
    },
    {
      "id": "world",
      "displayName": "World",
      "cells": [0, 40, 41],
      "parallax": {"x": {"num": 1, "den": 1}, "y": {"num": 1, "den": 1}},
      "collision": true
    },
    {
      "id": "foreground",
      "displayName": "Foreground",
      "cells": [0, 81, 82],
      "parallax": {"x": {"num": 1, "den": 1}, "y": {"num": 1, "den": 1}},
      "collision": false
    }
  ]
}
```

The short `cells` arrays above are illustrative; persisted planes contain
`width * height` entries. Existing unversioned `map.json` files load as one
`world` layer and save unchanged until the author explicitly adds a layer.

Parallax uses integer rational math in both renderers:

```text
layer_scroll = layer_offset + floor(camera * numerator / denominator)
```

The signed rounding rule is part of the format. Mode 0 writes the result to the
layer's HOFS/VOFS registers while retaining the unwrapped logical value for map
streaming; Mode 4 samples the same logical plane with the same integer result.
The editor may display friendly values such as `0.35x`, but it stores `7/20`
so the two renderers cannot drift through floating-point rounding.

Only selected layers participate in collision. A normal platformer uses
`World` for collision and treats `Sky`, `Mountains`, and `Foreground` as visual
planes even when their tiles have semantic tags.

## Object depth lanes

GBA OBJ priority has four values. If an OBJ and a BG have the same priority,
the OBJ is in front of that BG. Four ordered backgrounds therefore provide
four useful, exactly representable object lanes:

| Semantic object depth | OBJ priority | Result with the four-layer example |
| --- | --- | --- |
| `front` | 0 | in front of Foreground and every other BG |
| `before_foreground` | 1 | behind Foreground, in front of World |
| `before_world` | 2 | behind World, in front of Mountains |
| `before_mountains` | 3 | behind Mountains, in front of Sky |

The current immediate API selects these lanes by placing an object after the
corresponding explicit map pass; it exposes no raw priority number and rejects
the unavailable behind-all lane. A future editor would present semantic names,
generate stable constants such as `depth_before_foreground`, and report the
derived P0-P3 mapping. A fifth “behind Sky” lane is not offered in Mode 0
because the hardware cannot represent it exactly.

The eventual runtime-facing abstraction should be a bounded **scene
compositor**, rather than an eighth argument bolted onto raw `map()`. One
possible authoring shape is:

```lua
scene_begin(scene_overworld, camera_x, camera_y)
scene_spr(player_tile, player_x, player_y, depth_before_foreground)
scene_end()
```

Changing one variable can then move the hero at runtime without converting a
background plane to OAM:

```lua
if entering_door then
  player_depth = depth_before_foreground
else
  player_depth = depth_front
end
```

In Mode 4, `scene_end()` composites the planes and queued objects in the exact
GBA order. In Mode 0, it assigns BG priorities, writes independent scroll
registers, and puts the semantic depth into OBJ Attribute 2. Existing
`map()`/`spr()` remain the immediate-mode compatibility path; the scene API is
opt-in and must not silently change their ordering semantics.

The API names above are design placeholders, not implemented builtins. If
accepted, they require the complete public-API renderer contract, including
registry, renderer classification, documentation, parser/transpiler/runtime,
selection, pixel-parity, and benchmark coverage.

## Renderer behavior

### Mode 4

Before blitting a map cell, `map()` resolves an animated anchor to the current
frame's source sprite. The logical map is not rewritten every animation tick.
`mget()` therefore remains stable while the visual frame changes.

### Mode 0

Each synchronized BG animation owns one resident destination tile, normally
its anchor ID. Frame pixels stay in ROM. At a frame boundary the runtime copies
64 bytes per changed 8bpp tile into that BG VRAM slot during the existing
presentation commit.

All map entries continue to reference the same anchor tile. Consequently every
`Grass Top` instance animates together without scanning the map, rewriting map
cells, or consuming OAM.

OBJ animation is deliberately outside the first slice. Moving coins, actors,
and temporarily bumping blocks continue to use `spr()` and the existing object
proof.

The Mode 0 analyzer must include the worst-case animation upload in its VBlank
budget. Forced Mode 0 must reject an unrepresentable animation; AUTO must fall
back to Mode 4 with a stable, source- or asset-positioned diagnostic.

Scene-layer scrolling adds a second cost proof. A pixel-only camera change is
normally a cheap register write, but each layer crosses 8-pixel tile
boundaries at different parallax rates. The analyzer and benchmarks must cover
the worst frame where several streamed planes need map updates together.

Each 64x32 background consumes two screen blocks. Consequently one, two,
three, and four layers retain the existing limits of 960, 896, 832, and 768
8bpp BG tile slots respectively. The editor must show both `BG layers used`
and `BG tile slots remaining`; parallax does not create extra layers for free.

## Coin-block flow

An active block's map cell references the `active` anchor, whose visual is the
shared `Coin Flash` BG animation. On a hit, game logic performs one state
transition:

1. replace that cell with the `used` anchor;
2. increment the coin counter and play the sound;
3. spawn the rising coin as an OBJ.

Other active blocks keep referencing the animated anchor and continue flashing.
The hardware tilemap stores only visual tile indices; semantic tags and behavior
remain compiler/runtime data.

## Editor surfaces

### Sprite Lab

- Multi-select arbitrary sheet cells and create an animation clip.
- Timeline edits frame order, duration, looping, and preview.
- Tile Type inspector owns name, role, named tags, collision, states, and clip.
- A Mode 0 footprint line shows resident slots, ROM frames, and bytes per
  animation boundary.

### Map Lab

- `Scene Layers` and `Tile Types` are sibling views: layers choose where cells
  live; types choose what is painted into the selected layer.
- The layer stack is ordered front-to-back in the UI, with visibility, lock,
  reorder, duplicate, parallax, repeat, and collision controls.
- BG0-BG3 and P0-P3 appear as derived Mode 0 badges and in a compositor
  preview, not as the primary author vocabulary.
- A live parallax preview scrubs the main camera and makes each layer's
  relative motion visible without running Lua.
- Selecting an object-depth lane previews actors behind or in front of the
  adjacent layer and exposes the generated symbolic depth constant.
- `Types` is the default paint palette; `Raw Tiles` preserves expert access.
- Tiles appear by semantic name with compact `ANIM`, `SOLID`, and `BUMP` badges.
- Selecting a cell reveals type, state, visual, collision, and configured
  transition.
- Terrain/metatile brushes can be added later without changing the animation
  contract.

## Compatibility and migration

- Existing `project.json`, `spritesheet.json`, and `map.json` remain valid.
- An existing map is one implicit `world` plane; adding a second plane is the
  explicit migration point and must remain reversible until the first v2 save.
- The semantic file is optional and versioned.
- Raw editing remains available and always shows the underlying anchor ID.
- Animation frames retain normal logical sheet coordinates for `sget()` and
  `sset()`; the visible VRAM copy remains a renderer cache.
- The first slice should not add a public Lua API. A later `tile_state()` or
  behavior callback would be a public API change and must update the builtin
  registry, renderer classification, render-mode documentation, parser,
  transpiler, runtime, renderer-selection, parity, and benchmark coverage.

## Long-term delivery slices

These slices begin only after the active graphics roadmap reaches its
multi-plane and asset-allocation hold point and the product explicitly chooses
to proceed with engine-level semantics.

1. **Metadata and editor preview**: tile-type schema plus a versioned multi-plane
   map model, validation, reversible loading, layer stack, parallax scrubber,
   compositor preview, and no runtime behavior change.
2. **Mode 4 scene compositor**: integer parallax, ordered planes, semantic
   object-depth lanes, and exact image fixtures.
3. **Mode 0 scene compositor**: BG allocation, independent register scroll,
   OBJ Attribute 2 depth, streaming pressure proof, parity fixtures, and AUTO
   cost evidence.
4. **Background animation**: Mode 4 anchor resolution and Mode 0 one-slot VRAM
   updates integrated with every scene plane.
5. **State workflow**: generated symbolic constants, selected-cell state
   inspector, coin-block example, and documentation.
6. **Metatiles and terrain brushes**: grouped 16x16+ visuals and autotiling,
   built on the same type, animation, and layer registry.

## Acceptance conditions for the first complete vertical slice

- A `Grass Top` animation can use non-contiguous source sprite IDs.
- Painting it in multiple map cells requires no Lua list of IDs or positions.
- Mode 0 changes one BG resident slot per frame boundary and uses no OAM.
- Mode 4 and Mode 0 produce pixel-identical accepted frames.
- A `Coin Block` can transition one cell from active to used while every other
  active block keeps animating.
- A project with no semantic tile metadata builds byte-for-byte through the
  existing asset and renderer paths, apart from unavoidable toolchain metadata.
- A four-plane scene previews and renders the same back-to-front order in both
  renderers, with independently computed integer parallax.
- Changing `player_depth` from `front` to `before_foreground` moves the same OBJ
  behind only the Foreground plane and does not change its tile allocation.
- A forced Mode 0 build rejects a fifth BG plane, an object depth behind the
  backmost BG, or a scroll/update peak that cannot be proven within budget.
