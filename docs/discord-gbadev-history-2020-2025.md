# Additional gbadev findings from 2020–2025

Researched on 2026-09-19 through the user's existing Discord browser session.
This follows the [first hardware review](discord-gbadev-hardware-notes-2026-09-19.md).

This pass deliberately searched older history: date filters before 2021, 2022,
2023, 2024, and 2026; oldest-first results; and additional result pages. Topics
included optimization, BIOS copies, compression, blending, interrupts,
metatiles, transparency, allocation, frame waits, and 4bpp graphics. Useful
discussions span October 2020 through December 2025. This is a targeted search
across the server's history, not a claim to have read every message.

The practical priorities are **sleeping during idle frame time**, **conditional
4bpp asset packing**, and **shared animated BG tiles instead of sprite overlays**.
The remaining findings mostly strengthen implementation contracts or suggest
future features. No runtime code was changed or new target benchmarks run.

## 1. Idle frame time is a battery optimization opportunity

**September 10, 2025:** a support discussion distinguishes polling `vid_vsync`
from `VBlankIntrWait`: both can synchronize display work, but the BIOS wait
halts the CPU while waiting. AntonioND explains that the Tonc examples were
changed to teach the interrupt-based approach.
[Power explanation](https://discord.com/channels/768759024270704641/829850171151876127/1415408027439333622),
[example maintenance](https://discord.com/channels/768759024270704641/829850171151876127/1415413027712405648).

**mTile opportunity:** `runtime/hw/pace_tinygo.go:22–29` spins on a timer, and
`runtime/hw/wait_tinygo.go:5–15` spins on VCOUNT. Faster rendering currently
creates more idle polling rather than allowing the CPU to sleep.

Evaluate an interrupt-driven idle path for power savings. Preserve the existing
two-display-frame cadence, the audio timer, and the distinction between
“use the current VBlank if still suitable” and “wait for the next VBlank.”
Blindly replacing every polling loop with `VBlankIntrWait` changes that
distinction. If using BIOS waits, the IRQ dispatcher must also maintain the
BIOS interrupt flags; acknowledging hardware IF alone is insufficient.
[Tonc BIOS waits](https://gbadev.net/tonc/swi.html#vsyncing-part-ii-vblankintrwait),
[Tonc IRQ bookkeeping](https://gbadev.net/tonc/interrupts.html).

This is a power-saving hypothesis, not a measured battery-life improvement or
a promise of higher frame rate.

## 2. 4bpp could halve tile bytes for eligible assets

**December 23, 2021:** GValiente recommends 4bpp sprites where possible because
they use less ROM and load faster.
[Discord advice](https://discord.com/channels/768759024270704641/831589248239009832/923585207171436644).

An 8×8 tile occupies 32 bytes at 4bpp versus 64 bytes at 8bpp. Each tile/OBJ
selects a palette bank of 16 entries, with index zero reserved for transparency.
[Tonc tile and palette layout](https://gbadev.net/tonc/objbg.html).

**mTile opportunity:** normal Mode 0 BGs explicitly select 8bpp
(`runtime/tiled_map.go:11–14`), as do hardware sprites
(`runtime/mode0_obj.go:316–324`). The current experimental canvas paths also
use 8bpp. A tested 4bpp canvas packer exists in
`runtime/tilecanvas_pixels.go`, but is not the current production path.

Start with immutable assets whose bank assignments can be proven exact. Each
complete hardware OBJ—not merely each of its 8×8 cells—must fit its bank.
Preserve logical color identities, `pal()` remapping, source transparency,
and `sget`/`sset` behavior. Mutable graphics can introduce colors that were
absent at build time. Retain 8bpp when the proof fails.

Halving tile bytes does **not** halve total frame time, OAM entries, or sprite
scanline cost. Require pixel parity and target benchmarks before AUTO admission.

## 3. Animate shared background tiles rather than many overlay sprites

**October–November 2023:** developers discuss updating an animated metatile's
graphics in VRAM without redrawing every map occurrence. Another developer
reports substantial CPU cost from temporarily implementing animated background
tiles as sprites. The reported percentage is specific to that project.
[Shared tile update](https://discord.com/channels/768759024270704641/831589248239009832/1167883371981066330),
[sprite workaround cost](https://discord.com/channels/768759024270704641/831589248239009832/1179795817566052422).

**mTile application:** repeated water, lava, conveyor belts, and similar scenery
are candidates for one shared tile update instead of many individual sprite
draws. Mode 0 already has dirty-tile uploads and `sset` support; measure whether
an eligible animation fits the existing mutation budget. All occurrences of a
shared tile change together, so this is unsuitable when they need independent
animation phases unless separate tile identities are allocated.

The related metatile advice is about a software map format, not a larger native
GBA tile size: group reusable 8×8 tiles into larger logical units, then expand
them when updating the visible map. This can reduce level-data duplication.
[Metatile explanation, October 2023](https://discord.com/channels/768759024270704641/829850171151876127/1167019855808053299).

## 4. Decompression buffers need the expanded size

**November 25, 2022:** a developer's animation worked uncompressed, and its
decompressed frames initially looked correct before becoming corrupted. The
discussion identifies the actual mistake: allocating a buffer for the
compressed tile size instead of the decompressed size.
[Discussion and diagnosis](https://discord.com/channels/768759024270704641/829850171151876127/1045524357037948938).

**Future asset-pipeline guard:** carry compressed and expanded lengths as
different metadata fields; validate destination capacity before decoding.
Choose compression granularity around how assets are used. A whole compressed
sheet does not automatically provide random access to individual animation
frames; either retain the expanded sheet or independently compress suitable
chunks. No current production LZ77 path was identified in this scoped audit.

## 5. “Faster copy routine” does not mean a compatible replacement

**July 18, 2024:** replacing `memcpy` with `CpuFastSet` in a serializer produced
intermittent crashes. The replies question its size/alignment contract; the
caller had only ensured multiples of four.
[Reported failure](https://discord.com/channels/768759024270704641/829850171151876127/1263453194902573146),
[caller assumption](https://discord.com/channels/768759024270704641/829850171151876127/1263455737347309609).

The documented GBA contract requires four-byte-aligned addresses and a length
in words that is a multiple of eight—**32 bytes**. Nonconforming lengths are
rounded up. That is a concrete hazard, though the Discord exchange does not
prove it was the sole cause of that serializer's crashes.
[libtonc BIOS contract](https://gbadev.net/libtonc/group__grpBiosMain.htm),
[GBATEK memory copy](https://rust-console.github.io/gbatek-gbaonly/#bios-memory-copy).

**mTile application:** retain explicit units, alignment, and tail handling in
specialized helpers. mTile already has custom IWRAM copy/fill routines; a BIOS
replacement needs a measured benefit and contract tests, not its name alone.

## 6. Palette index zero is a semantic constraint

**August 4, 2021:** GValiente explains that 8bpp BGs treat index zero as
transparent, while 4bpp BGs reserve the first entry of each bank. The global
background palette entry zero supplies the backdrop.
[4bpp transparency](https://discord.com/channels/768759024270704641/829850171151876127/872223491938721842),
[8bpp transparency](https://discord.com/channels/768759024270704641/829850171151876127/872223318328102992).

**Already protected in mTile:** opaque logical color zero cannot simply be
written as hardware OBJ index zero. Normal Mode 0 rejects nonempty color-zero
text (`docs/render-modes.md:98–102`); experimental object packing remaps opaque
zero to another physical entry and checks exhaustion
(`runtime/objectcanvas_pack.go:64–75`). Preserve this when changing palette
packing. Identical RGB values also need not be identical logical colors if
later palette remaps can distinguish them.

## 7. Hardware sprite alpha is not general alpha compositing

**December 2021:** multiple discussions explain that all sprites feed one OBJ
layer. The hardware chooses the foremost OBJ pixel before the blending stage,
so it cannot blend one sprite with a second sprite underneath.
[Explanation](https://discord.com/channels/768759024270704641/829850171151876127/922534755117977660).

OBJ-to-BG blending is possible; windows can selectively mask layers and effects.
Windows do not remove the OBJ-to-OBJ limitation.
[GBATEK effects](https://mgba-emu.github.io/gbatek/#lcd-io-color-special-effects),
[Tonc windows](https://gbadev.net/tonc/gfx.html#windowing).

**Future renderer constraint:** particle transparency, shadows, lighting, and
translucent HUDs cannot all be translated into semi-transparent OBJs with exact
software-renderer semantics. Prove the scene/layer arrangement or retain the
software path. mTile does not currently expose arbitrary hardware blending.

## 8. HBlank effects are promising, but their tables are live DMA input

**October 30, 2020:** the server discusses changing palettes per scanline to
produce richer imagery. The ambitious full-width palette-streaming proposal is
speculative in that thread; it is not a measured recipe ready to adopt.
[Earliest discussion](https://discord.com/channels/768759024270704641/771045950709694474/771638226456543242).

The narrower, documented technique is useful: HBlank DMA can change palettes,
scroll offsets, or window bounds between lines, enabling gradients, water
waves, and scanline parallax without software-redrawing the whole scene.
[Tonc HDMA example](https://gbadev.net/tonc/dma.html),
[Butano HBlank effects](https://gvaliente.github.io/butano/group__hblank__effect.html).

**October 11, 2025:** an HBlank IRQ-to-DMA conversion flickered on both emulator
and hardware while the effect's source table was being updated. The discussion
calls out modifying the active DMA buffer and considers double buffering or
disabling/rearming DMA around updates.
[Flicker investigation](https://discord.com/channels/768759024270704641/829850171151876127/1426522853435768943).

**Future mTile feature:** keep source storage alive, publish coherent tables,
rewind the source each frame, and account for HBlank updating the following
scanline. DMA repeat can reload count/destination, but does not rewind source.
Reserve a channel explicitly and retain exact renderer semantics. This is an
effect-specific proposal, not a general replacement for arbitrary Lua drawing.

## 9. Simulation, display, and audio clocks need explicit meanings

**December 16, 2020:** audio-buffering advice uses the actual 280,896 CPU cycles
per display frame rather than assuming exactly 60 Hz.
[Early timing discussion](https://discord.com/channels/768759024270704641/768759024760913922/788811484519464981).

**mTile relevance:** the LCD runs at about 59.73 Hz, so every-second-frame
presentation is about 29.86 Hz. mTile deliberately uses that cadence
(`runtime/profile.go:121–151`), while `time()` uses simulation ticks divided
by nominal 30 (`runtime/math.go:264–278`), and audio has a separate 256 Hz clock.
For rhythm games or long scripted music synchronization, choose the intended
clock explicitly. This is a contract/design observation, not a demonstrated
timing bug. The older discussion's PCM buffering advice is future-facing:
production Audio v2 is PSG-based.

## 10. Inspect emitted ARM code before adding clever micro-optimizations

**June 24, 2025:** PinoBatch points out that certain constants can be formed from
two rotated immediates instead of fetched from a literal pool.
[ARM constant example](https://discord.com/channels/768759024270704641/829850171151876127/1387068837584699515).

**mTile application:** this is a candidate for measured hot assembly paths,
not a global source rewrite. ROM versus IWRAM placement, instruction mode,
prefetch, and register pressure affect the result. Inspect the linked binary
and benchmark realistic workloads; the compiler may already select a suitable
sequence.

## Suggested follow-up order

1. Prototype deadline-aware CPU sleep and validate cadence/audio coexistence;
   measure physical power use before claiming savings.
2. Benchmark a small immutable 4bpp sprite fixture with exact palette semantics.
3. Compare shared animated BG tiles against repeated sprite overlays.
4. Keep compression, BIOS-copy, blending, and HDMA contracts in future design
   reviews. Do not add those features merely because they appeared in this search.

Any public Lua API or renderer-eligibility change must satisfy the existing
`AGENTS.md` classification, documentation, parity, and benchmark gates.
