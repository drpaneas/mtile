# LZ77 asset-compression research

This optional native mGBA experiment compares raw ROM copies with BIOS LZ77
WRAM decompression. It does not enable compression in the asset pipeline or
add a public Lua API.

```sh
python3 scripts/compression-check/check.py --output-dir /tmp/mtile-lz77-check
```

Requirements: Go, TinyGo 0.42, ARM GNU Binutils, a C compiler, and the existing
`build/player/mgba/libmgba-core.a`. The output directory must be new.
`--runtime`, `--mgba-source`, and `--mgba-build` select alternate inputs.

The checker copies the runtime and inserts the diagnostic BIOS wrapper into
that temporary copy. It generates deterministic redundant and incompressible
16-KiB sheets. Each is encoded once as a whole sheet and again as 64 separate
256-byte animation frames. Reports include padded ROM payload sizes, raw-copy
cycles, validation cycles, BIOS cycles, end-to-end validated-decode cycles,
frame decode costs, byte parity and destination sentinels.

The experimental wrapper checks expanded capacity, stream bounds, valid
backreferences, token output bounds, and source/destination alignment before
calling BIOS code. Undersized destinations, including a redundant asset
buffer sized from the compressed payload, must remain entirely untouched.
Seven malformed streams also must be rejected before writes.

Timing uses mGBA's HLE BIOS and is not physical-device signoff. ROM sizes
exclude a possible frame-offset table and the decoder's code. The whole-sheet
case needs 16 KiB of expanded working storage; individual frames need 256 bytes.
The test demonstrates why an asset policy must weigh ROM savings against CPU,
RAM, latency, and random access instead of enabling compression universally.
