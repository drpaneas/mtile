# gbadev Discord hardware findings for mTile

Research date: 2026-09-19. Searched the user's open gbadev Discord server,
including server-wide DMA results and targeted searches in `help-and-support`
and `asm` for DMA, VRAM, optimization, division, scanlines, waitstates, and
byte access. Read surrounding discussion where context affected the advice.
This is a targeted review, not an exhaustive archive search.

A [follow-up covering older discussions from 2020–2025](discord-gbadev-history-2020-2025.md)
adds power-saving waits, conditional 4bpp packing, animated tiles, compression
contracts, and hardware effects.

Discord links require server access. Community reports are distinguished below
from documentation and observations of mTile's current working tree. No runtime
changes or new benchmarks were performed; existing uncommitted work was left
untouched.

## 1. Measure audio timing during full-frame DMA

**Highest-priority investigation; not a reproduced mTile failure.**

The server warns that DMA delays interrupt handling. In a May 28 discussion,
CKDEV reported that replacing DMA transfers with CPU copies removed audio
clicking. This is another project's result, not proof of the same symptom in
mTile. [Interrupt warning, April 26](https://discord.com/channels/768759024270704641/829850171151876127/1498055725564297278),
[reported improvement, May 28](https://discord.com/channels/768759024270704641/829850171151876127/1509332852456816641).

In mTile, `runtime/mode4.go:65–105` sends the complete framebuffer through one
DMA by default and documents about **76,970 cycles** for the copy. Audio's
Timer 0 service period is **65,536 cycles**
(`runtime/audio/native_types.go:30–35`). `serviceCommit` advances the scheduler
by a fixed quantum per serviced interrupt
(`runtime/audio/native_controller.go:445–470`).

DMA pauses the CPU. Depending on its starting phase, a transfer longer than
one timer period can encompass two overflows before their interrupt is
serviced. A single pending interrupt bit does not count both events. This
creates a plausible lost-tick/song-timing risk. [Tonc DMA](https://gbadev.net/tonc/dma.html),
[GBATEK interrupt and DMA reference](https://mgba-emu.github.io/gbatek/).

**Next check:** compare serviced audio ticks with an independent elapsed-time
hardware counter, vary DMA start phase, and record maximum service latency.
The existing coexistence fixture checks handler execution cost and event/queue
health, but does not independently establish service cadence against elapsed
time (`runtime/cmd/audiopsgcoexist/main.go:260–285`). Compare smaller DMA chunks
with the existing IWRAM CPU-copy path; measure total frame cost and audio
timing together. Merely moving the copy into VDraw does not remove IRQ blocking.

For scale, Tonc measured **76,889 cycles** for its own EWRAM-to-VRAM Mode 4
DMA32 copy on hardware. That is corroborating evidence, not a new measurement
of mTile. [Tonc copy benchmark](https://gbadev.net/tonc/text.html).

## 2. DMA is not a fallback for unaligned memory

**Concrete alignment mismatch; native reproduction pending.**

The server explicitly calls out DMA source alignment and the cost of mismatched
copy alignment. [CKDEV, August 27](https://discord.com/channels/768759024270704641/829850171151876127/1542366284719857814),
[aikku93, August 27](https://discord.com/channels/768759024270704641/829850171151876127/1542367078730829875).

`runtime/hw/fill_tinygo.go:14–29` deliberately sends an unaligned destination
to **32-bit DMA**. DMA32 discards low address bits; the vendored mGBA
implementation masks source and destination down to transfer-width boundaries
(`third_party/mgba/src/gba/dma.c:92–104`). This emulator implementation
corroborates the risk; a physical sentinel-buffer reproduction remains to be
done. Consequently that fallback can write
before the requested slice and leave its end unfilled. The byte-slice copy
helper also lacks alignment validation (`runtime/hw/mmio_tinygo.go:61–68`).

**Next check:** exercise offsets 1, 2, and 3 with sentinel bytes around the
requested range on the native target. Define and enforce the helper's alignment
contract. For ordinary RAM, a safe unaligned path can peel bytes and then use
aligned stores; video memory needs halfword/word-safe handling instead. This
has not been demonstrated as a failure in a current game.

## 3. Preserve DMA startup and count safeguards

A May 28 thread discusses crashes from touching DMA registers too soon after
enabling a transfer. The same discussion includes an emulator-only crash, so
it should not be treated as proof that every observed failure was that hardware
hazard. [Discussion](https://discord.com/channels/768759024270704641/829850171151876127/1509337661402382407).

GBATEK specifies a two-clock startup interval. Counts are transfer units, not
bytes; a zero DMA3 count means 65,536 transfers. mTile already rejects zero and
out-of-range counts in `runtime/hw/mmio_tinygo.go:57–84`.
[GBATEK](https://mgba-emu.github.io/gbatek/), [Tonc DMA](https://gbadev.net/tonc/dma.html).

The existing September 9 `runtime/audiopsgcoexist.elf` has two ARM ALU
instructions between enabling DMA and reading its control register at the two
inspected Mode 4 sites. No startup defect was observed there. Source code has
no explicit delay, so a compiler/disassembly regression check would protect
this property across toolchain and layout changes.

## 4. Keep tiles resident and budget the complete visible update

The server describes wasted work from continually reuploading unchanged map
tiles, and recommends keeping persistent HUD content in hardware tilemaps.
[Map-upload mistake, May 28](https://discord.com/channels/768759024270704641/829850171151876127/1509335552606998578),
[persistent score tiles, September 18](https://discord.com/channels/768759024270704641/829850171151876127/1550575479826944152).

**Already implemented:** Mode 0 skips clean maps
(`runtime/mode0.go:175–181`), reuses resident assets, and checks that sufficient
VBlank remains for the pending commit (`runtime/mode0.go:143–152`). Mode 4
uploads its hidden page before waiting for the display flip.

Preserve these distinctions: the complete frame is 280,896 cycles; the
nonvisible interval is 83,776 cycles. Include map/tile/OAM/palette uploads and
interrupt work in the commit budget. A hidden-page write can occur during
active display; do not import Game Boy VRAM restrictions into GBA code.
[Tonc timing](https://gbadev.net/tonc/video.html),
[Discord correction about GBA VRAM, July 1](https://discord.com/channels/768759024270704641/829850171151876127/1521937036523864296).

## 5. Treat memory placement and waitstates as measured choices

Server advice favors ARM hot routines in IWRAM, explains that ROM prefetch
changes comparisons with EWRAM, and warns that accelerated EWRAM timings vary
by console revision. [Code placement, August 27](https://discord.com/channels/768759024270704641/833314924766953482/1542332911049637979),
[prefetch distinction, September 5](https://discord.com/channels/768759024270704641/829850171151876127/1545900778756112404),
[hardware compatibility report, May 28](https://discord.com/channels/768759024270704641/829850171151876127/1509343965214806127).

**Already implemented:** startup sets `WAITCNT=0x4317`
(`runtime/crt0/gba.s:52–63`), and hot rendering routines use IWRAM. No EWRAM
waitstate-control writes were found. Keep standard EWRAM timing: the documented
Micro lockup is sufficient reason not to make overclocking a default.
[GBATEK](https://mgba-emu.github.io/gbatek/),
[mGBA author's prefetch explanation](https://mgba.io/2015/06/27/cycle-counting-prefetch/).

Do not fill all IWRAM with code/assets and forget stacks. A September 7 support
discussion traced a near-full allocation to only 120 bytes of remaining stack
space. mTile reserves 1 KiB IRQ and 8 KiB user stacks in
`runtime/crt0/gba.ld:14–18`; preserve those reservations and inspect linked
sizes when adding hot routines. The other project's stack layout is not
mTile's layout. [Stack-space warning](https://discord.com/channels/768759024270704641/829850171151876127/1546570572815802479).

## 6. Arithmetic optimization must preserve rounding

The server recommends fixed-point arithmetic and avoiding repeated general
division, but also warns that signed shifts and division can round differently.
[Fixed-point advice](https://discord.com/channels/768759024270704641/829850171151876127/1540307728113868852),
[rounding warning](https://discord.com/channels/768759024270704641/829850171151876127/1506400725771616428).

**Already implemented:** mTile specializes several exact division shapes and
corrects power-of-two shifts toward zero (`runtime/fixedpoint.go:34–95`). Its
Lua remainder deliberately follows the divisor's sign. Preserve negative,
fractional, overflow-boundary, and zero-divisor behavior when considering
reciprocal tables or multiplication replacements. Community shortcuts are not
permission to approximate the public Lua contract.

## 7. Budget sprite pixels, not only object count

The server warns that sprite-based text can exhaust per-scanline rendering
capacity even when the scene seems small.
[GValiente, June 25](https://discord.com/channels/768759024270704641/829850171151876127/1519721845664776374).

**Already implemented:** mTile distinguishes 128 OAM entries from its normal-OBJ
1,226-cycle scanline budget and conservatively handles dynamic positions
(`docs/performance.md`, “Full OAM and scanline proof”;
`internal/transpiler/renderer_selection.go:778`). Keep hardware limits,
pixel-parity fixtures, and benchmark evidence together when extending Mode 0.
Passing an object-count check alone is insufficient.

## 8. Preserve video access widths and real-device signoff

The server discusses the danger of generated byte accesses in video-memory
code and emulator differences in timing tests.
[Access-width discussion, August 14](https://discord.com/channels/768759024270704641/829850171151876127/1537749623110635520),
[emulator-test discussion, July 26](https://discord.com/channels/768759024270704641/829850171151876127/1530771646896541896).

Use aligned 16/32-bit video stores: byte writes duplicate data in BG VRAM and
palette RAM, and are ignored in OBJ VRAM/OAM. mTile's volatile hardware helpers
already use the proper widths (`runtime/hw/mmio_tinygo.go:10–47`). Avoid
replacing them with generic byte-copy operations.
[GBATEK](https://mgba-emu.github.io/gbatek/).

Physical audio signoff remains explicitly pending in
`docs/audio/completion.md:59–61`. Follow the existing hash-verified procedure
in `docs/audio/hardware-signoff.md`, including cold boot and sustained graphics
load. Modern emulator tests are valuable, but are not physical signoff; the
mGBA author's documented hardware DMA investigation illustrates why.
[Hardware investigation](https://mgba.io/2020/01/25/infinite-loop-holy-grail/).

## Suggested order

1. Establish audio elapsed-time/cadence coverage during full-frame DMA.
2. Prove and correct the unaligned fill/copy contract on the native target.
3. Add a narrow DMA startup code-generation guard.
4. Complete the existing physical-device signoff using the tested ROM hashes.

These are follow-up recommendations, not changes made by this research.
Any later public Lua API change must also satisfy `AGENTS.md`'s renderer
classification, documentation, parity, and benchmark requirements.
