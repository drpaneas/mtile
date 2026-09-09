# Mtile IDE

Mtile keeps the complete game-making loop in one resizable Ebitengine
window. Its four workspaces share the same project model:

1. **Sprite Editor** paints the indexed spritesheet. Sprite ID 0 is permanently
   transparent and cannot be painted.
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
| Copy / paste one sprite | Ctrl/Cmd+C / Ctrl/Cmd+V |
| Resize the multi-sprite selection | Ctrl/Cmd+[ / Ctrl/Cmd+] |

The top-left spritesheet cell is sprite 0, the map's permanent transparent
eraser. A lock overlay marks it in both the sheet and pixel canvas; drawing,
fill, paste, primitives, and flips cannot modify it. The dashed spritesheet
grid remains a UI overlay and is never written into the game artwork.

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

Problems, Output, Build, Runtime, and Generated Go are real bounded views.
Generated Go is the exact build artifact and receives Go syntax color rather
than pretending to be editable Lua.

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
