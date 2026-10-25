# Website content review — 2026-10-13

This review covers the current static website's 71 API entries, introductory
lesson, first-game walkthrough, example index, and 15 published guide sources.
It compares instructions and API descriptions with the checked-out editor,
CLI, transpiler, runtime, asset validators, and existing tests. The website
renders repository Markdown directly; corrections belong in those sources or
`website/api.json`, not generated HTML.

## Corrections

| Material | Finding and correction | Implementation evidence |
| --- | --- | --- |
| `printh` | Produces no log or file. Removed examples that depended on logging. | `internal/transpiler/transpiler.go`, `runtime/math.go` |
| `rnd` | Table-item selection is not implemented. Documented numeric behavior and explicit index selection. | `runtime/apiv.go`, `runtime/math.go` |
| Input | Listed all ten controls and the exact held-tick repeat schedule: 1, 15, 19, 23… | `runtime/input.go` |
| Palette | Added single-bank reset and the full reset's transparency side effect. | `runtime/palette.go`, transpiler `pal` lowering |
| Tables | `count(t, value)` includes hash values; `count(t)` uses the contiguous array prefix. `del` also searches hash entries. | `runtime/table.go` |
| Conversion/math | Documented nil on invalid `tonum`/`ord`, and zero for nonpositive `sqrt`. | `runtime/string.go`, `runtime/math.go` |
| First game | Added project creation before build/edit; corrected Preferences naming. | `cmd/mtile/main.go`, `cmd/editor/preferences.go` |
| Editor reference | Removed obsolete F6/LCD/Settings controls; distinguished Map Lab number keys from workspace navigation. | `cmd/editor/game.go`, `cmd/editor/preferences.go` |
| Map edits | Resizing affects every plane; legacy shrink shortcuts crop and clamp to one tile. | `cmd/editor/map_editor.go`, editor model |
| Object artwork | Separate OBJ bank is immutable; fallback shared-sheet poses can observe `sset`. | `runtime/pose.go`, renderer contract |
| Graphics roadmap | Bounded 4bpp pose variants have shipped; general BG allocation remains separate. | pose-variant analyzer and renderer contract |
| Renderer matrix | Corrected stale exclusions of proven arithmetic/table/helper sprite IDs and clarified currently admitted clip/transparency forms. | `internal/transpiler/render_modes.go` and classification tests |
| `foreach` | Generic length checks can be hoisted for loops proven not to mutate the array. | transpiler iterator-length optimization |
| Audio editor | Transport cursor uses yellow rather than the selected channel color. | native Music Lab drawing code |
| Performance | Scoped historical benchmarks; corrected dense-map budget contradiction, overrun/pacing claims, polling versus end-to-end latency, Go versus Lua allocation, and profiling instructions. CPU load can exceed 1. | `runtime/profile.go`, benchmark fixtures and linked reports |

## Verification and limits

`TestWebsiteAPIExamples` parses and transpiles every API snippet with explicit
fixture assets, then checks generated Go syntax. The fixture supplies the
referenced pose, animation, color variant, and native-audio assets. This does
not execute the snippets or establish pixel parity or timing.

The website tests build all 101 pages and check API coverage against the public
registry, renderer classifications, local links/fragments, generated API-guide
synchronization, screenshot references, and search coverage. Runtime and
renderer tests remain the behavioral checks; link success alone proves no API
semantics.

Historical benchmark results remain dated snapshots. This review does not
rerun every benchmark or re-certify linked historical engineering reports.
Screenshots are actual editor Draw captures with project/state provenance in
`docs/media/editor/capture.json`; the Develop image deliberately shows no
running ROM. Physical-GBA audio listening/capture remains a separate pending
release gate. No new physical-hardware result is claimed by this review.

Verification completed for this review:

- Website: all six tests passed; 101 pages and 71 API entries generated.
- API snippets and exhaustive renderer classification/documentation gates passed.
- Full `internal/transpiler`, `internal/build`, and `cmd/mtile` suites passed.
  Build/CLI tests required normal TinyGo cache access after an initial sandbox denial.
- Runtime suites passed with default tags and `mtile_mode0`.
- Generated Lua API guide synchronization and `git diff --check` passed.
