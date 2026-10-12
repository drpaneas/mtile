# Pose color variants

Verified on 2026-10-12 with TinyGo 0.42.0 and the native mGBA player.

Celeste was migrated from 179 authored poses to 78 canonical poses and seven
named replacement maps. All 1,373 host replay frames match the saved
pre-variant game: zero pixel/state differences and identical final RNG.
The native Mode 0/Mode 4 comparison covers 861 logical frames, including
first LCD scans, with zero image/state differences.

The resident Mode 0 bank uses 11,328 bytes, versus the previous 28,864 bytes
(451 8bpp tiles): a 61% reduction. All variants share canonical 4bpp pattern
bytes and select independent hardware palette banks. OAM remains 126/128;
the worst proven scanline remains 1,104/1,226 cycles. Runtime fixtures check
simultaneous different variants, asymmetric pixels, both flips, odd 32-byte
tile offsets, palette contents and zero tile uploads across variant switches.

The planner microbenchmark on Apple M2 Pro measured 3,450 ns/op, 1,688 B/op
and 15 allocations/op. This is build-time work. Mode 4 uses exact compiled
recolored pixels as its software fallback; it does not gain the hardware
palette-bank storage reduction.

Reproduce:

```sh
go test ./internal/transpiler -run '^$' -bench BenchmarkPoseColorVariant -benchmem
cd runtime && go test -tags 'mtile_mode0 mtile_mode0_obj' ./...
# From the repository root:
python3 examples/celeste/tests/verify_mode0_migration.py
go run examples/celeste/tests/mode0_native_verify.go -project examples/celeste
```

The Celeste example is a locally ignored project in this repository. The
compiler and runtime fixtures remain part of the tracked test suite.
