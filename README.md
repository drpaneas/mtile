# Mtile

**Make Game Boy Advance games in Lua.**

Write your game, draw its sprites and maps, compose hardware sound, and build
an actual `.gba` cartridge—all from one desktop editor.

[Download Mtile](https://github.com/drpaneas/mtile/releases/latest) · [Get started](#quick-start) · [Documentation](docs/README.md) · [Examples](examples/README.md) · [Contribute](CONTRIBUTING.md)

**New to game development?** [Make your first game with Coin Quest](docs/first-game.md):
a playable starter with sprites, sound, and four guided lessons.

![Four independently scrolling backgrounds and a foreground sprite from Mtile's background-layers example, running in mGBA.](docs/media/background-layers.gif)

*An actual ROM built from [background-layers](examples/background-layers).
[Still image](docs/media/background-layers.png) · [Capture details](docs/media/README.md)*

## Why Mtile?

- **Lua to native code.** A small, PICO-8-inspired Lua dialect compiles through
  Go and TinyGo. Your cartridge does not run a Lua interpreter.
- **A complete editing loop.** Sprite, map, audio, and code workspaces share
  one project, with build diagnostics and a playable mGBA preview.
- **Real GBA graphics and sound.** A 240×160 display, indexed sprites, layered
  maps, and the four Game Boy hardware sound channels available on the GBA.
- **Inspectable output.** Keep the generated Go, inspect memory and cycle
  budgets, and run the resulting ROM in an emulator or on GBA hardware.

```text
game.lua + assets → generated Go → TinyGo → game.gba
```

**Status: early development.** APIs and asset formats can change. Games target
about 30 updates per second; actual performance depends on the workload.
Mode 4 provides software rendering, while Mode 0 uses hardware tiles and
sprites for supported projects. `auto` selects Mode 0 only when its compatibility
and cost checks pass. See [renderer support](docs/render-modes.md) and
[measured limits](docs/performance.md). Physical-GBA audio validation is
[still pending](docs/audio/hardware-signoff.md).

## Quick start

1. [Download the archive for your desktop](https://github.com/drpaneas/mtile/releases/latest). Choose Windows x64, macOS Apple Silicon or Intel, or Linux x64 or ARM64.
2. Extract the whole archive and double-click **Mtile.exe** (Windows), **Mtile.app** (Mac), or **Mtile** (Linux). **Coin Quest** opens ready to play. Move with the arrows, collect five coins, and press Z to restart.
3. Press Escape to edit `game.lua`. Change `speed = 2` to `speed = 3`, save, and choose **Build + Reload**. Click the preview to try the faster explorer. Choose **Export .gba** to save your game as a cartridge.

The download includes Go, TinyGo and the preview player. You can play, edit
and export offline. The four guided lessons live in your project's `lessons/`
folder. [Desktop setup](docs/getting-started.md#using-a-desktop-bundle) ·
[Your first game](docs/first-game.md)

**Using the terminal?** Open the extracted folder containing the launcher and
`examples/` and enable the [bundled `mtile` command](docs/getting-started.md#use-mtile-in-a-terminal).
Open Hello from that folder. Close the editor, then build its cartridge.

```sh
mtile edit examples/hello
mtile build examples/hello
```

Your ROM is `examples/hello/game.gba`. To create your own project, run
`mtile new -template coin-quest mygame` for Coin Quest or `mtile new blankgame`
for a blank game. Choose an unused directory name. To compile Mtile itself,
follow [Build the desktop tools](docs/getting-started.md#build-the-desktop-tools).

For an external code editor, [`mtile lsp`](docs/language-server.md) provides
builtin completion, hover help, and live Lua and renderer diagnostics.

![mTile Sprite Lab showing registered Nerdo poses, drawing tools, and the OBJ artwork page.](docs/media/editor/poses.png)

*The desktop editor. See the [illustrated IDE guide](docs/ide.md) for all four
workspaces, color variants, animations, rooms, and preferences.*

For character and object artwork, read [when to use poses and animations](docs/poses-and-animations.md).
Use `spr` for simple shared-sheet cells, `pspr` for a complete authored object,
and `anim` for a timed sequence of poses.

## A small game fits in a small file

For a separate experiment in the blank project above, replace its `game.lua`
with this complete example. Move the circle with the left and right arrow keys:

```lua
x = 120

function _update()
  if btn(0) then x -= 2 end
  if btn(1) then x += 2 end
  x = mid(4, x, 235)
end

function _draw()
  cls(0)
  circfill(x, 80, 4, 7)
  print("hello, gba", 8, 8, 7)
end
```

Mtile uses 16:16 fixed-point numbers and statically bound calls. It supports
a deliberate subset of Lua; it is not a drop-in Lua 5.x or PICO-8 cartridge
runner. Read the [language reference](docs/reference.md#lua-dialect) before
porting a larger game.

## Explore the examples

| Start with | What you will learn |
| --- | --- |
| [Hello](examples/hello) | Input, sprites, and a mapped background |
| [Multiple files](examples/multi-file) | Organizing a game with `require()` |
| [Background layers](examples/background-layers) | Four map planes and parallax |
| [Coin block](examples/coin-block) | Animation and changing a map cell |
| [Hardware melody](examples/audio-gb-melody) | Four-channel sound and music |

The [example index](examples/README.md) separates learning projects from
benchmarks and research projects.

## Documentation and contributions

Browse the tutorials and search the Lua API in the [mTile guide](https://drpaneas.com/mtile/).
The [website README](website/README.md) explains how to build and preview it locally.

- [Getting started](docs/getting-started.md): installation, first ROM, and editor preview.
- [IDE guide](docs/ide.md): sprite, map, audio, and code controls.
- [Poses and animations](docs/poses-and-animations.md): when to use `spr`, `pspr`, or `anim`, with working examples.
- [Reference](docs/reference.md): commands, project structure, input, and Lua builtins.
- [Audio guide](docs/audio/README.md): hardware sounds and music.
- [Performance](docs/performance.md): frame budgets, memory limits, and benchmark evidence.
- [Contributing](CONTRIBUTING.md): repository map, checks, and focused ways to help.

Bug reports with a small reproducer, clearer examples, documentation fixes,
and measured runtime improvements are welcome. For help, see
[Support](SUPPORT.md). Please follow the [Code of Conduct](CODE_OF_CONDUCT.md).
If Mtile is useful to you, a star helps other game developers discover it.

## Credits and license

Mtile is [MIT licensed](LICENSE). Its API is inspired by
[PIGO8](https://drpaneas.com/pigo8/) and PICO-8. The default palette is
[DawnBringer's Aurora](https://lospec.com/palette-list/aurora).
[TinyGo](docs/third-party/tinygo.md) compiles the cartridge, and
[mGBA](docs/third-party/mgba.md) powers the preview. Bundled components retain
their own licenses; see [third-party notices](docs/third-party/README.md).

Mtile is an independent homebrew project, unaffiliated with Nintendo.
