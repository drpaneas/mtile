# Contributing to Mtile

Thanks for your interest! Bug reports, fixes, and small focused features are
welcome.

## Repository layout

Two Go modules:

- `github.com/drpaneas/mtile` (repo root) - CLI, editor, transpiler,
  asset pipeline. Requires Go 1.25+.
- `github.com/drpaneas/mtile/runtime` (`runtime/`) - the GBA runtime linked
  into every ROM. It must stay free of editor and host-tool dependencies;
  hardware access goes through `runtime/hw/`. Requires Go 1.22+.

## Before you send a PR

Run the same checks CI runs:

```sh
gofmt -l .                     # should print nothing
go vet ./... && go test ./...  # repo root
cd runtime
go vet ./... && go test ./...  # runtime module
```

If you have [golangci-lint](https://golangci-lint.run/) installed, run
`golangci-lint run ./...` in both modules.

With [TinyGo](https://tinygo.org/) 0.41.1 on `PATH`, the full pipeline can be
exercised locally:

```sh
go run ./cmd/mtile build examples/hello/
cd runtime && make smoke-gba   # startup smoke ROM + zero-allocation gate
```

TinyGo-dependent tests skip automatically when `tinygo` is not installed.

## Audio runtime changes

The active product is the four-channel Game Boy PSG v2 specification in
[`docs/audio/`](docs/audio/). Production scope is exactly CH1, CH2, CH3, and
CH4. Direct Sound, FIFO, PCM, software mixing, and audio DMA are deferred.

Changes to `runtime/audio/`, `runtime/hw/psg.go`, generated audio startup, or
the v2 assets/compiler must keep both host hot paths and linked GBA programs
allocation-free. With TinyGo 0.41.1 on `PATH`, run:

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
graphics load. These targets use TinyGo `-print-allocs=.` and fail on any
target heap allocation. The generated-package test guards the actual Lua/v2
asset startup shell, including `stat(80..92)` telemetry.

Legacy Direct Sound/PCM targets remain reproducible historical research but
are not CI, release, or contribution gates. Do not extend them as product
features. Keep Timer 0 and all shared PSG registers under the single v2
controller.

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

## Guidelines

- Keep the runtime allocation-free on hot paths; `make smoke-gba` enforces
  this with `-print-allocs`.
- No divides or Go-slice bounds checks inside per-pixel loops; see the
  design rules in docs/performance.md.
- `runtime/target-gba.json`, `runtime/target-gba-smoke.json`, and
  `runtime/crt0/gba-smoke.s` are generated (`make generate` in `runtime/`)
  and gitignored - never commit them.
- New Lua builtins need parser, transpiler, and runtime coverage plus tests
  in all three layers.
- Match the existing comment style: explain *why*, especially for hardware
  timing and memory-layout decisions.

## License

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE).
