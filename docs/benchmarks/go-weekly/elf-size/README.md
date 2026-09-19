# Experiment 6: GBA ELF size comparison

**Retain**, 2026-09-19. `cmd/elfsize` is a standalone diagnostic using only the
Go standard library. It explains ROM/RAM costs without adding dependencies,
changing builds, or touching cartridge runtime code.

```sh
go run ./cmd/elfsize game.elf
go run ./cmd/elfsize -before before.elf -after after.elf
go run ./cmd/elfsize -before before.elf -after after.elf -json -top 0
```

The default displays the ten largest symbols or absolute symbol-size changes;
`-top 0` includes all. JSON is deterministic for identical paths and contents.
Missing symbol tables leave section totals available and explicitly disable
symbol comparison. Inputs must be linked, little-endian ELF32 ARM GBA binaries.

## Accounting

- **ROM load bytes** are the union of file-backed `PT_LOAD` physical ranges.
  **ROM image span** also includes holes between the GBA ROM origin and the last
  loaded byte. File backing is checked, including rejection of truncated loads.
- **RAM occupancy** is the union of allocated section VMA ranges, with IWRAM,
  application EWRAM, and the reserved EWRAM debug mailbox reported separately.
  It measures allocated coverage, not an address high-water mark across gaps.
- Section LMAs come from complete, consistent `PT_LOAD` file/VMA mappings.
  `.data` therefore contributes both ROM initialization bytes and IWRAM bytes;
  it can contain executable functions. NOBITS sections consume no ROM bytes.
- Exact symbol aliases share one row. ARM Thumb state is removed from function
  addresses before range attribution. Partial overlaps do not inflate totals.
  Symbols are matched by section and sorted alias names, rather than shifting
  addresses; repeated local names aggregate and renamed groups appear as
  removals/additions.
- Each section reports bytes not covered by sized symbols. Segment padding
  outside sections is reported separately. Section/symbol rows are explicitly
  nonadditive: neither aliases nor overlaps are used to manufacture totals.

## Independent reference results

The retained experiment-5 ELF pair remains useful evidence even though that
runtime optimization was rejected and restored. LLVM `readelf`, GNU `size`,
linker maps, saved native reports, and actual ROM file lengths agree with the
new command. Independent reviewer assertions are in `independent-validation.json`.

| Fixture | ROM load bytes | IWRAM | Application EWRAM | Debug mailbox |
| --- | ---: | ---: | ---: | ---: |
| Testio Mode 0 | 24,376 | 10,200 | 19,956 | 0 |
| Testio Mode 4 | 26,356 | 13,220 | 75,936 | 0 |
| Hash collision baseline | 42,940 | 11,660 | 153,928 | 512 |
| Hash collision candidate | 43,228 | 11,764 | 153,928 | 512 |

The hash pair's entire **+288 ROM / +104 IWRAM** change is explained by three
function deltas, with no change in unattributed bytes:

| Function | Before | After | ROM delta | IWRAM delta |
| --- | ---: | ---: | ---: | ---: |
| `Context.TableSet` | 1,172 | 1,276 | +104 | 0 |
| `Context.growTableWithRoots` | 2,368 | 2,448 | +80 | 0 |
| `Context.hashFind` | 656 | 760 | +104 | +104 |

Six real alias groups also match the independent symbol dump, including
`memcpy`/`memmove` (296 bytes) and the three division aliases (280 bytes).
The `.data` VMA remains `0x03002400`, while its ROM LMA moves from `0x08009e30`
to `0x08009ee8`. Linker-map assignment rows do not reliably expose actual
`_sidata`/`_heap_end` values; the command uses ELF metadata instead.

## Cost and validation

On Apple M2 Pro, darwin/arm64, Go 1.27.1:

- Five 250ms repetitions: median parse+attribution **50.905µs / 51.019µs** for
  baseline/candidate, approximately **103KiB and 999 allocations per ELF**
  (105,216–105,217 bytes).
- Twenty compiled-command JSON pair comparisons after two warmups:
  **10.279ms median**, **12.065ms p95**, **12.167ms maximum**, including process
  startup, file access, comparison, encoding, and captured output.

Parsing benchmarks preload the files and inspect ELF metadata. Their reported
MB/s uses full file size as a normalizer; it is not disk or payload-scan
throughput. The command has no game/editor runtime overhead because it is not
integrated into production builds.

Synthetic tests independently specify overlaps, aliases, Thumb addresses,
separate VMA/LMA, NOBITS, debug exclusion, section/segment padding, ROM holes,
stripped files, malformed headers, unsupported architectures, missing mappings,
conflicting load addresses, out-of-range symbols, overflowing ranges, truncated
backing, deterministic JSON, and successful help. All tests, four real-ELF
reference checks, and the scoped race check passed.

## Reproduce

Normal tests need no external toolchain or saved binaries:

```sh
go test ./cmd/elfsize
go test -race ./cmd/elfsize
```

With this session's saved experiment directory:

```sh
MTILE_ELFSIZE_FIXTURES=/tmp/mtile-go-weekly-loop go test ./cmd/elfsize -count=1 -v
MTILE_ELFSIZE_FIXTURES=/tmp/mtile-go-weekly-loop go test ./cmd/elfsize -run '^$' -bench '^BenchmarkELFParse$' -benchtime=250ms -count=5
go run ./cmd/elfsize -before /tmp/mtile-go-weekly-loop/hash-fingerprint/native/collision-mode0/baseline/game.elf -after /tmp/mtile-go-weekly-loop/hash-fingerprint/native/collision-mode0/candidate/game.elf
```

Raw logs, source-independent symbol/section dumps, all four JSON summaries,
comparison output, timing samples, and machine-readable acceptance results are
retained beside this report. Temporary compiled executable and full evidence:
`/tmp/mtile-go-weekly-loop/elf-size`.
