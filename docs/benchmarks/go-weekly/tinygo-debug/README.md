# TinyGo mGBA panic logging, 2026-09-19

**Keep the isolated reproducer and upstream-ready patch. Do not enable a
patched TinyGo fork in the normal build.** TinyGo 0.42.0's optional `mgbadebug`
runtime can capture ordinary Go panics after a two-line source patch (one
constant plus its separating blank line). This closes a diagnostic gap that
mTile's explicit runtime logger does not cover. It is not a compiler upgrade or
a speed optimization, and mTile's existing preview logger stays in place.

## Failure and fix

The installed TinyGo reports 0.42.0, Go 1.27.1, LLVM 22.1.4 on darwin/arm64.
`runtime_arm7tdmi.go` defines `zeroSizeAllocPtr`, but its build constraint excludes
`mgbadebug`. The replacement `runtime_mgba.go` omits the definition even though
`gc.go` uses it with `-gc=none`. Both unmodified diagnostic builds therefore fail:

```text
# runtime
.../src/runtime/gc.go:28:24: undefined: zeroSizeAllocPtr
```

The [complete patch](upstream.patch) copies the ARM7TDMI definition into
`runtime_mgba.go`:

```go
const zeroSizeAllocPtr uintptr = 16 // points somewhere in the BIOS which is not readable
```

The experiment copies TinyGo resources to a temporary directory and uses the
unchanged real compiler with explicit `TINYGOROOT`. Homebrew's launcher must be
bypassed because it unconditionally resets that variable. The four relevant
installed runtime/target file hashes match before and after the experiment.
No installed toolchain or product source was edited.

## Native panic proof

Both fixtures use mTile's generated target, startup assembly and linker script,
`-gc=none -opt=2 -panic=print`, and the native `mtile-player`. They do not import
the mTile runtime or call `rt.DebugLog`. The bounds fixture obtains its invalid
index from a volatile register so the compiler generates the bounds failure.
The unmodified ordinary builds run but emit no diagnostics; adding `mgbadebug`
to unmodified TinyGo fails to compile. Patched debug builds produce:

```text
mGBA[GBA Debug]: MTILE_PANIC_READY
mGBA[GBA Debug]: panic: MTILE_ARBITRARY_PANIC
mGBA[GBA Debug]: MTILE_BOUNDS_READY
mGBA[GBA Debug]: panic: runtime error at 0x08000848: index out of range
```

These are recorded in [explicit-panic stderr](panic-patched-debug-player.log)
and [bounds-panic stderr](bounds-patched-debug-player.log). Both players return
four valid frames and exit normally when asked, despite the cartridge's panic
loop. The compiler's allocation coverage passes the existing zero-allocation
checker for every successful diagnostic build.

| Diagnostic | Ordinary ROM bytes | Patched debug ROM bytes |
| --- | ---: | ---: |
| Explicit panic | 764 | 2,924 |
| Volatile bounds panic | 792 | 2,380 |

## Healthy cartridge preservation and cost

The fixture is `examples/testio`, forced separately to Mode 0 and Mode 4, with
no keys held. The patch with `mgbadebug` **disabled** produces byte-identical
release ROMs, identical generated Go, identical memory totals and matching
first-four-frame hashes in both modes. This is direct evidence of zero release
overhead for this fixture.

| Release renderer | ROM bytes | IWRAM bytes | EWRAM used | EWRAM free |
| --- | ---: | ---: | ---: | ---: |
| Mode 0, either resource tree | 24,376 | 10,200 | 19,956 | 241,676 |
| Mode 4, either resource tree | 26,356 | 13,220 | 75,936 | 185,696 |

Release SHA-256 values:

- Mode 0: `9b8abbbacc1c002d7787c1d1c090ebea35e5149c35751e6e0f29620bc029fe4c`
- Mode 4: `2c49d735a9f57cfb89a42d6e7c5529683e8df96fdc1d1d6f10ac96c7d187d359`

For **enabled** logging, compare normal mTile preview builds with builds that
also set `mgbadebug`. After twenty warmup updates, all 300 logical frames
(21–320) match on RGBA, watches, GC count, allocated/live/peak heap bytes, live
handles, heap errors, missed frames and audio-overflow state. EWRAM is unchanged.

| Preview renderer | Ordinary ROM → debug ROM | ROM increase | IWRAM increase | Ordinary → debug frame cycles |
| --- | ---: | ---: | ---: | ---: |
| Mode 0 | 25,816 → 27,396 | 1,580 | 20 | 1,567 → 1,575 |
| Mode 4 | 27,884 → 29,136 | 1,252 | 72 | 229,270 → 229,278 |

Each cycle value is the median, p95 and maximum of the 300 recorded `LastFrame`
samples; all three coincide here. This counter measures emulated GBA work,
excluding deliberate frame pacing. The eight-cycle difference includes changes
to code layout and is not an isolated measurement of the logger itself. These
healthy frames emit no TinyGo panic messages. This simple fixture is not a
claim about game-wide performance, physical hardware, or panic-report latency.
Cached builds took approximately 1.52–1.60 seconds each; this single ordered run
does not establish a compilation-speed effect.

## Validation and reproduction

The focused build checks, emulator suite, live existing-preview-logger test,
and full host runtime suite passed. The live logger check covers its long/short
message handling and interrupt-state restoration; that implementation remains
unchanged. Commands and outputs are in [validation.json](validation.json) and
`tests-*.log`. The parent's broader required repository gates had already passed
on the same product source; this experiment adds only scripts and records.

Run the [reproducer](../../../../scripts/go-weekly/tinygo-debug/README.md) to
generate a fresh matrix. [results.json](results.json) contains per-frame records,
[commands.json](commands.json) has exact commands and elapsed times, and
[metadata.json](metadata.json) pins source and tool hashes, including the
pre-existing dirty worktree. The patch is retained for a future upstream change;
no upstream issue or message was sent. Until a supported TinyGo release contains
the fix, the private copy is an opt-in diagnostic experiment only.
