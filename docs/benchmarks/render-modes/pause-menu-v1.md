# Shared pause menu: parity and cost

The common Start menu is exercised by the dedicated Save Game example and
a second generated cartridge with only `_draw() cls(1)`: no game input,
clock, save code, or registration. Both renderers display the same fixed
sprite overlay over the frozen game. The registered example additionally runs in AUTO, selecting
Mode 0 with identical ROM bytes to forced Mode 0.

Each renderer captured 120 stable pause frames with zero differing pixels.
The interaction checkpoints also match: opening the menu, freezing game
updates, saving once while A is held, blocking resuming during the five-second
wait, clearing that reminder, restoring game input, loading saved progress,
restoring after machine reset, and ending the session. The plain cartridge
verifies greyed-out Save/Load feedback and exact background restoration.

| Steady menu measurement | Mode 0 | Mode 4 | AUTO |
| --- | ---: | ---: | ---: |
| Samples | 120 | 120 | 120 |
| Median cycles | 724 | 642 | 724 |
| P95 cycles | 724 | 642 | 724 |
| ROM bytes (complete example, debug build) | 44,992 | 48,040 | 44,992 |

Cycle counts include system UI presentation and input polling, excluding
the deliberate pacing wait. The fixed overlay uses one 64x64 4bpp OBJ. The
compact panel is 64x48;
feedback can expand it to 64x64. It is rasterized only when menu state changes,
before the presentation wait. Bounded DMA captures/restores VRAM and OAM and
commits the prepared pixels in VBlank; the remaining window includes headroom
for a native-audio IRQ. The game framebuffer and BG planes are untouched.
Mode 0 reserves one OAM entry, 64 scanline cycles, 2 KiB of OBJ tiles and
five physical OBJ palette entries. Approximately 5 KiB of fixed Context RAM
holds the UI and backups. These resources are included in the compiler's
normal memory and eligibility checks. Runtime tests compare every borrowed
tile, OAM and palette word plus display/blend registers after resuming, and
check key swallowing, failure messages, save-wait restart/expiry, unavailable
actions, and permanent Exit.

Additional [scene evidence](pause-overlay-scenes-v1.json) checks Tidewalk,
Lantern Rooms and a 127-object fixture in Mode 0, Mode 4 and AUTO. Every pixel
outside the system OBJ matches the unpaused scene; the paused image stays
stable, and a second Start restores the entire original image exactly.
All checkpoint images match across renderers. Each scene's AUTO ROM hash
matches its forced Mode 0 hash. These scene captures use 240 LCD scans of
cold-start warmup so the larger Mode 4 scene has finished initializing.

The `pause_menu` registration is renderer-independent, but its callbacks are
included in renderer analysis. Analysis considers normal update, save, and
load as alternative transitions, including callbacks before the first game
update. A callback cannot hide a graphics mutation from AUTO's proofs.
The synthetic analysis transitions are never executed in the generated game.

Recorded evidence: [menu timing and hashes](pause-menu-v1.json),
[interaction checkpoints](saving-example-v1.json),
[game without input/save code](pause-no-input-v1.json), and
[save workload timings](saving-v1.json). All are emulator evidence;
physical console/cartridge verification remains pending.

Reproduce from the repository root with TinyGo and the native player built:

```sh
MTILE_SAVE_OUT=/tmp/mtile-pause-check \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run 'Test(PauseWithout|PauseOverlayScenes|SaveExample|SaveTarget).*Diagnostic$' -count=1 -v
```

The diagnostic retains the ROMs and screenshots for each backend. The
ordinary test gate requires the recorded parity, interaction and timing
reports; it does not label physical hardware verified.
