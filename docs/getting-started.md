# Getting started

[Documentation index](README.md) · [Command reference](reference.md#commands)

This page gets your tools ready and opens **Coin Quest**, a complete starter
game. Once it runs, continue with [one small code change](first-game.md#your-first-change).
You can return to the remaining setup details when you need them.

If you have a desktop download, start with [opening the bundle](#using-a-desktop-bundle).
For a source checkout, follow the requirements below.

## Requirements

| Task | Dependencies |
| --- | --- |
| Build a ROM from source | Go 1.27.1+ and TinyGo 0.42.0 on `PATH` |
| Open the source-checkout editor | The above, CMake, a C compiler, and OS graphics/audio support |
| Use a desktop bundle | Desktop graphics and audio support on the target OS. Go, TinyGo and the player are included |

For a source checkout, install [Go](https://go.dev/doc/install) and the pinned
[TinyGo 0.42.0 release](https://github.com/tinygo-org/tinygo/releases/tag/v0.42.0).
Check that both are available:

```sh
go version
tinygo version
```

Once you have a checkout or desktop bundle, `mtile doctor` checks the ROM build
tools and suggests fixes. From a checkout use `go run ./cmd/mtile doctor`.
Go must already be installed to run the CLI from source.

For source-checkout preview, install [CMake](https://cmake.org/download/) and
a C compiler. On macOS, the Xcode Command Line Tools provide the compiler.
On Windows, use a CMake-compatible native C toolchain. CI currently exercises
Linux and macOS; Windows source/bundle builds need separate verification.

On Debian/Ubuntu, the editor additionally needs these development packages:

```sh
sudo apt install build-essential cmake pkg-config libc6-dev libgl1-mesa-dev \
  libglu1-mesa-dev libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev \
  libxxf86vm-dev libasound2-dev
```

The CLI-only `new` and `build` commands do not open a desktop window.

## Build your first ROM

Create Coin Quest so your first build already has something to play:

```sh
git clone --recurse-submodules https://github.com/drpaneas/mtile.git
cd mtile
go run ./cmd/mtile doctor
go run ./cmd/mtile new -template coin-quest mygame
go run ./cmd/mtile build mygame
```

`new` creates the source and graphics assets and refuses to overwrite an
existing directory. `build` writes `mygame/game.gba` and reports the selected
renderer and memory use. Keep command flags before the project directory.

**Ready:** `mygame/game.gba` exists. Next, open it in the editor below. If you
prefer an empty canvas later, `go run ./cmd/mtile new blankgame` creates a
blank project in a different directory.

If you cloned without submodules, initialize the pinned emulator source:

```sh
git submodule update --init --recursive
```

## Play and edit

From the repository root:

```sh
go run ./cmd/mtile edit mygame
```

The first preview in a source checkout builds the headless mGBA helper and
caches it. Choose **Develop** and select **Build + Reload**
(or Ctrl/Cmd+R). A successful build starts the ROM in the same window.

Click the GBA display to give it keyboard focus. Arrow keys are the D-pad,
Z/X are GBA A/B, and Escape returns focus to the source editor. The
[IDE guide](ide.md) describes the remaining controls and workspaces.

**Ready:** collect five coins and press Z to restart. Continue with
[your first change](first-game.md#your-first-change): change one value, rebuild,
and see the result. That is enough setup to begin making the game your own.

![Develop workspace showing Lua code, Build + Reload, diagnostics, and the preview before building.](media/editor/develop.png)

For readable text, open Preferences with Ctrl/Cmd+`,` and choose COMFORT or
LARGE, or use Ctrl/Cmd+`+`/`-`. Ctrl/Cmd+`0` restores COMFORT.

After making changes in the editor, use **Export .gba** to build an updated
release ROM. You can load that file in another emulator or transfer it to
compatible GBA flash-cartridge hardware.

## When you are ready to change the artwork

Coin Quest uses simple shared-sheet sprites. Start by recoloring its explorer
in **Sprites**, save, and rebuild. The [first-game walkthrough](first-game.md)
shows where to find it.

For a character with several visual states, try
[Moth Study](../examples/moth-study/README.md): one pose, then a clip, then
input and facing. For scenery, [Tidewalk](../examples/tidewalk/README.md)
introduces a map, camera, and parallax separately. Or
[choose another small game](small-games.md); each comes with ready artwork.

## Build the desktop tools

You can use `go run` throughout development. To build executable files instead:

```sh
go build -o build/mtile ./cmd/mtile
go build -o build/mtile-editor ./cmd/editor
```

Keep the binaries together so `mtile edit` can find `mtile-editor`. On Windows,
use `build/mtile.exe` and `build/mtile-editor.exe`. The GBA runtime and Lua
frontend are embedded; source builds still need TinyGo on `PATH`.

## Using a desktop bundle

Download the archive for your operating system and processor from
[GitHub Releases](https://github.com/drpaneas/mtile/releases/latest) and extract it. Double-click
**Mtile.app** on macOS, **Mtile.exe** on Windows, or **Mtile** on Linux. The
macOS archives target macOS 14 or later on Apple Silicon and macOS 15 or later
on Intel. The Linux build uses the desktop's X11, OpenGL and audio libraries.

Coin Quest opens in Develop with the game ready for arrow-key input. Press
Escape to edit. Your project is saved in **mTile Projects/Coin Quest** inside
your home folder and reopens on the next launch. Change `speed = 2` to
`speed = 3` in `game.lua`, save, and choose **Build + Reload**. **Export .gba**
writes your modified game to a cartridge file. All of this works offline.

The CLI is also available under `bin/` on Linux and Windows and inside
`Mtile.app/Contents/MacOS/` on macOS. The [release guide](../scripts/release/README.md)
describes how maintainers build and test the archives.

## Common setup problems

| Symptom | Check |
| --- | --- |
| TinyGo unavailable | Run `tinygo version`, use `-tinygo /path/to/tinygo` before the project argument, or set `MTILE_TINYGO`. |
| Preview cannot build its helper | Initialize submodules and check CMake/C compiler availability. An existing helper can be selected with `MTILE_PLAYER`. |
| Linux editor fails to compile | Install the X11, GL, and ALSA development packages listed above. |
| Game ignores the keyboard | Click the embedded GBA display to focus it. |
| Forced Mode 0 build fails | Read the source-positioned diagnostic; use `-renderer auto` or `-renderer mode4` for unsupported drawing calls. |
| Game exceeds its frame budget | Inspect `stat(1)` and the [performance guide](performance.md); host CPU percentage is not GBA frame cost. |

Still stuck? Include the command, full error output, operating system, and
Go/TinyGo versions when [asking for help](../SUPPORT.md).
