# OBJ pages and named animation clips

## Outcome

Sprite Lab has three clear tabs: Shared tiles, Poses, and Animations. Authors
register poses from artwork, assemble named clips from a global pose picker,
preview their speed and loop behavior, and draw them from Lua with
`anim("idle", x, y)`. The app performs frame selection automatically.

## Design

- Pages use a 512×320-pixel canvas (2,560 8×8 source tiles), fitted to the
  editing area like Background. `PAGE+` adds the same size while capacity
  remains. After 25 full pages, the final page is 512×192 pixels (1,536 tiles),
  using all 65,536 source slots. Page size organizes the ROM library; the
  hardware separately budgets all simultaneously drawn objects.
- Normalize old sheets transactionally into this layout, preserving all painted
  pixels, flags, shared source references, pose appearance/IDs, and clips. New
  pages refit immediately and support undo/redo. Keep oversized legacy pose
  parts intact if their source art cannot fit together on one page.
- Poses uses Register Pose, Update Pose and Delete Pose. Its preview is static.
- Animations owns clip naming, a picker of all registered poses, an ordered pose
  sequence, Add Pose/Remove/Move, preview, FPS and Loop. New clips start empty.
  Register Clip validates and saves the clip for builds; saved clips can be
  renamed, updated and deleted. Pixel drawing stays in Poses.
- `anim(name, x, y, flip_x, flip_y, elapsed_seconds)` uses the saved clip speed.
  The last three arguments are optional. Default time is the game clock;
  explicit elapsed time lets characters restart/play independently. Looping
  wraps; a non-looping clip holds its final pose. Negative elapsed time starts
  at the first pose.
- Reuse the PSpr runtime and Mode 0 scene proof. Named clips add frame metadata,
  not another pixel bank. Exact tile deduplication and reuse highlighting remain.

## Verification

- Full-page and terminal-page dimensions, transactional normalization, exact composed-pose parity,
  preserved global pose IDs, save/reopen, maximum-bank behavior and undo/redo.
- Independent pose registration and clip assembly; global picker and ordering;
  clip save/delete; loop and one-shot preview timing.
- Inspect Poses and Animations at minimum/desktop sizes, including the picker.
- Public Lua registry/classification, generated clip assets, elapsed/default time,
  runtime pixels, forced Mode 0/AUTO budgets, missing clips and dynamic names.
- Paired native Mode 0/4 animation captures and required repository test gates.

## Scope

The page layout, editor vocabulary/workflow and named-clip API are in scope.
A general entity/state-machine system, physics and additional rendering modes
are not. GBA VRAM, OAM, scanline and transfer limits remain enforced.

## Status

Complete. Larger 512×320 pages replace the initial 128×256 page layout; only
the final capacity-limited page is smaller. Named clips replace the earlier
editor-only animation preview design. Existing artwork remains intact; opening
an old sheet repacks it in memory, and normal Save persists the result.

Verification passed:

- Full editor, model and assets test suites, including real Nerdo pose-pixel
  preservation on a disposable project copy and undo/rename regressions.
- Required transpiler, build, CLI and runtime suites.
- Native Poses/Animations captures at 1920×1200 and 1024×900, including the
  global pose dropdown and immediate blank-page addition.
- [Named animation emulator evidence](benchmarks/render-modes/anim-runtime-v1.md):
  240 game frames and 480 LCD scans compared across Mode 0/4, with zero pixel
  differences. Automatic playback also advances in a cart without manual
  counters or a `time()` call.

The editor and CLI binaries were rebuilt. See the
[editor walkthrough](ide.md#poses-and-animations) for the authoring workflow.
