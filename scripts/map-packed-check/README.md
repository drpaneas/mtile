# Native packed-map span check

This optional gate executes the actual ARM7TDMI helper in mGBA. It checks 18,754 deterministic cases against an independent byte oracle: copy/fill mixtures, all uniform colours, clipping, all destination/source-relative alignments, span lengths 0–260 and 32767, random batches, large source coordinates, guard bytes, source and span immutability, saved registers, stack balance, and ARM/Thumb returns. Memory callbacks reject every source load that extends outside a single span; padding is not readable test data.

Dependencies are the same as `scripts/division-check`: an `arm-none-eabi-` assembler/linker, a host C compiler, and the mGBA static library produced by the player build.

```sh
python3 scripts/map-packed-check/check.py --output-dir /tmp/mtile-map-packed-check
```

The retained directory contains the extracted assembly, ELF, disassembly, binary, test report and tool/library fingerprints. `--candidate`, `--mgba-source`, `--mgba-build`, `--tool-prefix`, and `--cc` override their repository defaults. The helper has a 800-byte size limit. Cycle measurements are diagnostics; the deterministic Celeste route supplies the end-to-end performance comparison.
