# Poses, animation, and facing

This small example draws a clockwork surveyor using two saved clips: `idle` (four
poses) and `walk` (five poses), both at 6 FPS. It demonstrates animation rather
than movement or collision: the character stays at `(100, 60)`.

```sh
go run ./cmd/mtile edit examples/nerdo
go run ./cmd/mtile build examples/nerdo
```

Build + Reload in Develop, then click the preview. Hold left/right to play
`walk` and change facing; release to return to `idle`.

## Follow the code

`_update()` reads input and stores `walking` and `facing_left`. `_draw()` clears
the screen and calls `anim` with the saved clip name and horizontal flip.
The runtime selects the frame using the game clock. Changing clips does not
restart that shared clock; this example deliberately has no per-action timer.

In Sprite Lab → Poses, select a registered pose to inspect its source tiles.
In Animations, select `idle` or `walk`, change FPS, and preview the sequence.
Save and rebuild to see the change in the cartridge. Call-level flips use the
clip's combined pose bounds automatically; do not add a manual width offset.

## Try a color variant

The example starts with original colors and no saved variants. To author one:

1. Select a pose and open **COLOR VARIANTS**.
2. Create a variant named `alternate`.
3. Change a used color's replacement; inspect both previews, then save.
4. Replace `_draw()` with this comparison and rebuild:

```lua
function _draw()
  cls()
  pvariant()
  anim("idle", 40, 60)
  pvariant("alternate")
  anim("idle", 140, 60)
  pvariant()
end
```

Both characters use the same pose geometry. Replacements are project-wide,
so a variant applies to any pose using those source indices; index 0 remains
transparent. Reset with `pvariant()` before later pose draws. The name must
exist before building this snippet. Mode 0 variant banks have additional
resource and API-combination restrictions; AUTO can fall back to Mode 4.
See [color variants](../../docs/ide.md#color-variants) and
[the renderer contract](../../docs/render-modes.md#pose-color-variants-pvariant).

The surveyor's original artwork and its generator use the [repository's MIT
license](../../LICENSE). To restore the supplied object tiles and poses, run
`python3 examples/nerdo/gen_assets.py` from the repository root.
