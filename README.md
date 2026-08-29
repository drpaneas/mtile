# Mtile

[![CI](https://github.com/drpaneas/mtile/actions/workflows/ci.yml/badge.svg)](https://github.com/drpaneas/mtile/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/drpaneas/mtile.svg)](https://pkg.go.dev/github.com/drpaneas/mtile)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Mtile is a Lua-to-Game Boy Advance toolchain. You write a game in a small
Lua dialect, paint sprites and maps, author sound and music in the bundled
editor, and build a native Mode 4 GBA ROM.

The screen is the GBA's real **240x160** bitmap. Sprites are 8x8 cells on an
8 bits-per-pixel sheet. Maps are a grid of 16-bit sprite IDs. The default
palette is **aurora**, a built-in 256-color table taken from
[Aurora on Lospec](https://lospec.com/palette-list/aurora).

The Lua API (`cls`, `spr`, `map`, `btn`, `print`, ...) is heavily inspired by
[PIGO8](https://drpaneas.com/pigo8/), a PICO-8-flavored game API for Go, here
retargeted from a fantasy console to real GBA hardware.

```
game.lua + assets  →  generated Go  →  TinyGo  →  game.gba
```

## Requirements

The desktop release bundle requires only [Go](https://go.dev/dl/) 1.25 or
newer as a language/toolchain dependency. It includes the pinned TinyGo
toolchain, the Mtile runtime, the Lua frontend, and a headless mGBA player.
TinyGo, Lua, mGBA, CMake, SDL, and Qt do not need to be installed separately.
Normal desktop graphics and audio libraries still come from the operating
system.

A source checkout additionally needs TinyGo 0.41.1 on `PATH` for ROM builds
and CMake plus a C compiler to build the in-editor player.

On Linux the editor uses [Ebitengine](https://ebitengine.org/), which needs
the X11/GL development headers. On Debian/Ubuntu:

```sh
sudo apt install libc6-dev libgl1-mesa-dev libglu1-mesa-dev libxcursor-dev \
  libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev libasound2-dev pkg-config
```

macOS and Windows need no extra packages.

## Installation

For the simplest installation, unpack the matching Mtile release bundle and
put its `bin/` directory on `PATH`. The bundle is relocatable; keep its
`toolchains/` directory beside `bin/`.

To build from source instead:

```sh
git clone --recurse-submodules https://github.com/drpaneas/mtile.git
cd mtile
go build -o mtile ./cmd/mtile
go build -o mtile-editor ./cmd/editor
```

Put both binaries on `PATH`, or keep them next to each other. `mtile edit`
looks for a sibling named `mtile-editor` or `editor`, then the
`MTILE_EDITOR` environment variable, then `go run ./cmd/editor` from a
source checkout.

You can also run everything with `go run` from the checkout and skip the
install entirely (that is what the examples below do).

Source builds find this checkout's `runtime/` automatically. Installed
binaries materialize their embedded runtime into a private versioned cache;
`mtile build -runtime /path/to/runtime` remains available for development.

## Quick start

From this repository:

```sh
go run ./cmd/mtile new mygame
go run ./cmd/mtile edit mygame
go run ./cmd/mtile build mygame
```

That creates `mygame/` and opens one IDE with Sprite, Map, Audio Studio, and
Develop workspaces. Develop keeps Lua source and the live cartridge side by
side: edit `game.lua`, then press **Build + Reload**. The IDE writes
`mygame/game.gba` and runs it in the same window. A build automatically opens
the Build log while a compact state banner above the GBA screen reports
progress, failure, or the running preview.

You can still run the ROM externally or copy it to real hardware:

```sh
mgba mygame/game.gba        # or mgba-qt, or drag the file onto mGBA
```

Click the embedded GBA screen so it receives input. Arrow keys are the D-pad;
GBA A/B map to Z/X. Press Escape to return keyboard focus to source code.

## Commands

```
mtile new   [options] project-dir
mtile build [options] project-dir
mtile edit  [options] project-dir
mtile render-audio [options] project-dir
```

`new` writes a native project and refuses to overwrite an existing directory.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-width` | 240 | Sprite-sheet width in pixels |
| `-height` | 160 | Sprite-sheet height in pixels |
| `-map-width` | 30 | Map width in 8x8 cells |
| `-map-height` | 20 | Map height in 8x8 cells |

`build` transpiles `game.lua`, embeds the assets, and links a ROM.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-o` | `project-dir/game.gba` | Output ROM path |
| `-tinygo` | managed bundle, then `PATH` | TinyGo executable |
| `-runtime` | checkout or embedded runtime | Runtime module directory |
| `-keep-work` | off | Keep the temporary generated Go workspace |

`edit` opens the Ebitengine IDE. Audio Studio is one native four-channel Music
Lab: its piano/drum editor, CH1–CH4 hardware inspector, live-performance
controls, retained takes, and named song order remain visible together.
Develop is a multi-file Lua workstation with a real project tree, document
tabs, quick-open, selection, undo/redo, search, completion, syntax color, live
parser diagnostics, generated-Go inspection, structured build output, and a
persistent interactive mGBA preview. Its inspector reads the real PSG
registers, CPU cycles, linker-measured EWRAM/IWRAM use, and bounded debug
mailbox from the running cartridge; it does not invent CPU, memory, channel,
or watch values. Host audio buffering is labelled separately from cartridge
audio so a PSG-only game is never presented as using Direct Sound PCM.

Click the tabs, press `1` through `4`, or use Tab to switch workspaces. Saves
go to disk on mode switch, Ctrl/Cmd+S, normal quit, and—when enabled—before a
build. The shared GBA-inspired shell offers CLEAN, SOFT, and FULL visual effect
levels (`F6`). **Settings** persists that choice together with LCD treatment,
preview volume, autosave-before-build, and compact/comfort/large interface
text. See the [IDE guide](docs/ide.md) for controls and architecture.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-editor` | auto | Editor executable |

`render-audio` runs the same four-channel software PSG renderer used by the
Music Lab Play button and writes a deterministic 16-bit WAV. It is useful for
checking a song without a GBA, comparing editor output, or attaching an audio
preview to a review.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-o` | `project-dir/music-preview.wav` | Output WAV path |
| `-song` | 0 | Song ID to render |

## Project layout

```
mygame/
├── game.lua
├── src/                         # optional required Lua modules
│   └── player.lua
├── project.json                 # palette and optional Lua entry point
└── assets/
    ├── spritesheet.json         # 8bpp pixels, 8x8 sprite cells, flags
    ├── map.json                 # uint16 cell grid
    ├── palettes/
    │   └── aurora.json          # 16 or 256 RGB15 colors
    └── audio/
        ├── settings.json        # Game Boy PSG output settings
        ├── sounds.json          # typed CH1–CH4 hardware sounds
        └── music.json           # clips, sections, and songs
```

Sheet and map sizes are project data. There is no shared sheet/map address
space: `sset` never writes map cells, and `mset` never writes sheet pixels.

Generated games whose Lua source never calls `sset` compact the sprite sheet
by 8x8 cell: only non-empty cells are copied into EWRAM, and a two-byte index
preserves every declared sprite ID. Empty or unused capacity therefore costs
two bytes per ID instead of 64 pixel bytes. If any function calls `sset`, the
build automatically emits the traditional complete mutable EWRAM sheet;
`sget` and `fset` do not require that copy. This selection is exact because
the dialect has no first-class functions or dynamic builtin lookup.

A compact sheet can contain at most 65,535 non-empty cells (the existing map
ID limit is 65,536 cells including empty ID 0). `spr`, `sspr`, `map`, and
`sget` preserve logical sheet coordinates; the compact representation is an
internal build/runtime detail.

## Game loop

`game.lua` may define:

```lua
function _init()
end

function _update()
end

function _draw()
  cls(0)
  print("hello gba", 8, 8, 7)
end
```

`_update` and `_draw` run once every two display frames (30 Hz), matching
PICO-8's native `_update` rate: each rendered image is shown for exactly two
scans of the GBA's 59.73 Hz panel, paced by a hardware timer so the cadence
never wavers. `_init` runs once at startup. `_update60` is rejected; the
Mode 4 software framebuffer cannot honestly sustain it (see
[docs/performance.md](docs/performance.md)).

Numbers are 16:16 fixed point. `sin(0.25)` is `-1` because Y grows downward.

## Input

`btn(i)` and `btnp(i)` expose all ten GBA controls. Indices 0..5 retain
the original Mtile mapping for compatibility; the remaining native buttons
are appended:

| Index | GBA | `runtime` Go constant |
| ---: | --- | --- |
| 0 | Left | `ButtonLeft` |
| 1 | Right | `ButtonRight` |
| 2 | Up | `ButtonUp` |
| 3 | Down | `ButtonDown` |
| 4 | B | `ButtonB` |
| 5 | A | `ButtonA` |
| 6 | Start | `ButtonStart` |
| 7 | Select | `ButtonSelect` |
| 8 | L | `ButtonL` |
| 9 | R | `ButtonR` |

Held-button repeat for `btnp`: first extra fire at 15 update ticks, then
every 4 - PICO-8's native schedule at the same 30 Hz rate.

## Drawing

The current runtime draws into a 240x160 Mode 4 software framebuffer:

- Color 0 is transparent for `spr` / `sspr` / `map` until `palt` changes it
- `cls` does not change the current draw color
- `print` uses a 6x8 cell font (40 columns by 20 rows)
- `print(str, color)` vs `print(str, x, y [, color])`

The exhaustive [render-mode compatibility matrix](docs/render-modes.md)
classifies every builtin as renderer-independent, compatible with both
backends, conditionally compatible with a future Mode 0 backend, or Mode 4
only. Mode 4 remains the only shipping renderer; the matrix is the contract
for future `mode0` and `auto` build selection. The corresponding
[engineering and benchmark plan](docs/render-mode-plan.md) defines the staged
proof and its go/no-go gates.

## Audio

Audio v2 is the active runtime, compiler, asset format, and editor contract.
Legacy PICO-compatible or PCM assets are rejected with an unsupported-format
diagnostic rather than silently converted. The historical software-mixer
prototype and its evidence remain archived in
[`docs/audio/legacy/v1/`](docs/audio/legacy/v1/).

Opening Audio Studio in an older graphics-only project initializes empty v2
PSG assets. Projects with actual legacy audio remain unchanged and require an
explicit conversion.

Audio v2 is a deliberate hard reset around the four Game Boy PSG channels in
the Game Boy Advance: CH1 pulse with sweep, CH2 pulse, CH3 programmable wave,
and CH4 noise. The GBA hardware generates the sound; Mtile only schedules
bounded register updates at 256 Hz. Direct Sound, FIFO streaming, PCM samples,
software mixing, and audio DMA are explicitly deferred so this version stays
small, predictable, and inexpensive on the ARM7 CPU.

One runtime controller owns the shared sound registers, Timer 0, and CH1–CH4.
The v2 asset model contains `sounds.json`, `music.json`, and `settings.json`
only. It does not contain a sample library or PCM API.

The implemented design is documented in the [audio overview](docs/audio/README.md),
[engineering plan](docs/audio/plan.md), [hardware model](docs/audio/gba-hardware.md),
and [editor specification](docs/audio/editor.md). Physical-GBA listening and
capture is the remaining release sign-off gate.

## Lua dialect

Mtile Lua is a small language, not full Lua 5.x. Supported extras:

- `?expr` print shorthand at the start of a line
- `!=` alongside `~=`
- compound assignment (`+=`, `<<=`, ...), including table fields
- binary literals (`0b1010`) and integer division (`\`)
- bitwise operators `& | ^^ ~ << >> >>> <<> >><`
- single-line `if (condition) statement`

Rejected at parse time: closures, `goto` / labels, variadic functions, and
`string.format`. PICO-8-style `foreach(table, function_name)` is supported as
statement syntax and lowers to an `all(table)` loop; the callback must be a
static user-function or builtin name, not a closure or dynamic expression.
See the dedicated [`foreach` guide](docs/loops/foreach.md) for semantics,
examples, performance guidance, and mutation pitfalls.

## Built-in functions

The function set and calling conventions follow
[PIGO8](https://drpaneas.com/pigo8/)'s PICO-8-style API.
Renderer compatibility and the exact Mode 0 restrictions for every function
are documented in [docs/render-modes.md](docs/render-modes.md).

Graphics: `cls`, `pset`, `pget`, `line`, `rect`, `rectfill`, `circ`,
`circfill`, `spr`, `sspr`, `map`, `camera`, `clip`, `pal`, `palt`, `color`,
`cursor`, `print`

Assets: `sget`, `sset`, `fget`, `fset`, `mget`, `mset`

Input: `btn`, `btnp`

Math: `flr`, `ceil`, `abs`, `sgn`, `min`, `max`, `mid`, `sin`, `cos`,
`atan2`, `sqrt`, `rnd`, `srand`, `band`, `bor`, `bxor`, `bnot`, `shl`,
`shr`, `lshr`, `rotl`, `rotr`

Tables: `add`, `del`, `deli`, `count`, `all`, `pairs`, `foreach`

Strings: `sub`, `tostr`, `tonum`, `chr`, `ord`

Time: `t`, `time`, `stat`

`stat(n)` reports runtime telemetry: `stat(1)` is last tick's CPU load and
`stat(2)` is the worst since boot, each as a 0..1 fraction of the 30 Hz
budget (1.0 = the whole tick consumed by update, draw, and blit).

Native Audio v2 games expose these bounded integer counters:

| ID | Audio value |
| ---: | --- |
| `80` | 256 Hz scheduler service calls |
| `81` | Applied hardware events |
| `82` | Active PSG channels |
| `83` | Channel ownership replacements |
| `84` | Priority rejections |
| `85` | Rejected commands |
| `86` | Command-queue overflows |
| `87` | Maximum scheduler service cycles |
| `88` | Absolute 96-PPQN song tick |
| `89` | Completed songs |
| `90` | Minimum measured scheduler cycles |
| `91` | Mean measured scheduler cycles |
| `92` | Measured scheduler services |

Missing audio returns zero. Cumulative values saturate at 32,767 so they stay
representable as 16:16 integers instead of wrapping negative.

## Examples

From the repository root:

```sh
go run ./cmd/mtile build examples/hello/
```

- [`examples/hello`](examples/hello) - D-pad moves an 8x8 sprite on a mapped
  background. In mGBA, click the window and use the arrow keys.
- [`examples/mapdemo`](examples/mapdemo) - maximum-width continuous 16:16
  world (4095x20 cells; 136 full screens plus 15 tiles), with a full-screen
  `map()` redraw every tick and live CPU readout (~62% measured).
- [`examples/color`](examples/color) - `print` smoke project from `mtile new`
- [`examples/audio-gb-melody`](examples/audio-gb-melody) - compact native
  CH1–CH4 looping melody.
- [`examples/audio-gb-song`](examples/audio-gb-song) - multi-section native
  Game Boy song using all four hardware channels.
- [`examples/audio-gb-editor`](examples/audio-gb-editor) - Music Lab
  save/build/run round-trip fixture.
- [`examples/audio-sfx`](examples/audio-sfx) - legacy v1 interactive SFX fixture
- [`examples/audio-music`](examples/audio-music) - legacy v1 pattern fixture
- [`examples/audio-samples`](examples/audio-samples) - legacy v1 WAV/PCM fixture
- [`examples/audio-arpeggio-conformance`](examples/audio-arpeggio-conformance)
  is a legacy v1 deterministic capture fixture.
- [`examples/audio-zelda-fortune-teller`](examples/audio-zelda-fortune-teller)
  is a legacy v1 PICO-8 compatibility fixture.
- [`examples/audio-zelda-underworld`](examples/audio-zelda-underworld)
  is a legacy v1 PICO-8 compatibility fixture.

The PICO/PCM examples are archived migration fixtures. The active v2 compiler
intentionally rejects their legacy audio assets.

## Performance

The frame budget, measured cycle costs for every drawing operation on real
ARM7TDMI timing, hard limits (heap, tables, strings, map edits), and
good/bad practice for hitting 60 Hz are documented in
[docs/performance.md](docs/performance.md). The approved native-GBA audio
architecture, single-screen editor, hardware-resource plan, reference ROMs,
benchmarks, and staged release gates are in the
[audio engineering plan](docs/audio/plan.md). Host regression benchmarks run
with `go test -bench=. -benchmem` in `runtime/`; on-target cycle
measurements build with `make benchspike-gba`.

## Development

The CLI module is `github.com/drpaneas/mtile`. The GBA runtime is a second
module at `github.com/drpaneas/mtile/runtime` with no editor or TinyGo
host-tool dependencies. Hardware access goes through `runtime/hw/`.

```sh
go test ./...                  # CLI module
go test ./...                  # runtime module (run inside runtime/)
golangci-lint run ./...
```

TinyGo ROM tests skip when `tinygo` is not on `PATH`. CI runs tests and lint
on Linux and macOS, and builds the example ROMs with a pinned TinyGo; see
[.github/workflows/ci.yml](.github/workflows/ci.yml).

`runtime/target-gba.json` and `runtime/target-gba-smoke.json` are generated
by `make generate` in `runtime/` and contain machine-local paths; they are
gitignored and regenerated on demand.

## Acknowledgements

- The game API is heavily inspired by
  [PIGO8](https://drpaneas.com/pigo8/), which brings PICO-8-style functions
  (`cls`, `spr`, `map`, `btn`, `print`, ...) to Go.
- The default 256-color palette is
  [Aurora](https://lospec.com/palette-list/aurora), created by DawnBringer
  as part of the GrafX2 Toolbox, from the Lospec palette list.

## License

[MIT](LICENSE)
