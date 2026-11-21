# Mtile desktop bundle

Build native Game Boy Advance games in Lua.

## Open Coin Quest

Extract the whole download. On macOS, double-click **Mtile.app**. On Windows,
open the extracted folder and double-click **Mtile.exe**. On Linux, open
**Mtile** in the extracted folder. Keep the folder together when moving it.

Mtile opens Coin Quest in **Develop**. Move with the arrow keys, collect five
coins and press Z to restart. Press Escape to give keyboard focus to the code.
Your editable copy lives in **mTile Projects/Coin Quest** in your home folder.

Change `speed = 2` to `speed = 3` in `game.lua`, save, then press
**Build + Reload**. Click the preview and try moving again. The first rebuild
can take a little longer while the included compilers prepare their caches.
Choose **Export .gba** to save the modified game as a Game Boy Advance ROM.
You can play, rebuild and export offline. The archive includes Go, TinyGo and
the mGBA preview helper.

The project's README and four lessons are also available offline. Open a Lua
file under `lessons/` in Develop and choose **Run Lesson**. **Back to My Game**
returns to your own project.

## Open an example from the terminal

Open a terminal in the extracted folder containing the launcher and `examples/`.
Add the bundled CLI to this terminal's path using the command for your system.

**macOS** (the folder containing `Mtile.app`)

```sh
export PATH="$PWD/Mtile.app/Contents/MacOS:$PATH"
```

**Linux** (the folder containing `Mtile`)

```sh
export PATH="$PWD/bin:$PATH"
```

**Windows PowerShell** (the folder containing `Mtile.exe`)

```powershell
$env:Path = "$(Join-Path $PWD 'bin');$env:Path"
```

In that same folder, open Hello. Close the editor to return to the terminal,
then run the second command to export the game.

```sh
mtile edit examples/hello
mtile build examples/hello
```

The ROM is `examples/hello/game.gba`. Browse `examples/README.md` for other
projects in this download.

## Verify a built example

Download `examples.sha256` from the same GitHub release as your desktop package.
Save it in the extracted folder beside `examples/`. It lists the expected ROM
hash for each of the 20 unmodified standalone examples. After running
`mtile build examples/hello` in that folder, check Hello with the command for
your system.

**Linux**

```sh
grep '  examples/hello/game\.gba$' examples.sha256 | sha256sum -c -
```

**macOS**

```sh
grep '  examples/hello/game\.gba$' examples.sha256 | shasum -a 256 -c -
```

**Windows PowerShell**

```powershell
$hash = (Get-FileHash examples/hello/game.gba -Algorithm SHA256 -ErrorAction Stop).Hash
if ((Get-Content examples.sha256 -ErrorAction Stop) -notcontains "$hash  examples/hello/game.gba") { throw "Hello checksum mismatch" }
"examples/hello/game.gba: OK"
```

To check another example, build it and replace `hello` with its folder name.

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
