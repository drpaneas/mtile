# Immutable resident pose evidence

Captured on 2026-09-27 using TinyGo 0.42.0 and the repository's headless mGBA
player. The adjacent `pose-resident-v1/` JSON records preserve source-project
and ROM hashes, toolchain details, game-frame timing samples, and first/all
LCD-scan parity. Reproduce from the repository root with a fresh output path:

```sh
MTILE_PSPR_OUT=/tmp/mtile-resident-evidence \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestPSprStreamTargetDiagnostic/(full-window-four-bg|full-window-audio|full-window-resident-four-bg-audio|full-window-overflow-four-bg|full-window-overflow-audio)$' \
  -count=1 -v
```

These fixtures exceed the existing per-occurrence pose-streaming transfer
budget. The compiler can now accept them when all reachable immutable object
layouts fit in a shared resident OBJ bank. Every packed object's contiguous
row-major pixel run is preserved exactly; equal complete runs and subruns may
share storage. The bank uploads while the LCD is force-blanked, before visible
rendering, and requires no pose staging buffer or later pose-pixel transfers.
Already accepted streaming scenes retain their existing allocation.

| Accepted fixture | OAM entries | Resident tiles | Maximum visible commit | Median Mode 0 / Mode 4 frame cycles |
| --- | ---: | ---: | ---: | ---: |
| Eight 64×64 poses with four changing BG maps | 8 | 128 | 36,360 | 50,578 / 1,565,107 |
| Eight 64×64 poses with dense native audio | 8 | 128 | 30,431 | 12,145 / 1,782,553 |
| 128 poses, four changing BG maps, dense native audio | 128 | 8 | 63,314 | 257,509 / 2,460,392 |

All three accepted cases match Mode 4 exactly across **360 game frames** and
**761 LCD scans**, including the first scan after presentation. Every
visible commit remains below the usable **82,544-cycle** VBlank budget. This
includes simultaneous changes to all four maps and the full 128-entry OAM
table while native audio runs. Cold resident-bank uploads happen outside that
visible window. The JSON distinguishes frame-rate measurements from pixel and
presentation safety; the latter do not imply an unlimited game CPU budget.
The three Mode 0 captures respectively recorded 0, 1, and 40 missed frames;
the dense-audio stress test demonstrates safe presentation rather than a
claim of perfect 30 Hz pacing under that combined load.

Two further fixtures use ten distinct 64×64 layouts whose resident bank cannot
fit. Forced Mode 0 retains the original source-positioned transfer-budget
rejection for four changing BGs or dense native audio. AUTO selects Mode 4 and
matches forced Mode 4 on another **240 game frames**. Thus residency does not
remove the streaming proof or accept an oversized library by truncation.

The host regression suite independently verifies the 512/513-tile boundary,
exact pixel-run sharing with aliases and different layouts, unreachable-pose
exclusion, shared-sheet mutation fallback, legacy sprite/text/packing
reservations, finite-call and OAM limits, forced/AUTO renderer selection, and
pixel parity for packed and overlapping objects with authored/global flips,
camera movement and screen-edge clipping. Runtime tests also verify that
resident draws schedule and upload zero OBJ pixel bytes after initialization.

This is an implementation of the existing conditional `pspr`/`anim` contract.
No Lua API, animation clock, pose bounds, palette behavior, clipping, or draw
order changed. Programs using `sset` retain streaming;
native-audio and palette restrictions remain in force.
