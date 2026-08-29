# Mtile repository instructions

## Public Lua API renderer classification is mandatory

Any change that adds, renames, removes, or changes the semantics of a public
Lua API must update the renderer contract in the same change. An API change is
incomplete until all of the following are true:

1. The public builtin registry in `internal/transpiler/transpiler.go` is
   updated.
2. `BuiltinRendererCompatibility` in
   `internal/transpiler/render_modes.go` explicitly classifies the API as
   renderer-independent, available in both renderers, Mode 0 conditional, or
   Mode 4 only. If none of those categories is correct, introduce and test an
   explicit new category; never leave or treat it as `RendererUnknown`.
3. `docs/render-modes.md` documents the classification, exact semantic reason,
   and every Mode 0 restriction or fallback condition.
4. Parser, transpiler, runtime, and renderer-selection tests cover the API.
   Mode 0 or AUTO eligibility additionally requires pixel-parity fixtures and
   benchmark evidence for the accepted call shapes.

Mode 0 support must preserve exact API semantics. Do not classify an API as
`RendererBoth` or admit it to AUTO because it can produce a merely similar
image. Unsupported or unproven behavior must fail a forced Mode 0 build or make
AUTO fall back to Mode 4 with a stable, source-positioned diagnostic.

Run at minimum:

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

The exhaustive classification and documentation tests in
`internal/transpiler/render_modes_test.go` are the merge gate for this rule.
