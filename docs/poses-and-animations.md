# When to use sprites, poses, and animations

[Documentation index](README.md) · [Editor walkthrough](ide.md#poses-and-animations) · [Exact API reference](reference.md#pose-sprites-and-animation-storage)

**For a character or reusable object with its own artwork, start with poses.**
Draw a still pose with `pspr`; draw a timed sequence of poses with `anim`.
Keep `spr` for shared-sheet cells and simple rectangles, especially artwork
also used by the map. Start with [Moth Study](../examples/moth-study/README.md):
one still pose, then a looping clip, then input and facing, with ready artwork
and a complete program at each step. The sections below answer the next
question you encounter; the existing Nerdo exercise is another quick option.

## Try it with Nerdo

This example already contains the artwork and registered animations. You can
see the difference before creating any assets yourself.

From the repository root, open it:

```sh
go run ./cmd/mtile edit examples/nerdo
```

1. In **Develop**, select **Build + Reload** and click the game preview.
   Hold left/right, then release. The character changes its walking/idle
   animation and facing; it stays in the same place in this example.
2. In **Sprite Lab → Animations**, select **idle**. Click the FPS **+** button
   four times to change **6 → 10**, then use **PLAY** to preview it.
3. Click **SAVE CLIP**, return to **Develop**, and select **Build + Reload**.

**Checkpoint:** the idle animation plays faster, with no Lua changes.
You have changed an animation through its saved clip. You can stop here.

<details markdown="1">
<summary>One more small experiment: draw a still pose</summary>

Nerdo's pose **0** is its first idle picture. Replace only the idle draw call
in `game.lua`:

```lua
pspr(0, 100, 60, facing_left)
```

Save and rebuild. **Checkpoint:** while the arrows are released, the character
holds one picture. Holding an arrow still plays `walk` because that branch
still calls `anim`. Restore `anim("idle", 100, 60, facing_left)` afterward.

That is the distinction: `pspr` draws one saved picture; `anim` plays a saved
sequence. A fresh project needs its own registered assets before these calls
will work; Nerdo already includes them.

</details>

When you want to make your own artwork, use the short choice table below and
follow the registration steps. Independent action timers and hardware budgets
can wait until you need them.

## Choose by what you are making

| What you need | Choose | Why |
| --- | --- | --- |
| Ground, walls, or other repeated scenery | Shared tiles + `map` | Maps place shared-sheet cells; pose registrations are not map cells. |
| One coin/icon tile, or a simple rectangle already on the shared sheet | `spr` | A direct cell/rectangle draw is enough; no pose registration is needed. |
| A still character, lantern, sign, or interface panel assembled from artwork | Register a pose + `pspr` | One draw uses the saved tile layout, offsets, and part flips. Gameplay does not assemble it tile by tile. |
| Idle, walking, flying, flickering, or another repeating visual sequence | Register poses + a looping clip + `anim` | Frame order, FPS, and looping are saved and previewed in the editor. |

Maps, `spr`, `pspr`, and `anim` can coexist in one game. A character does not
need a clip if it has only one visual state, and a simple shared-sheet coin
does not need a pose.

## A pose is one picture; a clip is a sequence

A **pose** is one complete visual state: closed wings, an open door, or a
finished panel. It can contain many 8×8 tiles, with saved positions and flips.
It is not limited to one tile or a rectangular arrangement.

An **animation clip** is an ordered list of poses, with a name, FPS, and loop
setting. `anim` selects and draws **one pose** at the requested time. It does
not draw the whole clip at once or create a character/entity.

The usual asset workflow is:

1. Paint terrain in **Sprite Lab → Shared tiles** and place it in **Map Lab**.
2. Paint character/object artwork in **Poses**, select a complete visual state,
   and click **REGISTER POSE**. Repeat for the other states.
3. For moving artwork, open **Animations**, create a named clip, add the poses,
   set FPS and LOOP, preview it, and click **REGISTER CLIP**.
4. Save and rebuild. Call `pspr` with a numeric pose ID or `anim` with the
   registered clip name. The name must exist before building the example.

`pspr` takes a **zero-based numeric ID**, not the pose's editor name. Store IDs
in constants when useful: `LANTERN_UNLIT = 0`, then
`pspr(LANTERN_UNLIT, x, y)`. Deleting a pose shifts later IDs, so update those
constants. Clip frame order can change without changing the Lua clip name.

## Why use a clip instead of selecting frames yourself?

Manual frame selection couples gameplay to the sheet layout. For example,
this draw fragment assumes four 16×16 frames start at shared-sheet IDs 32,
34, 36, and 38:

```lua
local frame = flr(time()*8)%4
spr(32+frame*2, x, y, 2, 2, facing_left)
```

Register those visual states as poses and save an eight-FPS looping clip named
`walk`; the draw fragment becomes:

```lua
anim("walk", x, y, facing_left)
```

Now the editor owns the frame list and playback speed. You can reorder frames,
repeat one to hold it longer, change FPS, and preview the result without
rewriting atlas strides or frame-selection arithmetic. A pose also keeps its
tile arrangement out of gameplay code. Use the editor's supported artwork
move operation to preserve registered references when reorganizing OBJ pages.

Flipping a clip uses one fixed rectangle enclosing all its poses, so a narrower
frame does not move the mirror axis. Do not add your own width adjustment.
This preserves authored offsets and tile padding; you still need to align the
character's feet or center consistently while authoring the frames.

Use manual `pspr` selection when **game state selects a still picture**, such
as an unlit versus lit icon. Use `anim` when **elapsed time selects a sequence**.
Keep movement, collision, and other gameplay state in Lua in either case.

## Ambient loops and actions need different clocks

For a repeating idle or walk, the default clock used in Moth Study and Nerdo
is enough:

```lua
anim("idle", x, y)
```

When an action must start from its first frame, try
[Moon Door](../examples/moon-door/README.md). Its first lesson opens a ready
gate and holds the ending; pause is a separate optional lesson. The clock
details below are a reference for your own game.

<details markdown="1">
<summary>Start, restart, and pause a nonlooping action</summary>

**Changing the clip name does not restart playback.** `anim` has no stored
playhead per character. Two calls using the same clip and default clock use
the same frame. For an action that must start at frame zero, supply seconds
since that action began as the sixth argument.

Here is a complete timing example. First register pose 0 as a closed door and
a four-pose `open` clip at eight FPS with LOOP off. Tap A to start/restart
opening; tap Start to pause it:

```lua
opening = false
opening_ticks = 0
paused = false

function _update()
  if btnp(6) then paused = not paused end
  if not paused then
    if btnp(5) then
      opening = true
      opening_ticks = 0
    elseif opening then
      opening_ticks = min(opening_ticks+1, 30)
    end
  end
end

function _draw()
  cls()
  if opening then
    anim("open", 100, 60, false, false, opening_ticks/30)
  else
    pspr(0, 100, 60)
  end
end
```

The generated game updates at about 30 Hz, so `opening_ticks/30` supplies
elapsed seconds. This bounded counter stops while paused; LOOP off keeps the
final pose visible afterward. A separate counter per character gives each an
independent action start. Holding A or Start triggers `btnp`'s normal input
repeat; use your own button-edge handling if an action must fire only once
per press.

Returning early from `_update` does **not** pause the engine's shared clock.
Use a gameplay-owned elapsed counter when animations must freeze with the game.
If pausing is irrelevant, `time()-started` is also a convenient action timer.
`anim` does not report completion or decide when a door becomes passable;
your gameplay code owns those decisions.

</details>

## What do poses and clips buy you in Mode 0?

Come back to this section if a build diagnostic mentions hardware resources,
or you want to understand the compiler's allocation choices.

<details markdown="1">
<summary>Optional: packing, memory, and renderer limits</summary>

Their main benefit is an editable artwork/playback model. They also give the
compiler complete pose recipes and clip frame sets to analyze:

- Eligible pose rectangles can pack into larger hardware objects, including
  a complete 64×64 rectangle in one OAM entry. Irregular, overlapping, or
  individually flipped parts may need more objects.
- Equal nonblank 8×8 pixels share ROM storage. The active streaming working
  set can occupy OBJ VRAM, or an eligible immutable **reachable pose library**
  can stay resident and upload once. OBJ editor pages are not VRAM allocations.
- Literal clip names resolve at build time and narrow Mode 0's candidate
  frame set. Prefer direct branches with literal names for known states, as in
  [Nerdo](../examples/nerdo/game.lua). Dynamic names work, but Mode 0 budgets
  every registered clip they could select.

**A pose is not automatically faster than `spr`, and one pose is not always
one OAM entry.** Ordinary `spr` can also pack eligible immutable rectangles.
Packing saves object entries; it does not remove the pixels' scanline cost.
The chosen scene still has to fit OBJ VRAM, object count, scanline, ordering,
and transfer limits. `pspr`/`anim` themselves allocate no Lua objects, though
argument expressions can allocate.

All three object APIs are **Mode 0 conditional**. Using poses does not make
arbitrary raster drawing compatible with Mode 0: `pset`, `line`, `rectfill`,
and similar effects still require Mode 4. When the artwork can be preauthored,
put its pixels into shared tiles or poses; keep arbitrary runtime raster
effects when you actually need their semantics. Inspect the build's renderer
diagnostic rather than assuming an API choice guarantees hardware rendering.
The [renderer contract](render-modes.md) defines exact eligibility and fallback.

If only colors change, use a [color variant](ide.md#color-variants) instead of
painting duplicate poses. Geometry changes still need separate poses.
Variants have [additional Mode 0 restrictions](render-modes.md#pose-color-variants-pvariant):
that path excludes `spr`, `sspr`, `print`, `pal`, `palt`, and `sset`.

</details>

## Examples to copy

- **[Nerdo](../examples/nerdo/README.md):** the small starting point for idle/walk
  clips and facing. Its gameplay chooses a literal clip; the clip owns playback.
- **[Lumen](../examples/lumen/README.md):** characters, embers, lanterns, flowers,
  particles, and interface art use poses; loops, flower opening, and a recharge
  meter use named clips. Its gameplay-owned clock pauses animations, while
  map planes provide the four background layers.
- **[Shared tile animation](../examples/shared-tile-animation/README.md):** use
  this for terrain whose shared map tile must change, rather than expecting an
  object clip to animate map cells.

See the [editor walkthrough](ide.md#poses-and-animations) to author the assets
and the [API reference](reference.md#pose-sprites-and-animation-storage) for
signatures, bounds, storage, color variants, and errors.
