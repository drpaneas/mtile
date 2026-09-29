# Character animation capacity and correctness

This work builds on the existing, uncommitted OBJ-bank feature. Existing assets
and Lua drawing semantics must remain compatible.

## Outcome

Store a large character-animation library in ROM, author and resize it in Sprite
Lab, and stream the poses needed by the current frame. Remove implementation
limits that unnecessarily prevent fighters, projectiles, and effects from being
drawn together. Preserve the GBA's real memory, OAM, scanline, and VBlank limits;
AUTO must fall back to Mode 4 when exact hardware rendering cannot be proved.

## Implementation

1. Fix background `sset` uploads corrupting resident pose graphics. Reserve enough
   VBlank time for pose uploads and test unchanged, changed, and inactive poses.
2. Replace the two-pose runtime with compact per-site state and shared upload
   storage. Allocate nonoverlapping VRAM windows alongside `spr`/`sspr`, preserving
   painter order, text, and existing packed-object reservations.
3. Build hardware-object layouts from complete, nonoverlapping pose rectangles.
   Pack larger rectangles into larger GBA objects without changing pixels or
   flips. Use the same layout for compiler OAM/scanline accounting and runtime
   drawing. Preserve original pose parts for Mode 4.
4. Remove the requirement to enumerate a pose-ID expression when all authored
   poses can instead be conservatively budgeted. Retain precise subset proofs
   when available and exact rejection/fallback for actual resource overflow.
5. Expand the object bank to the full tile-ID address space, widen compact ROM
   indices and pose offsets to prevent overflow, and replace arbitrary asset
   limits with documented representation/hardware bounds.
6. Add safe object-sheet resizing with pose-ID remapping, undo, and persistence;
   improve sheet labels and reuse image import for large animation sheets.
7. Update the renderer contract and practical authoring documentation. Add a
   representative fighting-game fixture with multiple actors/effects.

## Acceptance

- Background edits cannot change independently stored character pixels.
- More than two poses and mixed ordinary sprites render correctly when budgets fit.
- Two dense 64x64 fighters leave hardware object entries available for effects.
- Large animation-ID expressions use the authored-pose bound when appropriate.
- Resize/import/save/reopen preserve pixels and pose appearance; shrinking cannot
  silently discard painted or referenced content; undo restores the prior state.
- Maximum supported compact-bank indices and aggregate pose offsets round-trip.
- Mode 0 and Mode 4 match in emulator captures, including flips, animation
  changes, overlap/painter order, and background mutation.
- Paired benchmark evidence covers cold, changed, and steady poses; no new
  hardware eligibility is claimed without that evidence.
- Required transpiler/build/CLI and runtime test gates pass, together with the
  affected editor/assets and tagged hardware-runtime suites.

## Physical bounds

The hardware has 128 OAM entries and finite OBJ VRAM/scanline/VBlank bandwidth.
The asset format uses 16-bit tile IDs and Lua uses signed 16.16 numbers. These
bounds cannot honestly be described as unlimited; the implementation should use
them efficiently and explain a failed forced-Mode-0 build or AUTO fallback.

## Progress

- Implemented all seven steps. The runtime uses packed hardware objects,
  compiler-assigned tile windows, and exact-size staging before VBlank.
- Animation storage supports 65,536 tiles and 32,768 pose recipes. Sprite Lab
  exposes Shared Tiles and Animation, safe resizing, and large image imports.
- Added regressions for mutable source/destination overlap, pending uploads,
  helper calls inside expressions, fixed-point ID wrapping, maximum asset
  indices, resize/undo/persistence, and invalid raw pose data.
- Native verification passed eight admitted cases across 960 game frames and
  2,040 LCD scans with zero differing pixels. Two oversized combinations safely
  fall back to Mode 4, matching another 240 frames. The largest accepted commit
  took 76,025 cycles of the usable 82,544-cycle window. See the
  [reproducible benchmark report](benchmarks/render-modes/pspr-stream-v1.md).
- Runtime tests pass with and without the hardware rendering tags. Asset,
  editor-model, editor-application, compiler, build, CLI, and recorded native
  evidence suites pass.

The extreme 128-pose, 512-tile stress case exceeds the CPU frame budget despite
safe presentation; the representative two-fighter scene does not. Remaining
limits describe actual storage formats, cartridge capacity, or GBA resources.
Conservative compiler proofs can still choose Mode 4 for scenes whose bounds
cannot be established. Cross-character tile sharing and 4-bit graphics remain
future optimizations rather than requirements for these verified workloads.
