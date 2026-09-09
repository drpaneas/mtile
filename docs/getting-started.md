# Getting started

[Documentation index](README.md) · [Command reference](reference.md#commands)

Start with a source checkout. This guide does not assume a published binary
release is available. If you already have a maintainer-built desktop bundle,
see [using a bundle](#using-a-desktop-bundle).

## Requirements

| Task | Dependencies |
| --- | --- |
| Build a ROM from source | Go 1.27.1+ and TinyGo 0.42.0 on `PATH` |
| Open the source-checkout editor | The above, CMake, a C compiler, and OS graphics/audio support |
| Use a desktop bundle | Go 1.27.1+ and OS graphics/audio support; TinyGo and the emulator helper are bundled |

Install [Go](https://go.dev/doc/install) and the pinned
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

For a playable game with lessons, follow [Your first game](first-game.md).
The commands below create a blank project instead.

```sh
git clone --recurse-submodules https://github.com/drpaneas/mtile.git
cd mtile
go run ./cmd/mtile new mygame
go run ./cmd/mtile build mygame
```

`new` creates the source and graphics assets and refuses to overwrite an
existing directory. `build` writes `mygame/game.gba` and reports the selected
renderer and memory use. Keep command flags before the project directory.

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
caches it. Choose **Develop**, edit `game.lua`, and select **Build + Reload**
(or Ctrl/Cmd+R). A successful build starts the ROM in the same window.

Click the GBA display to give it keyboard focus. Arrow keys are the D-pad,
Z/X are GBA A/B, and Escape returns focus to the source editor. The
[IDE guide](ide.md) describes the remaining controls and workspaces.

You can also load `mygame/game.gba` in a separately installed emulator or
transfer it to compatible GBA flash-cartridge hardware.

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

Unpack the bundle for your OS and architecture. Keep `bin/` and `toolchains/`
together, and add its `bin/` directory to `PATH`. Do not move just the `mtile`
executable: the sibling editor/player and bundled compiler are part of the
installation.

```sh
mtile new mygame
mtile build mygame
mtile edit mygame
```

The bundle supplies TinyGo, the Mtile runtime, and the mGBA helper. Go remains
a host dependency. You do not need a separate Lua installation, mGBA frontend,
CMake, SDL, or Qt to use the bundle. Build instructions for maintainers are in
[the release guide](../scripts/release/README.md).

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
