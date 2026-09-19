# Experiment 4: optional ordered sprite import

**Retain**, 2026-09-19. During clipboard-image placement, **D** switches between
nearest color (the default) and ordered dithering. Moving, switching, or
canceling a preview does not change the project. A click commits one undo step.
Undo/redo cancels an active preview so its indices cannot outlive their palette.

The new quantizer considers the nearest paint color and the best projected
alternative, with two linear palette scans per visible pixel. Mixtures use
sixteenths on a 4x4 Bayer pattern. Exact RGB15 colors, transparency, lowest-index
ties, and the exclusion of index zero from visible pixels remain unchanged.
Source-local phase repeats at every 8x8 tile; no error propagates between tiles.
The nearest quantizer is byte-for-byte unchanged. The imported palette is
snapshotted, and the ordered result is computed once, on demand.

## Predeclared gates and results

Measured on Apple M2 Pro, darwin/arm64, Go 1.27.1. See `result.json` and raw logs.

| Gate | Result |
| --- | --- |
| Median 4x4 averaged RGB15 error reduction at least 15% | **75.98%** across 12 fixed cases; individual reductions 38.4–93.6% |
| Exact-palette art, alpha-zero mask, visible nonzero indices | Passed, including nonempty Loop/Testio assets, alpha 1/127/255, and duplicate palette colors |
| Repeated 8x8 identity and stable image-local phase | Passed, including nonzero image bounds |
| First full UI toggle p95 below 50ms at 128x128 | **1.649ms**, PICO-8 16 colors; **15.330ms**, Aurora 256 colors; 40 samples each after two warmups |
| High-variation palette-search control | Synthetic 16/256-color computation p95 **1.614/28.119ms**, 40 samples each |
| Cached full UI toggle, including status and redraw request | **0 allocations** |
| Default nearest import unchanged | Exact output checks and unchanged source; median 448.9→447.2µs (16 colors), 3.990→3.986ms (256 colors) |
| Preview/cancel isolation; one commit undo; palette undo safety | Passed |
| Actual editor rendering | Captured and visually inspected both modes |

Benchmarks use maximum-size, deterministic, high-variation input, five 300ms
repetitions per case. Ordered medians were 1.386ms (16 colors) and 25.39ms
(256 colors). Both quantizers allocate approximately 192KiB in 16,385
allocations per import on this host; this includes existing image/color
interface conversion costs. The ordered output remains cached while placing
the image. Benchstat found no nearest timing difference (p=.548/.841), although
five repetitions are below its six-sample minimum for a 95% confidence interval.
These are import costs, not per-frame drawing costs.

## Quality and tile-reuse tradeoffs

The fixed corpus is horizontal/vertical gray ramps, horizontal/vertical color
ramps, a diagonal color ramp, and an orange shaded sphere. Each is tested with
the existing PICO-8 16-color and Aurora 256-color palettes. See `quality.json`
for all raw, 2x2, and 4x4 errors and unique tile counts. Error uses encoded RGB15
components and averages visible pixels in each box; it is not a perceptual
metric or a universal visual-quality guarantee.

Nearest color minimizes individual-pixel error. Dithering improves regional
means while introducing colored speckling and increasing raw pixel error:

| Example | Raw RGB15 MSE, nearest→ordered | 4x4 error reduction | Unique 8x8 tiles |
| --- | --- | --- | --- |
| PICO-8 shaded sphere | 15.70→33.57 | 46.3% | 96→160 |
| Aurora diagonal color ramp | 3.46→13.21 | 92.3% | 20→31 |

The two comparison PNGs show **source / nearest / ordered**, each enlarged 3x
with nearest-neighbor scaling. Editor PNGs show the actual temporary project
and placement overlay, including the mode and D hint. The visible speckling is
a reason to keep the feature optional and off by default.

Unique 8x8 tiles measure reuse potential only. The existing asset format retains
fixed sheet slots; these results do **not** show ROM deduplication or cartridge
size savings. Repeated identical source tiles still produce identical output.

## Reproduce

From the repository root, using a writable output directory:

```sh
mkdir -p /tmp/mtile-ordered-report
MTILE_DITHER_REPORT=/tmp/mtile-ordered-report go test ./internal/editor -run TestOrdered -count=1 -v
MTILE_DITHER_REPORT=/tmp/mtile-ordered-report go test ./cmd/editor -run 'TestSpritePasteDither|TestUndoPaletteChangeCancelsSpritePastePreview' -count=1 -v
go test ./internal/editor -run '^$' -bench '^BenchmarkImageImport(Nearest|Ordered)$' -benchtime=300ms -count=5
MTILE_DITHER_REPORT=/tmp/mtile-ordered-report go test -tags mtile_editor_bench ./cmd/editor -run '^TestSpritePasteDitherRender$' -count=1 -v
go test ./internal/editor ./cmd/editor ./internal/build ./cmd/mtile
go test -race ./internal/editor ./cmd/editor -run 'TestOrderedImport|TestSpritePasteDither|TestUndoPaletteChangeCancelsSpritePastePreview|TestSpriteImagePastePlacement' -count=1
cd runtime
go test ./...
```

The opt-in render test requires a native graphics session and uses the existing
Ebitengine benchmark loop. It imports an in-memory image into a temporary
project without reading or writing the system clipboard. The full transpiler
gate from experiment 2 remains applicable: this experiment changes no Lua API,
transpiler, or cartridge runtime code. Full temporary evidence and baseline
file copies are under `/tmp/mtile-go-weekly-loop/ordered-dither`.
