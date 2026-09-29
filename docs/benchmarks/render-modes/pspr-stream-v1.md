# Pose streaming capacity and presentation evidence

Captured on 2026-09-23 using TinyGo 0.42.0 and the repository's headless mGBA
player. The adjacent `pspr-stream-*-v1.json` files record ROM hashes, project
hashes, toolchain details, frame times, memory use, and parity results.

Reproduce all ten cases from the repository root:

```sh
MTILE_PSPR_OUT=/tmp/mtile-pspr-evidence \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestPSprStreamTargetDiagnostic$' -count=1 -v
```

Use a fresh output directory. The test generates each project, builds both
renderers, compares 120 matching game frames, and separately checks every LCD
scan against Mode 4's completed image for that same game frame. Checking the
first scan matters: ordinary screenshot parity passed in earlier unsafe
experiments whose uploads continued beyond VBlank.

| Accepted case | OAM entries | Maximum commit cycles | Median Mode 0 / Mode 4 frame cycles | Mode 0 EWRAM gain |
|---|---:|---:|---:|---:|
| Fighters, mixed drawing and mutations | 21 | 21,025 | 29,093 / 632,007 | 14,288 B |
| 1,000 animation frames, 65,536-cell bank | 2 | 2,908 | 8,900 / 146,130 | 32,628 B |
| Mutable source sheet | 3 | 4,505 | 26,843 / 168,521 | 31,180 B |
| 512 scattered tiles, eight poses | 8 | 68,901 | 422,209 / 1,158,155 | -12 B |
| 512 tiles with OAM compaction | 64 | 73,093 | 527,675 / 1,174,467 | -16 B |
| 512 tiles, 128 poses | 128 | 68,179 | 628,513 / 1,200,251 | -16 B |
| 320 tiles, BG, four tile edits and palette change | 5 | 64,768 | 312,536 / 1,538,547 | 7,124 B |
| 352 tiles, BG, legacy sprite cache and palette change | 8 | 76,025 | 350,419 / 1,688,087 | 50,656 B |

All eight accepted cases have zero differing pixels across **960 game frames
and 2,040 LCD scans**, including the first scan after each presentation. Each
visible commit fits the usable **82,544-cycle** VBlank budget. The 128-pose
stress case exceeds the game's 561,792-cycle CPU frame budget despite safe
presentation; its report preserves the missed-frame count. These measurements
establish exact rendering and safe transfers, not an unlimited frame rate.

The fighter fixture includes two packed 64×64 characters, additional effects,
overlapping parts with authored flips, global flips, camera movement, mixed
`spr`/`sspr`/`pspr`, text, changing background pixels, repeated helpers,
conditional draws, a drawing helper inside an argument expression, and the
fixed-point literal `65536` wrapping to pose zero. The large-bank fixture
addresses tile 65,535 and selects from 1,000 poses dynamically. The full-window
fixtures deliberately shuffle source tile IDs so their transfer cost cannot
depend on contiguous source art.

The combined-budget case changes its BG, four source tiles and palette every
frame. The legacy-cache case first draws its two packed 32×32 sheet sprites on
frame four, inside the measurement window, while streaming 352 pose tiles and
changing its BG and palette.

At the time of this capture, two additional cases were rejected by forced Mode 0: 512 streamed
tiles combined with four changing BGs, and 512 tiles combined with dense native
audio. AUTO selects Mode 4 and matches forced Mode 4 on another **240 game
frames**. Earlier native measurements of these unsupported combinations took
104,172 and 118,120 commit cycles respectively, beyond usable VBlank. Their
fallback reports preserve the source-positioned diagnostics.

The later [resident-pose optimization](pose-resident-v1.md) admits these two
small immutable libraries without per-frame streaming. The current diagnostic
also includes larger `full-window-overflow-four-bg` and
`full-window-overflow-audio` libraries to retain the same rejection and AUTO
fallback checks when neither residency nor streaming fits.

Pose pixels are prepared before VBlank in a buffer sized to the compiler's
actual tile allocation. Presentation uses one contiguous DMA. This keeps a
small animation inexpensive in EWRAM, while the full 512-tile workload needs a
32 KiB staging buffer. Preparation and transfer-state bookkeeping occur before
the visible commit, including when all 128 pose sites are active.
