# E11: bounded BIOS LZ77 asset trial

Decision: **drop default runtime/asset-pipeline integration; keep the optional
research checker**. No existing game demonstrated ROM pressure that offsets
these measured CPU/RAM costs. Per-frame compression can be useful for a
specific future animation workload, but it needs an explicit asset policy.

Native mGBA with HLE BIOS, TinyGo 0.42, forced blank, ROM sources and EWRAM
outputs; no physical-hardware or physical-BIOS timing claim.

| 16-KiB sheet | Raw ROM | Whole LZ77 padded ROM | Raw-copy cycles | Validated LZ77 decode cycles |
| --- | ---: | ---: | ---: | ---: |
| Repeated tile patterns | 16,384 | 1,980 (-87.9%) | 47,291 | 331,600 (7.0x) |
| Deterministic incompressible bytes | 16,384 | 18,436 (+12.5%) | 47,291 | 1,925,305 (40.7x) |

The redundant sheet's BIOS-only decode costs 188,988 cycles; complete stream
validation costs 142,535. The incompressible equivalents are 628,786 and
1,296,442. These figures expose validation cost instead of hiding it outside
the reported safe-call timing.

Separately encoding 64 256-byte animation frames uses 3,840 bytes for the
redundant fixture or 18,688 bytes for incompressible input, before any frame
index. A 64-entry 32-bit offset index would add 256 bytes. Frame decode costs
7,953 or 30,471 cycles, respectively, versus 898 cycles for a raw frame copy.
Decoding all frames costs 508,992 or 1,950,144 cycles versus 57,472 raw-copy
cycles. The whole-sheet case needs 16 KiB of expanded working storage; one
independent frame needs 256 bytes. Current immutable raw assets can remain
ROM-resident, whereas decoding them requires a destination buffer.

All whole-sheet and frame byte comparisons and prefix/suffix sentinels pass.
The wrapper validates expanded length, destination capacity, source bounds,
backreferences and token output bounds before calling BIOS. A buffer sized to
the redundant compressed payload (1,980 bytes for 16,384 bytes of output) is
rejected without changing any byte. An output one byte too small is likewise
rejected. Seven malformed streams also leave all destination memory untouched:
wrong signature, zero expanded size, truncated header/literal/token, a
backreference before output start, and a token that overruns expanded size.

Reproduce from the repository root:

```sh
python3 scripts/compression-check/check.py --output-dir /tmp/mtile-lz77-check
```

Integration consists only of `scripts/compression-check/{README.md,check.py,
check.c,fixture.go}` from the isolated dma workspace. The checker generates
its data and a temporary runtime, so the BIOS assembly wrapper never changes
production crt0. Native report and full executable artifacts:
`/private/tmp/mtile-discord-experiments-ofgo5qwq/dma-results/compression-final/`.
The report has zero failures and records compiler version and mGBA hash.
