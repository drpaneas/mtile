# Native DMA and alignment regression

Run from the repository root after building the headless mGBA player library:

```sh
python3 scripts/dma-check/check.py --output-dir /tmp/mtile-dma-check
```

Requires Go, TinyGo 0.42, ARM GNU Binutils, a C compiler and the repository's
`build/player/mgba/libmgba-core.a`. The supplied output directory must be new.
`--runtime`, `--mgba-source`, and `--mgba-build` accept alternative source/build
roots. The checker copies the runtime into the output directory and adds a
diagnostic cartridge and BIOS wrapper there; it never changes the input runtime.

The real target code runs in native mGBA. Guard bytes cover all four Fill32
pointer alignments, invalid counts, both copy helper APIs, small DMA transfers,
chunk boundaries, and all 9,600 words of a framebuffer. A Timer1 cascade counts
actual Timer0 overflows independently of its IRQ handler, detecting merged
interrupts at eight phases with both an empty and a 16,384-cycle handler body.
The latter tests the documented audio-service budget, not actual PSG output.

The cycle report compares the production DMA, explicit 4,096-word chunks, and
BIOS CpuFastSet for a forced-blank EWRAM-to-VRAM copy. mGBA's HLE BIOS is used;
CpuFastSet figures do not certify physical BIOS timing. This is a regression
and bus-cost probe, not a power measurement or physical-hardware signoff.
