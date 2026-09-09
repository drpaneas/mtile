# Mtile reference

[Documentation index](README.md) · [Getting started](getting-started.md)

Command flags, project structure, and the supported Lua language. For your first
ROM, follow the getting-started guide; for a working project, browse the
[examples](../examples/README.md).

- [Commands](#commands)
- [Project layout](#project-layout)
- [Game loop](#game-loop)
- [Input](#input)
- [Drawing](#drawing)
- [Audio](#audio)
- [Lua dialect](#lua-dialect)
- [Built-in functions](#built-in-functions)

## Commands

```
mtile doctor [options]
mtile new   [options] project-dir
mtile build [options] project-dir
mtile edit  [options] project-dir
mtile render-audio [options] project-dir
```

`new` writes a native project and refuses to overwrite an existing directory.

`doctor` checks the ROM toolchain, reports each failure with a suggested fix,
and exits nonzero if setup needs attention. It takes no project directory.
Use `-tinygo` to select a compiler as with `build`. It does not validate
graphics/audio availability or compile a ROM.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-template` | `blank` | `blank` or `coin-quest`, a playable game with four lessons; Coin Quest needs a sheet at least 24×8 pixels |
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
| `-renderer` | project setting | One-build override: `mode4`, `mode0`, or `auto` |

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
text. See the [IDE guide](ide.md) for controls and architecture.

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
├── project.json                 # palette, renderer policy, optional Lua entry
└── assets/
    ├── spritesheet.json         # 8bpp pixels, 8x8 sprite cells, flags
    ├── map.json                 # legacy BG0 grid or versioned BG0-BG3 planes
    ├── palettes/
    │   └── aurora.json          # 16 or 256 RGB15 colors
    └── audio/
        ├── settings.json        # Game Boy PSG output settings
        ├── sounds.json          # typed CH1–CH4 hardware sounds
        └── music.json           # clips, sections, and songs
```

Sheet and map sizes are project data. There is no shared sheet/map address
space: `sset` never writes map cells, and `mset` never writes sheet pixels.

Authored JSON must contain one complete value with valid UTF-8 text and unique
object keys. Duplicate keys, including escaped spellings or case aliases of
the same recognized field, are rejected with a file-specific error instead of
silently replacing a value. The editor writes valid JSON automatically.

Projects contain one shared sheet and either a legacy BG0 map or four versioned
BG0-BG3 planes. The Map Editor selects and hides planes like a layer stack;
`map()` draws every non-empty authored plane, while every non-empty
`map(bg, ...)` call uses its first argument as the selected plane. Old
positional calls migrate by prefixing the intended BG number. Map data access
similarly accepts `mget(bg,x,y)` and `mset(bg,x,y,id)`, while the legacy shorter
forms remain exact BG0 aliases.
See the [graphics model](graphics-model.md) and
[map asset schema](map-assets.md) for ordering, migration, masks,
parallax, mutation, and Mode 0 BG/OBJ behavior.

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

`_init` runs once at startup. `_update` and `_draw` target one update every
two GBA display frames (about 30 Hz). Work must fit the 561,792-cycle budget
to maintain that rate; expensive frames can miss it. `_update60` is rejected.
See the [performance guide](performance.md) for measurements and limits.

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

The compatibility renderer draws into a 240x160 Mode 4 software framebuffer:

- Color 0 is transparent for `spr` / `sspr` / `map` until `palt` changes it
- `cls` does not change the current draw color
- `print` uses a 6x8 cell font (40 columns by 20 rows)
- `print(str, color)` vs `print(str, x, y [, color])`

The exhaustive [render-mode compatibility matrix](render-modes.md)
classifies every builtin as renderer-independent, compatible with both
backends, conditionally compatible with Mode 0, or Mode 4 only. Mode 0 supports
statically proven map layers, camera/streaming/mutations, bounded sprites and
text, palette remaps, full clip reset, and aligned unscaled `sspr`; unsupported
forms fail before TinyGo. `auto` combines that semantic proof with the
benchmark-backed `tile-v1` cost profile: bounded `sset`/`mset` remain eligible,
including literal-layer `mset(bg,...)`, while runtime `fset`, dynamic-layer
mutation, and unbounded mutation fall back exactly to Mode 4 with a diagnostic.
The corresponding
[engineering and benchmark plan](render-mode-plan.md) defines the semantic
and performance gates.

## Audio

Audio v2 is the active runtime, compiler, asset format, and editor contract.
Legacy PICO-compatible or PCM assets are rejected with an unsupported-format
diagnostic rather than silently converted. The superseded software mixer and its tooling have been removed.

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

The implemented design is documented in the [audio overview](audio/README.md),
[engineering plan](audio/plan.md), [hardware model](audio/gba-hardware.md),
and [editor specification](audio/editor.md). Physical-GBA listening and
capture is the remaining release sign-off gate.

## Lua dialect

Mtile Lua is a small language, not full Lua 5.x. Supported extras:

- `?expr` print shorthand at the start of a line
- `!=` alongside `~=`
- compound assignment (`+=`, `<<=`, ...), including table fields
- binary literals (`0b1010`) and integer division (`\`)
- bitwise operators `& | ^^ ~ << >> >>> <<> >><`
- single-line `if (condition) statement`

Mtile treats the first statically observed numeric type of a global, local,
numeric-for counter, arithmetic expression, or record field as permanent and
lowers it to native 16:16 storage. Static field access (`object.x`) likewise
assumes a stable table layout after its first checked lookup. Code that changes
those types or record shapes is outside the optimized language contract; use a
separate binding or bracket indexing (`object["x"]`) for dynamic values.
Fixed global array literals containing only numbers also use compact 4-byte
numeric elements when they are accessed only through direct indexing or
`#array`, never escape, and are not shadowed or explicitly assigned a
non-number. Such arrays retain Lua's one-based indexing but their element type
is permanent; escaping, mixed, or otherwise unproven arrays keep ordinary
12-byte `rt.Value` table slots.
User-function parameters and return positions also lower to native numbers
when every static call site supplies numbers, every path returns a number in
that position, and the function cannot fall through. Multi-return functions
are analyzed slot by slot, so `(number, string, number)` becomes
`(rt.Number, rt.Value, rt.Number)`. Mixed, missing, recursive-unknown, or
otherwise unproven positions retain general Lua values. A specialized
parameter or result position must remain numeric for the duration of the call.
Lua expression-list adjustment is preserved at assignments, returns, table
constructors, and calls: only a bare final call expands to its declared result
count; parentheses collapse it to one value; a zero-result final call supplies
no value but still executes; missing destinations are nil-filled and excess
results are discarded. Builtin arity checks use that effective list rather
than the number of written argument expressions.
Calls to transitively proven non-allocating user functions bypass the temporary
GC-root wrapper. Functions that may create/grow tables or strings, contain an
unknown allocation effect, or participate in an unproven recursive cycle keep
the protected call path. The transpiler proves the complete transitive root
budget and rejects an over-capacity call chain or a recursive cycle whose live
roots can grow without bound; it never defers that case to a silent runtime
nil/OOM fallback. This changes call cost, not Lua-visible semantics.

Arithmetic uses the runtime's signed 16:16 domain. `%` follows Lua floor-modulo
sign rules, string ordering is bytewise lexicographic for both static and heap
strings, and `^` currently requires an integer literal exponent (including a
parenthesized or negated literal) so the fixed-point result is exact and
bounded. Dynamic or fractional exponents fail at their source position.
Table literals, static strings, and their generated registries are checked
against runtime capacities before Go generation.

Calls are statically bound. Function/builtin values, dynamic calls, local
function declarations, namespace aliases/escapes, and later mutation of a
statically lowered function or namespace are rejected explicitly. Every
explicitly initialized table that backs such a namespace must use a fresh table
constructor, including nested namespace fields; this prevents an older table
alias from mutating statically lowered dispatch. An implicit method `self` may
access and mutate ordinary data fields, but it cannot escape, be rebound, or be
shadowed. A genuinely local data table may shadow a namespace for ordinary
field/index access, but calling through that shadow is still dynamic and
unsupported. Top-level locals have declaration-order
visibility and become persistent Game fields; nested block locals remain
lexical, including compiler-generated temporaries for compound assignment.

Rejected at parse time: closures, `goto` / labels, variadic functions, and
`string.format`. PICO-8-style `foreach(table, function_name)` is supported as
statement syntax and lowers to an `all(table)` loop; the callback must be a
static user-function or builtin name, not a closure or dynamic expression.
See the dedicated [`foreach` guide](loops/foreach.md) for semantics,
examples, performance guidance, and mutation pitfalls.

## Built-in functions

The function set and calling conventions follow
[PIGO8](https://drpaneas.com/pigo8/)'s PICO-8-style API.
Renderer compatibility and the exact Mode 0 restrictions for every function
are documented in [docs/render-modes.md](render-modes.md).

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
