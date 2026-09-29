# Mtile IDE

Mtile keeps the complete game-making loop in one resizable Ebitengine
window. Its four workspaces share the same project model:

1. **Sprite Editor** paints shared tiles and poses, and assembles named animations. Shared
   tile 0 is the map eraser; animation tile 0 is paintable.
2. **Map Editor** places sprite cells on the GBA map.
3. **Audio Studio** authors the four Game Boy hardware channels available in
   the GBA: CH1 pulse+sweep, CH2 pulse, CH3 programmable wave, and CH4 noise.
4. **Develop** combines a multi-file Lua editor, build diagnostics, generated
   Go, and the live interactive cartridge in one workstation.

Use the tabs, keys `1`–`4`, or Tab outside a text field to change workspace.

## Sprite controls

The Sprite Editor keeps its everyday tools in one compact rail below the
pixel canvas. Pencil and Eraser support continuous click-drag painting. Fill
recolors one connected region; Line, Rectangle, and Circle use click-drag to
set their bounds. Hold Shift while releasing Rectangle or Circle to make a
filled primitive. Right-drag always erases, independently of the selected
tool.

| Action | Shortcut |
| --- | --- |
| Pencil / Eraser / Fill | P / E / F |
| Line / Rectangle / Circle | L / R / C |
| Flip selection horizontally / vertically | Shift+H / Shift+V |
| Copy / paste selected tiles | Ctrl/Cmd+C / Ctrl/Cmd+V |
| Cut selected tiles in Poses | Ctrl/Cmd+X |
| Paste in Poses (alternate shortcut) | Ctrl/Cmd+P |
| Resize the multi-sprite selection | Ctrl/Cmd+[ / Ctrl/Cmd+] |

In Shared tiles, the top-left spritesheet cell is sprite 0, the map's permanent transparent
eraser. A lock overlay marks it in both the sheet and pixel canvas; drawing,
fill, paste, primitives, and flips cannot modify it. The dashed spritesheet
grid remains a UI overlay and is never written into the game artwork.

## Poses and animations

Sprite Lab separates three jobs:

- **Shared tiles** draws the bank used by `map`, `spr`, and `sspr`.
- **Poses** draws character artwork and registers complete poses for `pspr`.
- **Animations** assembles registered poses into named clips for `anim`.

OBJ pages are **512×320 pixels**, containing **2,560 8×8 tiles**. Like the
Shared tiles view, each page fills the available canvas. These are ROM artwork
pages, independent of the GBA's active OBJ memory: the runtime budgets only the
currently drawn poses against VRAM, OAM, scanline and transfer limits.

**PAGE+** adds another page of the same size. When fewer source tile positions
remain, it adds a smaller final page: **512×192 pixels**. The full bank holds
**65,536 source tile positions** across 25 full pages and that final page;
PAGE+ is disabled once the bank is full. Switch with the tabs/arrows and click
the selected tab to rename it. Existing sheets are repacked into this layout
while retaining pose appearance, pose IDs and clip order. Page changes,
painting, pasting and undo update immediately, without restarting the editor.

### Register a pose

1. Open **Poses** and choose a page.
2. Draw artwork, or copy a PNG and click **PASTE**. Its width and height
   must be multiples of eight and fit the selected page. Click to place it;
   Escape cancels. Imported colors use the project's palette.
3. Drag a rectangle around the complete pose on the page. The enlarged canvas
   at the left edits those pixels using the visible drawing/flip tools.
4. Click **REGISTER POSE**, type a name, and press Enter. Repeat for the other
   poses. The registered-pose strip lets you select and rename existing poses.

**UPDATE POSE** replaces the selected pose's tile selection. **DELETE POSE**
removes its registration and references from clips; it does not erase artwork.
Deleting a pose shifts later numeric pose IDs, so update any numeric `pspr`
references in Lua. Registering a pose refers to its existing source tiles;
registering two poses over the same tiles links their pixel edits.

### Copy or move artwork

In **Poses**, drag to select a rectangle of tiles, then use Ctrl/Cmd+C to copy
or Ctrl/Cmd+X to cut. Choose a destination page, or add one with **PAGE+**.
Press Ctrl/Cmd+P, Ctrl/Cmd+V, or **PASTE** to show a movable preview. Move the
pointer over the page and click to place it; Escape cancels. The entire
selection must fit inside the destination page.

Cut leaves the original artwork in place until placement succeeds. Clearing
the source and placing the destination form one undo step; canceling keeps
the original unchanged. Existing poses follow moved tiles, keeping their
appearance and animation references. If the source changed after cutting,
the move is rejected so newer edits cannot be erased. Undo and changing banks
cancel a pending cut.

Copied tiles keep their exact palette indices. Paste prefers your copied
selection over an older clipboard image; a newly copied external image takes
priority and uses the normal PNG import. Copies are independent artwork:
edit the new tiles, then click **REGISTER POSE** to make a separate pose for
an animation clip. Copying artwork does not duplicate its pose registration.

### Register and preview a clip

1. Open **Animations** and create a **NEW CLIP**. Name it, for example `idle`.
2. Open the pose dropdown, choose a registered pose, and use **ADD POSE**.
   The picker includes poses from every page. Repeat to build the sequence;
   a pose may appear more than once.
3. Reorder entries with the move controls. Removing an entry changes only this
   clip; it does not delete the registered pose or its artwork.
4. Set **FPS** and **LOOP**, then use **PLAY / PAUSE** or **STEP** to preview.
   A clip with Loop off stops on its final pose.
5. Click **REGISTER CLIP** to save it for game code. **SAVE CLIP** saves later
   changes. Ctrl/Cmd+S saves the whole project as well.

To draw the saved `idle` clip at its configured speed:

```lua
function _draw()
  cls(0)
  anim("idle", 100, 60)
end
```

The call draws the correct pose automatically using the game clock. To restart
an action independently for one character, store its start time and pass elapsed
seconds as the sixth argument:

```lua
-- When the character starts this action:
started = time()

-- In _draw():
anim("punch", x, y, false, false, time() - started)
```

The preview heading shows the clip's calculated width and height in pixels.
This area encloses all its poses and updates as you edit the clip. The optional
fourth/fifth arguments flip horizontally/vertically inside that fixed area:
`anim("idle", 100, 60, true)` faces the other way without changing the position.
mTile calculates the adjustment automatically when building; do not add a
width offset yourself. Standalone `pspr` calls use their pose's calculated area.
Use Loop off
for actions that should hold their final pose. Clips are saved in
`assets/animation.json`; builds compile their names, ordered pose IDs, speed and
loop behavior. See [the API reference](reference.md) for precise semantics.

### Tile reuse

**TILE REUSE** shows unique tile patterns and pixel-ROM savings. Select a tile
and enable **HIGHLIGHT MATCHING TILES** to mark equal cells, their page tabs,
and registered poses. Builds automatically store identical nonblank 8×8 pixel
patterns once and omit blank patterns. Editable copies remain independent;
poses referencing the exact same source tile share edits.

A full page fits 48 rectangular 40×80-pixel poses (12 across, four down).
Thirty such poses fit on one page per hero, so six pages comfortably hold six
heroes with 30 poses each. Animation clips can use poses from any page.

## Presentation and effects

Every workspace uses the same atomic-violet cartridge shell with a quieter
midnight work surface. The accent identifies the active tool: cyan for
sprites, moss green for maps, the four hardware-channel colors in Music Lab,
lavender for Develop, with power green reserved for its running-cartridge
controls. Effects are kept on the canvas or outer shell; source text and labels
never receive a display shader.

Open **Settings** in the top-right corner to choose a level, or press `F6` to
cycle between:

- **CLEAN**: still UI with no transition or canvas flourish;
- **SOFT**: the default, with short tab/caret/canvas feedback;
- **FULL**: stronger motion and display persistence for a more nostalgic feel.

The setting does not alter game assets, generated code, cartridge output, or
audio. Buttons retain their tactile pressed state in all three modes.

Settings also persists the embedded LCD treatment, preview audio volume, and
whether Develop saves dirty source files before a build. With autosave disabled,
Develop refuses a dirty build instead of compiling stale disk contents.

## Develop controls

| Action | Shortcut |
| --- | --- |
| Save | Ctrl/Cmd+S |
| Quick-open a project file | Ctrl/Cmd+P |
| Build and run | Ctrl/Cmd+R |
| Reload file changed on disk | Ctrl/Cmd+Shift+R |
| Find | Ctrl/Cmd+F |
| API completion | Ctrl/Cmd+Space |
| Undo / redo | Ctrl/Cmd+Z / Ctrl/Cmd+Shift+Z |
| Copy / cut / paste / select all | platform-standard shortcuts |
| Indent / outdent selection | Tab / Shift+Tab |
| Move by word | Alt/Option or Ctrl/Cmd + arrow |

The project tree reflects real files. Static top-level `require("module")`
declarations resolve relative to the requiring file and then `src/`; missing
modules, traversal, absolute paths, and dependency cycles fail the build with
the source location. Each Lua document keeps its own cursor, selection, scroll,
undo, and dirty state. Saving uses an atomic replacement and refuses to
overwrite a file changed by another program until it is explicitly reloaded.

Develop's source badge reports `GLOBALS n / 256` from the complete explicit
`require` graph, including dirty text in every open document. The badge warns
at 224 fields, marks 256 as full, and blocks builds above the runtime limit.
Output lists each distinct persistent name and its first source position;
Problems lists every name beyond the limit. Top-level `local` state counts
because it persists in the generated game, while function locals, parameters,
loop variables, and fields inside a state table do not consume global slots.

Offsets are Unicode rune positions, so cursor movement and selection do not
split UTF-8 characters. Parser diagnostics use the same Lua parser as the
build, so a reported syntax error is not a separate linter's approximation.

Problems, Output, Build, Runtime, Watches, Memory, and Generated Go are real bounded views.
Generated Go is the exact build artifact and receives Go syntax color rather
than pretending to be editable Lua.

### Memory inspector

After **Build + Reload**, open **Memory** in the Develop debugger. The panel
grows to show the report while keeping the cartridge preview visible; scroll
inside it for details at smaller window or larger text sizes.

- **Cartridge / ROM** shows the complete built ROM and remaining space under
  mTile's 32 MiB ROM limit. This is a build limit, not a measurement of a
  physical cartridge's chip. ROM holds code and assets; it is separate from
  working RAM.
- **Pose library** shows registered poses/clips, actual linked OBJ pixels,
  and linked pose/clip/index arrays. Tile dedup compares painted source cells
  with unique pixel patterns; blank page capacity does not count as artwork.
  The array total excludes anonymous clip-name strings and alignment gaps.
- **Loaded now / pose cache** reports cached OBJ pixel bytes, cached windows,
  and the first four cached pose names/IDs, followed by a count if more remain.
  A cache window can retain a pose after the game stops drawing it. These are
  cache entries, not a list of currently visible characters.
- **OBJ upload / frame** is the actual number of OBJ tile bytes transferred
  in the latest completed presentation, including other sprites and any gaps
  inside a combined transfer. It excludes OAM. An unchanged pose can remain
  cached with **0 B** uploaded.
- **Runtime RAM** shows linker-measured main RAM (EWRAM) and fast RAM (IWRAM)
  use/headroom, including runtime state and reserved stacks. Pose staging is
  already included in these totals.
- **Why stream poses?** compares the unique artwork bytes that an
  all-artwork-in-RAM design would need with the actual linked pose staging
  buffer. It is an artwork-only comparison, not a second build or a measured
  total-game RAM saving. Other RAM and OBJ VRAM are excluded. No net saving is
  claimed for shared-sheet poses, absent buffers, or a library smaller than
  the staging allocation.

For example, Nerdo stores 10,752 bytes of unique OBJ pixels in ROM but reserves
only 2,816 bytes for pose staging. Its four idle poses retain 2,624–2,816 bytes
in OBJ VRAM as the clip advances; unchanged frames upload none. Keeping all
unique artwork in a RAM buffer would require 7,936 more bytes than that staging
buffer. Separately, merging identical source cells saves 64 bytes of pixel ROM.
These savings concern different memory pools and must not be added together.

Build figures belong to the last successful build, not unsaved artwork. Live
pose names are matched only to that same preview generation. Missing/older
telemetry and pending uploads are labeled unavailable rather than shown as
zero. Mode 4 explains that poses draw from ROM into the bitmap framebuffer;
it does not report a nonexistent hardware OBJ cache.

The cache scan and telemetry are preview-only. Native verification preserves
the release ROM byte-for-byte and checks actual LCD frame cadence as well as
pixel identity. See the [native debugger evidence](benchmarks/render-modes/pose-memory-debugger-v1/report.json).

The Runtime view receives live mGBA diagnostics, preview startup messages,
and mTile runtime failure messages. Lua memory failures also appear in
Problems with a suggested next step. Messages are bounded and belong to one
preview session, so stopped cartridges cannot write into a newer build's log.
Resetting or rebuilding allows the same failure to be reported again.

Logging is enabled only in editor preview ROMs. It covers mTile's explicit
runtime checks and startup failures; arbitrary TinyGo panics are not captured.
TinyGo 0.42's `mgbadebug` tag is currently incomplete for GBA, so mTile writes
directly to the emulator's debug registers while keeping its normal startup.

Building is asynchronous, so the UI remains responsive. A newer build or Stop
invalidates any older pending result; a late build can never replace the
cartridge the user most recently requested. Build + Reload selects the Build
view immediately, and the bezel banner keeps the current build/preview state
visible while the log fills below.

| GBA control | Keyboard |
| --- | --- |
| D-pad | Arrow keys |
| A / B | Z / X |
| Start / Select | Enter / Backspace |
| L / R | A / S |
| Rebuild and reload | Ctrl/Cmd+R |
| Release game input | Escape |
| Stop cartridge | Stop button |

Develop's **LCD / Clean** control is independent of the global effect level.
LCD uses an Ebitengine Kage shader for a subtle RGB dot matrix, scan response,
edge falloff, and optional frame persistence. Clean draws the exact
nearest-neighbor 240×160 emulator frame. Neither path changes the pixels
received from mGBA.

The inspector reads real VCOUNT/input state, CH1–CH4 PSG registers, frame/update/
draw counters, and a bounded set of named top-level Lua globals from a fixed
debug mailbox. The build's linker map supplies static/preallocated EWRAM,
remaining EWRAM headroom, and IWRAM use beside the live CPU budget. The host
audio queue is explicitly a desktop-preview transport; its startup grace
period is excluded from actionable dropout counts. Debug telemetry is included
only in editor preview builds and is absent from normal release ROMs.

Heap telemetry is separate from linker-measured EWRAM. `HEAP` is the current
bump-allocation cursor and can include unreachable garbage; `LIVE` is the byte
count retained by the most recent collection, while `PEAK`, `HANDLES`, and `GC`
show the largest observed live set, occupied handle slots, and collection
count. Health colour is based on `LIVE` after the first collection. A terminal
collector failure is shown in red, including `HEAP OOM` when the reachable
graph plus a requested allocation cannot fit in one 56 KiB semispace. Arbitrary
locals and source breakpoints are not claimed.

## Emulator architecture

The Develop surface does not contain a second Go-written emulator and does not
embed mGBA through cgo. The editor starts a hidden `mtile-player` process
built against the pinned mGBA core. A small versioned binary protocol carries
the GBA key mask to the helper and returns one 240×160 RGBA frame, 48 kHz stereo
PCM, hardware registers, and the bounded debug mailbox. Ebitengine owns the
only visible window and audio device.

This process boundary isolates native crashes, keeps mGBA headers out of the Go
application, and makes helper replacement explicit. The source, version, and
MPL notice are documented in [third-party/mgba.md](third-party/mgba.md).

## Release dependency policy

The release bundle contains:

- `bin/mtile`, `bin/mtile-editor`, and the native `bin/mtile-player`;
- the pinned TinyGo distribution under `toolchains/tinygo`;
- Mtile's Lua parser/transpiler and embedded GBA runtime;
- Mtile, mGBA, and TinyGo license notices.

The user installs only a compatible Go distribution as an additional
language/toolchain dependency. Lua, TinyGo, mGBA, CMake, SDL, and Qt are
implementation details of the bundle, not separate end-user prerequisites;
the operating system still provides its normal desktop graphics and audio
libraries. Maintainers create platform bundles with
[`scripts/release/build-bundle.sh`](../scripts/release/build-bundle.sh).
