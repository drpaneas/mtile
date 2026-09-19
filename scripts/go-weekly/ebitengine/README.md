# Desktop renderer and VM experiments

These opt-in tools exercise the desktop editor. They do not measure GBA cycles
or physical audio/MIDI latency, and do not change normal editor startup.

## Rendering benchmark

From the repository root:

```sh
MTILE_RENDER_CAPTURE=/tmp/mtile-render-captures go test \
  -tags mtile_editor_bench ./cmd/editor -run '^$' \
  -bench '^BenchmarkEditorRender$' -benchmem -benchtime=300ms -count=10
```

The benchmark copies `examples/background-layers`, sets default preferences,
and draws Sprite, Map, and Code workspaces into a 1200×800 offscreen image.
Each iteration runs on a separate engine frame. Twenty warm-up frames precede
each measurement; pixel readback fences GPU work. A small unfocused native
window supplies the graphics context. Linux needs a display or Xvfb.

VSync is disabled and TPS is `SyncWithFPS`, but operating-system presentation
can still pace the outer `ns/op`. Use `render-ns/op`, measured around actual
drawing and pixel readback, for renderer comparisons. `B/op` and `allocs/op`
also include engine and benchmark synchronization work. Readback itself is
expensive; these measurements are not normal editor FPS estimates.

For dependency comparisons, compile this same harness against both module
files using `go test -modfile=... -c -tags mtile_editor_bench`. Run the binaries
from `cmd/editor`, alternate their order, and compare repeated samples with
`benchstat`. Compare decoded RGBA pixels separately from timing; compressed
PNG byte sequences are not the image-equivalence criterion.

## Real-editor VM smoke test

```sh
go build -tags ebitenginevmguest -o /tmp/mtile-editor-guest ./cmd/editor
go run -tags mtile_editor_vm ./internal/cmd/editorvmcheck \
  -guest /tmp/mtile-editor-guest -project examples/hello \
  -out /tmp/mtile-editor-vm-check
```

Host and guest must use the same Ebitengine version. The host uses a hidden
native graphics context and a loopback connection; this is windowless, not
displayless. The driver checks keyboard/mouse workspace navigation, actual
rendered frames, editing and saving, exact source restoration through undo,
and clean guest termination. It writes PNGs, `result.json`, and a copied
project under the output directory; the input project is never edited.

The fixture must use `game.lua` as its entry point. The driver explicitly
selects a verified nonexistent TinyGo path and stops the automatic Develop
preview, keeping this an editing-only check. It does not exercise playback,
audio, or MIDI. In particular, the editor's direct Oto context is outside
Ebitengine VM audio capture. Physical device latency needs separate testing.
