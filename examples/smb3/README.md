# Super Mario Bros. 3 · mTile

This example plays the original **Super Mario Bros. 3** through mTile's GBA player, with an option to open it inside the **Learn** workspace. The generated GBA cartridge runs the complete, unmodified NES ROM through PocketNES on the ARM7. The Lua file is a launch card for the editor; the original NES program runs the game.

## Build and play

From the mTile checkout, with Python 3.9+ and your own matching NES ROM:

```sh
python3 examples/smb3/build.py "/path/to/mario3.nes"
./examples/smb3/play
```

The build downloads the [pinned PocketNES release](https://github.com/Dwedit/PocketNES/releases/tag/1-4-2020-B) and verifies its SHA-256 before use. The NES ROM never leaves your computer. To build offline, pass the official release ZIP or its extracted `pocketnes.gba` as `--core /path/to/file`. The builder accepts the 393,232-byte NES revision identified by SHA-256 `4377a7f5e6eb50bdd2ac6f249bf1a7085500aca8eb41f38545c3a2731c51a579`.

`play` opens the game in its own window using mTile's `mtile-player`. Set `MTILE_PLAYER` to the full path of the helper if the launcher cannot find it. Press **Esc** to close the game. The same cartridge also opens directly in a GBA emulator.

To inspect the cartridge in the mTile editor, run `./examples/smb3/play --learn`. This requires a checkout or editor that supports the Learn workspace. Set `MTILE_EDITOR` to an already built Learn editor if desired; otherwise the script runs the editor from source.

In **Learn**, click the original preview to play and give it keyboard focus (or press **⌘P** on macOS, **Ctrl+P** elsewhere). In either window, wait for the title, then press **Enter** to open the player menu and **Enter** to choose 1 Player. After the World 1 introduction clears, press **Right**, then **Up** to reach **1**, and **Z** to enter.

| Keyboard | Original control |
| --- | --- |
| Arrow keys | D-pad |
| Z | A (jump, confirm) |
| X | B (run, action) |
| Enter | Start |
| Backspace | Select |

## Fidelity and build boundary

The build writes the verified PocketNES GBA executable, one 48-byte menu entry, and every byte of the original iNES file. It uses the [upstream pack format](https://github.com/Dwedit/PocketNES/blob/f7e7cbbda8b605b940979155fa618de514ec84b4/src/rommenu.c) so the sole game starts automatically. It does not extract or redistribute a separate copy of the game's maps, sprites, audio, or code. Keep the generated `game.gba` private: it contains your NES ROM.

Mario's movement, collisions, enemies, levels and progression are driven by the original code. The GBA display is 240×160; PocketNES scales the NES picture and maps sound and timing to the GBA. Picture, audio and timing are therefore not bit-identical to the desktop NES reference. The editor's local controls cover one GBA keypad. The original game is not editable as mTile Lua.

Expected SHA-256 of the generated cartridge: `8df99dded49916e382814272833d947c513040bd244084d5f3193033255491ff`.


## Verify the replay

```sh
python3 examples/smb3/verify.py examples/smb3/game.gba build/player/mtile-player \
  --output /tmp/smb3-verification
```

Use a current `mtile-player` with memory-read support; replace the helper path above if yours is installed elsewhere. The optional output directory receives local PNG checkpoints and a JSON report.

The replay drives the cartridge through the same mTile player used by Learn. It reads the NES game state from inside the PocketNES session: title, map, World 1-1, running and jumping, the goal card, return to the map and the grounded slope in World 1-2. Running the same input twice also checks that the output is repeatable.

These are sampled milestones, not an exhaustive certification of every world or emulator edge case. The compatibility cartridge and desktop reference return to the map with the same goal card. Three sampled grounded slope positions in World 1-2 also match when the starts of the replays are aligned. They already differ in Mario's coordinates on one sampled goal frame: (2684, 342) in the GBA run and (2685, 351) in the NES run. The replay input is timed for the GBA version.
