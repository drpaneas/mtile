# Super Mario Bros. 3 · native mTile

An editable mTile project for the first two World 1 courses. The project builds a Game Boy Advance ROM with mTile and TinyGo. The GBA runs the compiled Lua game logic, draws backgrounds and sprites using the Mode 0 hardware, and plays the soundtrack through the four PSG channels.

The title, World 1 map, courses, sprites, HUD and sound data are generated locally from the matching NTSC NES cartridge. The importer uses Python 3.10 or newer and works offline. The generated assets and GBA ROM stay local.

## Open and build

From the mTile checkout, import the assets once:

```sh
python3 examples/smb3/build.py "/path/to/mario3.nes" --assets-only
go run ./cmd/mtile edit examples/smb3
```

Choose **Develop** and press **Run** to build and play the game in the editor. To build the same GBA ROM from the terminal:

```sh
go run ./cmd/mtile build examples/smb3
```

The output is `examples/smb3/game.gba`. Omitting `--assets-only` from the import command runs the same build after importing.

At the title, press **Start**. On the map, go **Right**, then **Up**, then press **A** to enter 1-1. Clear the course, then go **Right** twice and press **A** to enter 1-2.

| GBA controls | Editor keyboard |
| --- | --- |
| D-pad | Arrow keys |
| A, jump or enter | Z |
| B, run or tail attack | X |
| Start, menu or pause | Enter |

## Source

- `game.lua` handles the title, world map, stage progression, camera and HUD.
- `player.lua` handles the fixed-point simulation, controller edges, collision probes, poses and power states.
- `terrain.lua` uses the original 16-pixel metatiles for collision and updates the corresponding 8-pixel display tiles.
- `actors.lua` handles course objects, enemy contact and powerups.
- `tools/` imports ROM data before compilation. The course and sound extractors evaluate isolated NES data-loading routines locally during import. The GBA cartridge executes the native mTile game.

The two main-course terrain grids, object placements and palettes were checked against the NES reference. Separate movement tests matched 360 reference ticks for normal walking, running, reversing and jumping. The current project covers single-player World 1-1 and the main area of 1-2. Enemy timing, some interactive tiles and the soundtrack's percussion differ. The other courses, pipe bonus areas and two-player mode remain unimplemented. The GBA viewport is 240×160. The game schedules two simulation steps for each keypad sample and presentation. The measured 1-1 replay averaged about 56 simulation updates per second, below the nominal rate.

## Replay the verification

Build with `go run ./cmd/mtile build -keep-work examples/smb3`. The output prints a work directory containing the matching `game.elf`. With the native player helper and the ARM/LLVM symbol tools on your path:

```sh
python3 examples/smb3/tools/verify_native.py examples/smb3/game.gba \
  /path/to/workdir/game.elf build/player/mtile-player --output /tmp/smb3-verification
```

The replay uses GBA buttons to finish 1-1, receive a card and move in 1-2. It reads those checkpoints from the running game's memory, captures six screenshots and checks that the cartridge produces PCM.

ROM labels and behavior were cross-checked against [Southbird's SMB3 disassembly](http://www.sonicepoch.com/sm3mix/).
