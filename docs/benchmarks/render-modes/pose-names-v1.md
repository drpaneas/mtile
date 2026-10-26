# Literal pose names

Verified on 2026-10-26 with TinyGo 0.42.0, Go 1.27.1, and the native mGBA
player on macOS arm64. These are emulator measurements of generated ARM code.
The [raw report](pose-names-v1/report.json) records toolchain and player
provenance, ROM/Go/frame hashes, source revision, individual cycle samples,
memory usage, and zero missed frames for all four builds.

The [named fixture](pose-names-v1/fixture-named.lua) changes only
`pspr(0, ...)` in the [numeric control](pose-names-v1/fixture-numeric.lua) to
`pspr("shared", ...)`. It uses negative part offsets, authored part flips,
all four call-level flip orientations, and two animation clips sharing that
pose with unequal union bounds. Saved [poses](pose-names-v1/fixture-poses.json),
[clips](pose-names-v1/fixture-animation.json), and
[source pixels](pose-names-v1/fixture-objsheet.json) preserve the inputs.

## Pixel and runtime cost proof

All **120 synchronized game frames**, numbered 3 through 122, match exactly
between Mode 0 and Mode 4. An independent oracle computed from source pixels
and authored bounds also matches both renderers. All **240 sampled Mode 0
LCD scans** match the corresponding Mode 4 frame.

Within each renderer, named and numeric builds produce **identical generated
Go and byte-identical native ROMs**. Every cycle sample and frame hash is
identical. Thus the literal-name form adds zero runtime lookup, string
storage, ROM bytes, or RAM bytes relative to its numeric control.

| Renderer and form | Median frame cycles | p95 / max cycles | ROM | Static EWRAM | Static IWRAM |
| --- | ---: | ---: | ---: | ---: | ---: |
| Mode 0 numeric | 20,585 | 29,300 / 29,300 | 19,240 B | 8,728 B | 9,980 B |
| Mode 0 named | 20,585 | 29,300 / 29,300 | 19,240 B | 8,728 B | 9,980 B |
| Mode 4 numeric | 75,873 | 103,817 / 103,817 | 15,112 B | 2,140 B | 14,332 B |
| Mode 4 named | 75,873 | 103,817 / 103,817 | 15,112 B | 2,140 B | 14,332 B |

Mode 0's maximum commit is 5,160 cycles, or 6.25% of the 82,544-cycle
usable VBlank window. Its p95 frame consumes 5.22% of the 561,792-cycle
30 Hz budget. No build links a Lua heap.

## Accepted shapes and limits

`pspr("shared", x, y [, flip_x [, flip_y]])` accepts a case-sensitive literal
name, optionally parenthesized. Parser and transpiler tests cover all optional
flip arities, helper/loop allocation, expanded arguments and evaluation order,
color variants, deletion/reordering, and renderer selection. Every such call
lowers to its canonical numeric index before renderer analysis and generation;
the same finite draw, working-set, layering, OAM, scanline, palette, and transfer
rules apply. Unused oversized poses do not enter a known named subset. An
oversized selected pose still fails forced Mode 0 and makes AUTO select Mode 4
with a source-positioned diagnostic.

Unknown literal names fail all renderers with `PSPR_UNKNOWN_POSE` and a close
spelling suggestion when available. Numeric calls preserve their existing
signed 16:16 conversion and invalid-ID no-ops; clearly out-of-range numeric
literals emit nonfatal `PSPR_ID_OUT_OF_RANGE` warnings. Names are resolved
again on each build, so renaming or deleting the referenced name fails instead
of selecting a different pose. Dynamic string lookup and generated symbols
are outside this change.

## Reproduction

Use a fresh output directory and an available native player:

```sh
MTILE_POSE_NAMES_OUT=/private/tmp/mtile-pose-names-check \
GOCACHE=/private/tmp/mtile-go-cache CGO_ENABLED=0 \
go test -tags runtimeperfdiagnostic ./cmd/renderbench \
  -run '^TestNamedPoseTargetDiagnostic$' -count=1 -v
```

The diagnostic preserves its four fixture projects, ROMs, and report in that
directory. TinyGo needs access to its compiler cache. Default tests also check
the recorded report, so missing parity/cost evidence fails the local gate.
