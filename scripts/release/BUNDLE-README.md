# Mtile desktop bundle

Build native Game Boy Advance games in Lua.

## Install

Install Go 1.27.1 or newer from https://go.dev/doc/install. Unpack this directory
somewhere permanent and add its `bin/` directory to `PATH`. Keep `bin/` and
`toolchains/` together. TinyGo, the editor, and the mGBA preview helper are
included; do not move only the `mtile` executable.

## Make a game

```sh
mtile doctor
mtile new -template coin-quest mygame
mtile build mygame
mtile edit mygame
```

The ROM is `mygame/game.gba`. In the editor, choose Develop and use
Build + Reload after editing `game.lua`. Click the preview to play: arrow keys
are the D-pad, Z/X are A/B, and Escape returns focus to the editor.

Collect five coins and press Z to restart. `mygame/README.md` is an offline
walkthrough, with four runnable checkpoints under `mygame/lessons/`.
For a blank project, use `mtile new` without `-template coin-quest`.

`mtile help` lists commands; `mtile build -h` lists build options.
The first build can take longer while compiler/runtime caches are prepared.

## Status and support

Mtile is in early development. It uses a subset of Lua, targets approximately
30 updates per second, and has workload-dependent graphics and memory limits.
Audio uses the GBA's four Game Boy hardware channels. Read the project guides
before porting a larger game.

Source, documentation, and issue tracker: https://github.com/drpaneas/mtile

Attach `BUILD-INFO.txt`, your operating system, and the full build error when
reporting a problem. A bundle marked “modified” was built from a development
working tree rather than a clean revision.

Mtile's own code is MIT licensed; see `LICENSE`. Third-party notices, including
the compiler, emulator, and embedded fonts, are in `licenses/`. When sharing
a ROM containing the Aurora palette, include its source, notice and GPLv2
license from `licenses/aurora/`.
