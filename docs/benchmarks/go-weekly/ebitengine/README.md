# Ebitengine 2.10.2 / Oto 3.5.0 experiment — 2026-09-19

**Retain.** The real editor gains verified VM-driven UI testing and builds for
Linux/amd64 with `CGO_ENABLED=0`. Sprite, Map, and Code captures match the old
renderer exactly at the decoded RGBA level. No material rendering slowdown
reproduced in the repeated comparison. Production MIDI still uses cgo and the
release bundle still builds a separate native C mGBA helper.

Sources: [Go Weekly #617](https://golangweekly.com/issues/617),
[Ebitengine release notes](https://ebitengine.org/en/documents/2.10.html),
[Oto 3.5.0](https://github.com/ebitengine/oto/releases/tag/v3.5.0).

## Measurements

Apple M2 Pro, macOS/arm64, Go 1.27.1. The same opt-in test harness was compiled
against 2.9.9/3.4.0 and 2.10.2/3.5.0. Each timed draw runs on a distinct engine
frame, after 20 warm-up frames, into a 1200×800 image with GPU readback. VSync
is disabled and TPS follows FPS. Native presentation still paces the outer
benchmark near 60 Hz; `render-ns/op` measures the drawing/readback work inside
the frame and is reported separately.

| Workload | Baseline median | Candidate median | Result |
| --- | ---: | ---: | --- |
| Sprite rendering, eight alternating process pairs | 6.875 ms | 6.442 ms | −6.29%, p=.010 |
| Map rendering, ten samples | 15.87 ms | 14.13 ms | −10.98%, p<.001 |
| Code rendering, ten samples | 5.786 ms | 5.001 ms | −13.56%, p=.019 |
| Four-channel synthesis, six samples | 25.77 µs | 25.62 µs | No significant change |
| Live stream, six samples | 48.02 µs | 47.77 µs | No significant change |

The first non-interleaved Sprite pass was 11.2% slower. Eight baseline/candidate
pairs, alternating execution order, did not reproduce that regression. Both
passes remain in the evidence; these noisy graphics timings do not justify a
universal speedup claim. Sprite allocation count dropped 11.3%. Code allocation
count increased approximately 711/frame (58%), while bytes/frame increased only
1.5% and observed rendering became faster. Both synthesis controls remain at
zero allocations; they do **not** prove Oto device latency or MIDI responsiveness.

See [initial comparison](render-comparison.txt),
[alternating comparison](alternating-comparison.txt),
[audio controls](audio-comparison.txt), and [machine-readable results](result.json).
The discarded single-frame/presentation pilot runs are excluded from this data.

## Correctness and capability

- Exact decoded RGBA parity for [Sprite](Sprites.png), [Map](Map.png), and
  [Code](Code.png); dimensions and pixel hashes are in `result.json`.
- The real editor VM smoke advanced 55 ticks, navigated using keyboard and
  mouse, saved an injected edit, restored original source bytes through undo,
  saved again, and exited cleanly. See [VM result](vm-result.json) and
  [restored editor](vm-restored.png).
- The VM driver uses a copied fixture and an explicit verified nonexistent
  TinyGo path, preventing Develop's automatic preview from starting a compiler.
  Its scope is editing; it does not exercise cartridge playback, audio, or MIDI.
- Editor, emulator, performance, MIDI, and PSG tests pass with `-race -cpu=1,4`.
  See [race log](candidate-race-tests.log). Required repository gates were run
  separately by the coordinating task.
- The candidate editor cross-builds for Linux/amd64 with cgo disabled; the
  baseline fails in Oto/GLFW's unavailable non-cgo implementations.

## Reproduction

The opt-in commands and benchmark interpretation are documented in
[the experiment tools](../../../../scripts/go-weekly/ebitengine/README.md).
Archived `baseline.mod`/`.sum` and `candidate.mod`/`.sum` preserve the dependency
graphs. Run from the repository root so their local runtime replacement resolves:

```sh
go test -modfile=docs/benchmarks/go-weekly/ebitengine/baseline.mod \
  -tags mtile_editor_bench ./cmd/editor -run '^$' \
  -bench '^BenchmarkEditorRender$' -benchmem -benchtime=300ms -count=10
go test -modfile=docs/benchmarks/go-weekly/ebitengine/candidate.mod \
  -tags mtile_editor_bench ./cmd/editor -run '^$' \
  -bench '^BenchmarkEditorRender$' -benchmem -benchtime=300ms -count=10
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -o /tmp/mtile-editor-linux-amd64 ./cmd/editor
```

The Linux cross-build proves compilation only. Linux runtime libraries remain
required. Physical audio/MIDI latency and long-running minimized-window memory
behavior were not measured.
