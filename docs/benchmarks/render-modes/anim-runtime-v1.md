# Named animation runtime evidence

Captured on 2026-09-24 using TinyGo 0.42.0 and the repository's headless mGBA
player. Both fixtures preserve exactly **120 game frames and all 240 LCD
scans** per renderer, with zero differing pixels and no missed frames.

| Fixture | Covered behavior | Median Mode 0 / Mode 4 cycles | Maximum Mode 0 commit |
| --- | --- | ---: | ---: |
| `anim-clock` | Only `cls` and `anim` in `_draw`; no `_update`, input, counter, or `time` call. Three saved frames at 60 FPS prove automatic clock advancement. | 13,848 / 150,436 | 3,249 |
| `anim-clips` | Looping and held-final clips; default and explicit elapsed time; negative elapsed; independently restarted `time()-started` playback; dynamic and missing names; empty clips; authored and whole-pose flips; repeated helper sites; drawing inside an argument; mixed `pspr`/`spr`. | 55,690 / 225,507 | 7,571 |

The test also requires the first two captured game images to differ. This
prevents a frozen animation from passing merely because both renderers froze
identically. The largest observed commit is below the 82,544-cycle usable
VBlank window. Clip frames remain pose IDs in ROM and reuse the existing
pose compiler, packing, streaming, and draw ordering.

The adjacent `pspr-stream-anim-clock-v1.json` and
`pspr-stream-anim-clips-v1.json` record ROM hashes, toolchain, memory, timing,
and pixel parity. Reproduce in a fresh output directory:

```sh
MTILE_PSPR_OUT=/tmp/mtile-anim-native \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestPSprStreamTargetDiagnostic/(anim-clock|anim-clips)$' \
  -count=1 -v
```
