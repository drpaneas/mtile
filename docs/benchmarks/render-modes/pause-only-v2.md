# Fixed PAUSED message: ARM parity and cost

`pause(true)` enables Start pause/resume. It is **disabled by default**.
While enabled, Start freezes the game and shows the centered, spaced PAUSED message.
A second Start resumes. There is no panel, menu action or system exit.
The message uses fixed glyphs and a dark shadow, independent of project fonts.

A generated game with `_draw() cls(1)` and a saturating update counter proves
Start does not pause before any enable call. At tick 30, a dynamic boolean
expression enables it; after resume, a literal false disables it at tick 100.
There are no `btn` or time calls. Separate scene fixtures call literal true.
This covers `pause`'s literal and dynamic boolean shapes in both
renderers and AUTO.

Each renderer captures 120 stable pause frames with zero differing pixels.
A/B/D-pad do not change the paused image; Start restores the original image,
and stops pausing again after disabling. AUTO's ROM equals forced Mode 0.

| Steady pause cost | Mode 0 | Mode 4 | AUTO |
| --- | ---: | ---: | ---: |
| Samples | 120 | 120 | 120 |
| Median cycles | 422 | 430 | 422 |
| P95 cycles | 422 | 430 | 422 |
| Debug ROM bytes (fixture) | 25,172 | 25,388 | 25,172 |

Costs exclude deliberate pacing waits. One 64x64 4bpp OBJ carries the 51x8
message and shadow; all other pixels are transparent. The existing conservative
system reservation remains: one OAM entry, 64 worst-line cycles, 2 KiB of OBJ
tiles and palette indices 241..245. About 5 KiB of Context RAM holds backups
and prepared pixels. Bounded DMA captures/restores VRAM and OAM in VBlank.

[Scene captures](pause-overlay-scenes-v2.json) cover Tidewalk, Lantern Rooms
and a 127-object scene in Mode 0, Mode 4 and AUTO. Every pixel outside the
51x8 message stays unchanged, the paused scene remains stable, and Start
restores the entire original image. All paired checkpoints match. Runtime
tests also compare all borrowed tiles, OAM, palette words and display/blend
registers, and verify default-off input forwarding, opt-in, held Start across
enabling, disabling an active overlay, invalid settings, other buttons, key
suppression and custom font independence.

Saving is separate. The [three-file example](saving-example-v2.json) owns its
menus and enables pause only in gameplay. Start works normally in file/name
menus, and holding it while entering gameplay cannot immediately pause.
`save_pending()` advances whether pause is disabled, enabled or active.
A direct runtime `SaveWrite` test checks the full 150-poll reminder, successful
restart and failure preservation without any pause-result helper.

Evidence: [plain game and costs](pause-only-v2.json),
[scenes](pause-overlay-scenes-v2.json), [save API](saving-slots-v2.md).
Physical console/cartridge verification remains pending.

```sh
MTILE_SAVE_OUT=/tmp/mtile-pause-check \
  go test -tags=runtimeperfdiagnostic ./cmd/renderbench \
  -run 'Test(PauseWithout|PauseOverlayScenes|SaveExample|SaveTarget).*Diagnostic$' -count=1 -v
```
