# DMA alignment follow-up

Native comparison uses the same expanded fixture with 20 source/destination
alignment cases. The prior chunked-DMA implementation fails 18 cases. The
candidate passes every case, together with count guards, Fill32 checks and
Timer0 overflow delivery.

- Byte sources at offsets 1/2/3 are now assembled into exact 32-bit CPU writes.
- Destinations at offsets 1/2/3 are rejected before touching memory, for both
  byte- and word-slice helper APIs.
- Host behavior follows the same contract; new host tests check every byte and
  surrounding sentinels.
- Generic aligned copies: 8 words 199 -> 213 cycles (+14), 9,600 words 77,047 ->
  77,087 (+40), measured with matching fixtures. The eight-word unaligned-source
  fallback costs 331 cycles versus 165 for the aligned specialized call. No
  speedup is claimed from incidental fixed-call code-layout changes.
- The complete runtime test suite passes.

Keep the correction. All reviewed renderer destinations are aligned VRAM/OAM
bases plus word-sized tile/map/page offsets. The fallback preserves callers
that pass byte slices whose storage is not word aligned.

Source files to copy from the isolated dma workspace:
`runtime/hw/mmio_tinygo.go`, `runtime/hw/mmio_host.go`,
`runtime/hw/copy_bytes.go`, `runtime/hw/mmio_host_test.go`,
`scripts/dma-check/check.py`, `scripts/dma-check/fixture.go`, and
`scripts/dma-check/README.md`.

Evidence: `prealignment-check/report.json` (18 expected failures),
`alignment-check/report.json` (zero failures), with ELF/ROM/disassembly and
native logs retained in both directories.
