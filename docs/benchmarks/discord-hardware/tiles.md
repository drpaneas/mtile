# Tile, palette-zero, and sprite-budget trials

Measured 2026-09-19 using TinyGo 0.42.0 and the repository's native headless
mGBA player. The reports identify the helper hash and local toolchain. These
are emulator CPU/correctness measurements, not physical hardware power tests.
The production runtime was restored after the rejected 4bpp trial.

## E05 — reject runtime 4bpp conversion

A real opt-in candidate packs immutable low-index tiles at startup, changes
BG/OBJ tile stride and hardware depth attributes, and leaves logical palette
identity intact. Eligibility requires every sheet index in 0..15, no mutable
sheet, no text, and no packed multi-cell objects. That is a conservative global
one-bank subset of the general per-object palette problem. It is not an AUTO
change. The candidate patch and deterministic fixture generator are retained
in `scripts/discord-hardware`.

| Mode 0 measure | Baseline 8bpp | Candidate 4bpp |
| --- | ---: | ---: |
| Median / p95 game cycles | 30,745 | 30,713 |
| Median draw cycles | 2,544 | 2,512 |
| Median commit cycles | 12,036 | 12,036 |
| Cold upload cycles | 1,163,932 | 1,814,950 |
| ROM bytes | 31,424 | 31,536 |
| Free EWRAM bytes | 236,148 | 236,148 |
| Cold BG + OBJ tile bytes | 32,704 | 16,352 |
| Steady tile-upload bytes | 0 | 0 |
| OAM / worst-line OBJ cycles | 4 / 32 | 4 / 32 |
| Differing pixels / missed frames | 0 / 0 | 0 / 0 |

Cold tile bytes are the exact transfer-size sum for the generated 256-tile
sheet: 255 BG tiles plus all 256 OBJ tiles, including the blank OBJ0. Clear,
map, palette, and OAM transfers remain unchanged and are excluded from that
specific byte count. The upload API's cold statistic does not include ordinary
OBJ initialization, so this sum is derived from the checked candidate loops.

Both standard 20-warmup/120-sample and zero-warmup/120-sample captures match
Mode 4 pixel-for-pixel. Coverage includes all four flips, transparent source
zero, `pal(1,0)` producing opaque logical zero, and a display remap to index
200. The original logical indices remain palette identities even when their
current RGB values coincide. This is not proof of arbitrary bank allocation
or of opaque source-zero `palt` semantics beyond the existing renderer guard.

**Decision: drop this candidate.** A 0.10% steady improvement and halved
one-time tile traffic do not justify a 55.9% cold-cycle regression, larger ROM,
unchanged EWRAM, and added format restrictions. Build-time packed ROM assets
would be a different implementation; this experiment does not disprove that
approach and does not mark it implemented.

## E06 — retain shared animation as an authored pattern

The paired fixture compares 128 separately drawn animated 8x8 OBJs with 128
map cells referencing one mutable BG tile. Sixteen direct `sset` calls change
its first two rows per update, so the renderer uploads one 64-byte BG tile.
The source and placement are otherwise pixel-identical.

| Mode 0 measure | Sprite overlays | Shared BG tile |
| --- | ---: | ---: |
| Median / p95 game cycles | 52,115 / 52,115 | 5,368 / 5,378 |
| Median draw cycles | 49,855 | 620 |
| Median commit cycles | 2,058 | 795 |
| Cold upload cycles | 290,184 | 521,775 |
| ROM bytes | 12,940 | 14,808 |
| Free EWRAM bytes | 257,180 | 253,004 |
| OAM entries / worst-line cycles | 128 / 128 | 0 / 0 |
| Per-tick OBJ tile / OAM bytes | 0 / 1,024 | 0 / 0 |
| Per-tick BG tile / map bytes | 0 / 0 | 64 / 0 after first map |
| Differing pixels / missed frames | 0 / 0 | 0 / 0 |

All 120 synchronized logical frames match, including the first one. The
89.7% cycle saving repays the additional 231,591 cold cycles in about five
updates. The extra 4,176 EWRAM bytes largely come from adding a BG screen shadow
to a previously sprite-only cartridge. These costs are explicit, not hidden
by claiming that shared tiles are universally superior.

**Decision: keep the pattern and a small opt-in example.** The existing API
already supports it, so no production renderer rewrite is needed. The final
example removes two unused comparison scalars; its own native Mode-4/Mode-0
parity run records 5,386 median/p95 Mode 0 cycles, 14,648 ROM bytes, 253,012 free
EWRAM bytes, zero differing pixels, and zero missed frames over 120 frames.

A required counterexample puts half the sprites in the opposite animation
phase. One shared tile then differs by exactly 1,024 pixels per frame (122,880
over 120 frames). Use distinct tile IDs for independent phases. Do not rewrite
arbitrary sprite animation into shared BG mutation automatically. Metatile
compression is a separate asset representation experiment, not implemented by
this example; its executable trial is recorded below.

## E10 — retain exact zero semantics and separate sprite limits

Focused existing guards all passed: 46 compiler cases, 34 Mode 0 runtime cases,
and two experimental object-canvas cases. They exercise draw/display remaps,
opaque zero versus uncovered pixels, all 256 logical color identities with an
available escape index, exhausted palette rejection without destination
mutation, OAM overflow, packed-cache exhaustion, fractional Y bounds, and
separate 128-entry versus 1,226-cycle scanline constraints.

The native shared-animation baseline additionally uses the exact 128-entry
OAM boundary with only 128 cycles on its busiest scanline; reducing its bpp
would not reduce that object count or scanline width. Existing compiler tests
accept 38 overlapping 32-pixel OBJs at 1,216 cycles and reject the over-budget
combination. **Decision: keep these existing protections; no redundant runtime
change is justified.** The logs are retained alongside the JSON reports.

## E06b — reject blanket metatile compression

A standalone TinyGo ROM compares an aligned raw-map copy with 2x2 dictionary
expansion into the same 32x32 uint16 map buffer. It executes each path 100 times
for repeated and diverse maps, reads the hardware timer, compares all 1,024
indices, and verifies surrounding canaries. Both fixtures had zero mismatches
or boundary violations in native mGBA.

| Measure | Raw map | Repeated 2x2 dictionary | Diverse 2x2 dictionary |
| --- | ---: | ---: | ---: |
| Encoded payload bytes | 2,048 | 272 | 2,304 |
| Full expansion cycles | 6,042 | 27,530 | 27,530 |
| Expanded map RAM bytes | 2,048 | 2,048 | 2,048 |

The repeated case has two dictionary entries (16 bytes) and 256 one-byte IDs.
The diverse case needs 256 entries (2,048 bytes) plus those same 256 IDs. A
single diagnostic ROM contains both forms; this table describes encoding
payloads, not an end-to-end linked-ROM saving. The cost is 4.56 times that of
the existing aligned copy, while repeated-map storage saves 86.7% and the
all-unique counterexample grows 12.5%.

**Decision: drop as a blanket replacement.** A future storage-oriented option
would need a payload-size gate and a deliberate load-time budget, plus proof
for `mget`/`mset`, flags, layer identity, and streamed windows. The measured
prototype retains expanded RAM, so it does not save EWRAM or per-frame VRAM
upload bytes. Keep the reproducible experiment and exact data-dependent result;
no map format or public API changes are warranted by this microbenchmark alone.
