# Reproduce the tile experiments

These opt-in experiments do not alter the installed runtime. Keep generated
artifacts outside the repository. The player must already be built, and the
normal Go/TinyGo toolchain must be available.

```sh
python3 scripts/discord-hardware/fixtures.py --out /tmp/mtile-discord-tiles
MTILE_DISCORD_TRIAL_DIR=/tmp/mtile-discord-tiles go test -tags=discordtrial ./cmd/renderbench -run TestDiscordSharedAnimationTrial -count=1 -v
```

The native test captures 120 synchronized frames from the first logical game
frame. It compares the sprite and shared-BG versions, then verifies that the
independent-phase counterexample differs. It writes JSON, ROMs, and PNGs into
the fixture directory. The JSON records frame cycles, ROM/EWRAM, OAM/scanline
proofs, and exact pixel comparisons.

For the rejected 4bpp experiment, make an isolated copy of the repository and
apply `fourbpp-candidate.patch` there. Its environment flag enables only a
restricted immutable sheet with indices 0..15, no packed objects, and no text.
The patch is research code, not a supported renderer option. Keep all outputs
in separate artifact directories:

```sh
# In the isolated patched copy:
go run ./cmd/renderbench -project /tmp/mtile-discord-tiles/fourbpp -out /tmp/mtile-fourbpp-baseline -gate parity -warmup 20 -samples 120
MTILE_TRIAL_4BPP=1 go run ./cmd/renderbench -project /tmp/mtile-discord-tiles/fourbpp -out /tmp/mtile-fourbpp-candidate -gate parity -warmup 20 -samples 120
MTILE_TRIAL_4BPP=1 go run ./cmd/renderbench -project /tmp/mtile-discord-tiles/fourbpp -out /tmp/mtile-fourbpp-first -gate parity -warmup 0 -samples 120
```

This candidate packs 64-byte staged tiles into 32-byte 4bpp tiles at startup,
then uses 4bpp BG/OBJ attributes. It deliberately preserves the original asset
representation and logical palette indices. This makes exactness tractable but
adds cold CPU work. Build-time compressed asset storage is a separate,
unimplemented approach; do not interpret this rejection as disproving it.

Palette-zero and capacity guards can be rerun independently:

```sh
go test ./internal/transpiler -run 'TestMode0ObjectBudget|Test.*Palette|Test.*Palt|Test.*Text' -count=1
cd runtime
go test -tags=mtile_mode0,mtile_mode0_obj,mtile_mode0_objpack,mtile_mode0_text ./... -run 'TestMode0.*(Palette|Zero|OAM|Budget|Overflow)' -count=1
go test -tags=mtile_tilecanvas ./... -run 'TestObjectCanvasPackingExactColorsHolesAndOpaqueZero|TestObjectCanvasRejectsCapacityBeforeDestinationMutation' -count=1
```

## Headless audio reference check

After building the six-ROM bundle with `scripts/audio/build_hardware_signoff.sh`,
run the 42 PSG reference cases using the same pinned native library as the
player (no GUI or system audio needed):

```sh
python3 scripts/discord-hardware/audio-check.py --bundle /path/to/bundle --output-dir /tmp/mtile-audio-check
```

The output directory must be new. WAVs, per-case analysis logs, and source ROM /
library hashes are retained there. This checks emulator PCM behavior, not
physical-device sound or power.

## Native metatile and constant trials

```sh
python3 scripts/discord-hardware/metatiles.py --out /tmp/mtile-metatiles
python3 scripts/discord-hardware/constants/check.py --output-dir /tmp/mtile-constants
```

The metatile cartridge compares exact 2×2 dictionary expansion with flat-map
copying, including a repeated-map saving and an all-unique storage regression.
It is an opt-in diagnostic under `runtime/cmd/metatiletrial`, not a map-format
change. The constant checker compares actual ARM literal/immediate sequences
in ROM and IWRAM with value, flags, register and stack checks.
