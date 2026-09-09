# Contributing to Mtile

Bug reports, clearer examples, documentation fixes, and focused code changes
are welcome. Start with the [setup guide](docs/getting-started.md) and run an
[example](examples/README.md) before changing the toolchain. Please follow the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Your first contribution

- Fix a confusing setup step or add a small reproducible example.
- Reduce a bug to a minimal Lua file or project that can become a regression test.
- Improve diagnostics so a failed build points to an actionable source location.
- Measure a runtime bottleneck and include before/after GBA cycle evidence.

Search existing issues before starting. For a substantial feature or a public
API change, describe the problem and proposed behavior in an issue first so
the approach can be discussed. Small fixes can go straight to a pull request.
Use a branch, keep the change focused, and explain what changed and how it was
verified. Mention skipped checks rather than presenting them as passing.

For questions use [Support](SUPPORT.md); for private vulnerability reports use
[Security](SECURITY.md). Documentation-only changes do not require a TinyGo
installation.

## Repository layout

Two Go modules:

- `github.com/drpaneas/mtile` (repo root) - CLI, editor, transpiler,
  asset pipeline. Requires Go 1.27.1+.
- `github.com/drpaneas/mtile/runtime` (`runtime/`) - the GBA runtime linked
  into every ROM. It must stay free of editor and host-tool dependencies;
  hardware access goes through `runtime/hw/`. Requires Go 1.22+.

| Directory | Responsibility |
| --- | --- |
| `cmd/mtile` | Project creation, builds, and editor launch |
| `cmd/editor`, `internal/editor` | Desktop application and project editing |
| `internal/luaparser`, `internal/transpiler` | Language frontend and Go generation |
| `internal/assetsync`, `internal/build` | Project assets and ROM pipeline |
| `runtime`, `runtime/hw` | Cartridge code and hardware access |
| `cmd/mtile-player`, `internal/emulator` | Headless mGBA helper and preview protocol |
| `cmd/renderbench` | On-target rendering parity and cycle measurements |
| `docs`, `examples` | Guides, contracts, evidence, and runnable projects |

## Before you send a PR

Run these baseline checks from the repository root:

```sh
gofmt -l .                     # should print nothing
go vet ./... && go test ./...  # repo root
(cd runtime && go vet ./... && go test ./...)
python3 scripts/docs/check_links.py
```

With [golangci-lint](https://golangci-lint.run/) 2.13.2 (built with Go 1.27 or newer), run
`golangci-lint run ./...` in both modules.

With [TinyGo](https://tinygo.org/) 0.42.0 on `PATH`, the full pipeline can be
exercised locally:

```sh
go run ./cmd/mtile build examples/hello/
(cd runtime && make smoke-gba) # startup smoke ROM + zero-allocation gate
```

TinyGo-dependent tests skip automatically when `tinygo` is not installed.
The [CI workflow](.github/workflows/ci.yml) additionally exercises renderer
specializations, linked ROMs, allocation gates, and mGBA parity benchmarks.
On Linux, editor tests need the documented graphics dependencies and a display;
use `xvfb-run -a go test ./...` in a headless environment. On macOS, run
the editor tests in an active graphical login session. Without one, Ebitengine
can fail during display initialization before tests run.
`go test -c -o /tmp/mtile-editor.test ./cmd/editor` checks compilation only;
it does not substitute for the desktop tests.

For edits to the README and entry-point guides, use Python 3.9+ and run
`python3 scripts/docs/check_links.py`. This checks local destinations and
heading anchors without relying on network access. Regenerating the optional
README animation also requires Pillow; see [capture instructions](docs/media/README.md).

For release-script changes, run `sh -n scripts/release/build-bundle.sh` and
`python3 -m unittest discover -s scripts/release -p '*_test.py'`. These packaging
tests use stub compilers and a local archive; a real bundle still needs the
[release sign-off](scripts/release/README.md#before-distributing-a-bundle).

## Audio runtime changes

The active product is the four-channel Game Boy PSG v2 specification in
[`docs/audio/`](docs/audio/). Production scope is exactly CH1, CH2, CH3, and
CH4. Direct Sound, FIFO, PCM, software mixing, and audio DMA are deferred.

Changes to `runtime/audio/`, `runtime/hw/psg.go`, generated audio startup, or
the v2 assets/compiler must keep both host hot paths and linked GBA programs
allocation-free. With TinyGo 0.42.0 on `PATH`, run:

```sh
cd runtime
go test ./audio ./hw -count=1
make smoke-gba audioref-gba audiopsg-gba audiopsgcoexist-gba
cd ..
go test ./internal/transpiler \
  -run 'TestGenerated(Game|AudioGame)CompilesWithTinyGo' -count=1
scripts/audio/fuzz_smoke.sh
```

`audioref-gba` builds only the CH1–CH4 truth ROMs. `audiopsg-gba` guards the
managed controller and `audiopsgcoexist-gba` measures all four channels under
graphics load. These targets use TinyGo `-print-allocs-cover=.` and fail on any
target heap allocation. The generated-package test guards the actual Lua/v2
asset startup shell, including `stat(80..92)` telemetry.

The obsolete software mixer, refill streams, and audio-v1 asset tools have
been removed. Direct-register FIFO probes remain hardware research, outside
the product and release gates. Keep Timer 0 and all shared PSG registers under
the single v2 controller.

If scheduler work, retained state, register cost, or graphics coexistence
changes, update [`docs/audio/performance.md`](docs/audio/performance.md) with a
repeatable ARM7TDMI/mGBA extraction. Emulator results do not replace the
physical-GBA listening and capture gates in
[`docs/audio/hardware-signoff.md`](docs/audio/hardware-signoff.md).

## Performance changes

Anything touching `runtime/` hot paths (sprite/map blitting, print, table
ops, the heap) must come with numbers:

```sh
cd runtime
go test -bench=. -benchmem          # host: relative cost + 0 allocs/op
make benchspike-gba                 # target: real ARM7TDMI cycles in mGBA
```

`docs/performance.md` records the current measured costs; update its table
when they change. IWRAM placement (`//go:section .iwram`) is reserved for
proven hot loops - check the linker map's `.data` size stays well under
32 KB after adding one.

## Public Lua API changes

Every new, renamed, removed, or semantically changed public Lua API must update
the renderer contract in the same change. Adding the implementation alone is
not complete.

- Update the public `Builtins` registry and explicitly classify the API in
  `BuiltinRendererCompatibility` as renderer-independent, both renderers,
  Mode 0 conditional, or Mode 4 only. If a different class is genuinely
  required, add it explicitly with coverage; never leave `RendererUnknown`.
- Update [`docs/render-modes.md`](docs/render-modes.md) with the exact semantic
  reason, Mode 0 restrictions, and AUTO fallback behavior.
- Add parser, transpiler, runtime, and renderer-selection tests. Any API admitted
  to Mode 0/AUTO also needs pixel-parity fixtures and benchmark evidence for its
  accepted call shapes.
- Preserve exact semantics. A visually similar Mode 0 approximation is not
  sufficient; unsupported or unproven behavior must fail forced Mode 0 or make
  AUTO fall back to Mode 4 with a stable diagnostic.

Run the mandatory classification gate with:

```sh
go test ./internal/transpiler -run 'TestEveryBuiltinHasRendererCompatibility|TestRenderModeDocumentationMentionsEveryBuiltin'
```

## Guidelines

- Keep the runtime allocation-free on hot paths; `make smoke-gba` enforces
  this with `-print-allocs`.
- No divides or Go-slice bounds checks inside per-pixel loops; see the
  design rules in docs/performance.md.
- `runtime/target-gba.json`, `runtime/target-gba-smoke.json`, and
  `runtime/crt0/gba-smoke.s` are generated (`make generate` in `runtime/`)
  and gitignored - never commit them.
- Follow the public Lua API renderer-classification rule above; API changes need
  parser, transpiler, runtime, renderer-selection, and documentation coverage.
- Match the existing comment style: explain *why*, especially for hardware
  timing and memory-layout decisions.

## License

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE).
