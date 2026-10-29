# Selected-font evidence (2026-10-29)

**Current geometry is B, 6×10. See [production B evidence](../../../research/font-typography/production-b.md)
and [measurements](b/measurements.json). The results below are historical 4×5
evidence from before the size migration; run those commands at commit `a1e0fad`.**

The `international` preset changes ASCII artwork and adds Greek/German glyphs.
The fixed geometry stays 4×5 in a 6×8 cell. `ascii.lua` exercises a static HUD,
a changing decimal score, and a full 40-column row. The existing Mode 0 call
restrictions remain in force; Unicode strings still fall back to Mode 4.

```sh
go run ./cmd/renderbench -project examples/font-demo \
  -source docs/benchmarks/fonts/ascii.lua -out /tmp/mtile-font-parity \
  -warmup 10 -samples 60 -gate parity
go test ./internal/transpiler -run '^$' -bench BenchmarkSelectedFontAtlas -benchmem
(cd runtime && go test -run '^$' -bench BenchmarkSelectedFontPrint -benchmem)
```

Actual mGBA/TinyGo results in [report.json](report.json): **0 differing pixels**
over 60 synchronized captured frames. Mode 0 median 33,014 cycles vs Mode 4
75,225 (56.1% less), p95 headroom 528,724 cycles, no missed frames. Mode 0
leaves 248,576 EWRAM bytes free vs 111,116 in the debug Mode 4 build. These are
workload measurements, not a blanket performance claim for other games.
[Mode 0](mode0.png) and [Mode 4](mode4.png) captures match exactly.

[host.txt](host.txt) records Apple M2 Pro microbenchmarks: selected-font ASCII
and Unicode rendering both allocate **0 B / 0 allocations**. Atlas generation
allocates on the host only, before linking. The per-glyph parity test covers
all 95 ASCII glyphs in all five presets at colors 1, 7, and 255. Existing
runtime tests cover clipping, camera, palette remaps, cursor and control codes.

`TestBuildLinksExactlyOneSelectedFontInROM` inspects actual TinyGo ELFs for
PICO-8, arcade Mode 0 and international Mode 4, asserts one `projectFontROM`
symbol in cartridge address space, compares its 380 ASCII bitmap bytes with
the selection, and rejects linked `builtinFont`/`font4x5` fallback banks.
An unused custom font file is present during the check and is not linked.

The Unicode demo build is 26,932 ROM bytes, with 254,112 EWRAM bytes free.
Its one font has 174 glyphs: 696 bitmap bytes, plus 316 bytes of Unicode
codepoints and a 12-byte ROM slice descriptor (1,024 bytes total on ARM).
The Context borrows a pointer; there is no font bitmap copy into EWRAM.
