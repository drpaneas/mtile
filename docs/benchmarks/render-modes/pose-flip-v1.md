# Automatic pose and animation flips

Native mGBA verification of in-place `pspr` flips and stable clip-wide `anim`
flips. The deterministic fixture keeps draw coordinates fixed and exercises
negative part offsets, authored part flips, all four X/Y orientations, unequal
frame sizes, and one pose shared by two clips with different union bounds.
No user artwork or game source is changed by the diagnostic.

## Pixel and position proof

All **120 synchronized game frames** match exactly between Mode 0 and Mode 4.
An independent oracle also matches every pixel in both renderers, comparing
RGB5 colors against raw source pixels reflected within manually specified
bounding rectangles. It does not read generated flip offsets. Recorded
expected and actual bounding boxes agree for all three actors in every frame.

All **240 sampled Mode 0 LCD scans**, including the first after each present,
match the corresponding Mode 4 frame and the independent oracle. There are
no missed game frames. [Raw parity report](pose-flip-v1/parity/report.json)
and [per-frame bounds](pose-flip-v1/parity/frames.json).

| Renderer | Median cycles | p95 / max cycles | p95 of 30 Hz budget | Max commit cycles | Static EWRAM | Static IWRAM | Debug ROM |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Mode 0 | 19,622 | 28,369 | 5.05% | 5,122 | 8,604 B | 9,980 B | 18,712 B |
| Mode 4 | 172,339 | 190,041 | 33.83% | 77,004 | 40,400 B | 13,388 B | 14,400 B |

The runtime budget is 561,792 cycles per 30 Hz game frame. Mode 0's maximum
commit uses 6.21% of the 82,544-cycle usable VBlank window. These are emulator
measurements of generated ARM code, not a physical-hardware timing claim.

## Cached-pose cost

A second fixture draws one constant pose. The **same ROM** runs twice: A up
keeps both flip flags false; A held cycles through none, X, Y and XY. Both runs
sample the same 120 game frames after two warm-up frames.

| Input | Median cycles | p95 / max cycles | Commit cycles | OBJ pixel upload per sampled present |
| --- | ---: | ---: | ---: | ---: |
| No flip | 9,504 | 9,504 | 2,868 | 0 B |
| Alternating flips | 9,585 | 9,616 | 2,868 | 0 B |

Flipping adds **81 median cycles** and **112 p95 cycles** in this small scene.
The p95 increment is 0.020% of the 30 Hz frame budget. The cached pose remains
resident; its tile pixels are not uploaded again. Both runs retain the same
17,668-byte ROM, 7,964 bytes of static EWRAM and 9,980 bytes of static IWRAM.
[Control report](pose-flip-v1/flip-cache/report.json) and
[upload telemetry](pose-flip-v1/flip-cache/frames.json).

This comparison isolates active flip branches; it is not a measurement against
an older compiler. Debug cycle counters exclude telemetry publication. Both
runs maintain two LCD scans per game frame after warm-up.

The linked tables contain three 16-byte pose records and two 28-byte clip
records. Their new X/Y flip-offset fields account for **40 additional ROM
bytes**: eight per pose or clip, compared with the former 8-byte and 20-byte
record layouts. The bounds are computed during the build; no new mutable
bounds table or per-frame bounds scan is needed.

## Reproduction

Build the native player first, then use a fresh output directory:

```sh
MTILE_POSE_FLIP_OUT=/private/tmp/mtile-pose-flip-check \
CGO_ENABLED=0 GOCACHE=/private/tmp/mtile-go-cache \
go test -tags runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestPoseFlipTargetDiagnostic$' -count=1 -v
```

The diagnostic preserves ROMs, ELF/maps, generated Go, fixture inputs, cycle
samples and representative images. [Provenance](pose-flip-v1/provenance.json)
records the final capture path and source hashes; each case report includes
input-project, player and ROM hashes.
