# ARM division gate

This optional tool assembles the production division helpers for ARM7TDMI and
executes them in the repository's mGBA core. It compares exact quotient and
remainder results with independent host arithmetic and measures cycles against
the frozen restoring-loop implementation in `fixtures/restoring.s`.

Requirements: Python 3, GCC or Clang with `__int128`, ARM GNU Binutils, and the
headless mGBA library built through Mtile's existing CMake project. Run from the
repository root on macOS or Linux:

```sh
cmake -S cmd/mtile-player -B build/player -DCMAKE_BUILD_TYPE=Release
cmake --build build/player --target mtile-player
python3 scripts/division-check/check.py --output-dir /tmp/mtile-division-check
```

`--mgba-source`, `--mgba-build`, `--tool-prefix`, and `--cc` select other local
installations. `--candidate` and `--baseline` accept assembly files containing
the IWRAM division sections. Without `--output-dir`, build artifacts are
temporary. Retained artifacts include both ELF files, symbol addresses,
disassembly, text results, compiler versions, and source/library hashes.

The gate checks 3,012,900 operand pairs through all four signed/unsigned 32/64-bit
entrypoints, running both baseline and candidate for 6,025,800 ARM executions:

- A cross-product of 635 edge inputs: zero, one, small values, and every positive
  and negative power of two with offsets from -2 through 2.
- 100,000 deterministic full-width, small-divisor, and power-of-two pairs per
  width and sign; 32-bit power-of-two divisors stay within 32 bits.
- 100,000 fixed-point-shaped 64-bit pairs per sign, matching the widening
  numerator used by `Number.Div`.

Every call checks quotient, remainder, callee-saved registers, restored SP,
and return state for ARM or Thumb callers. Initial condition flags vary.
Candidate calls additionally require eight-byte stack alignment at every BL.
The baseline retains its original frames for comparable historical timings.
The host reference widens signed division to cover minimum-value divided by
negative one without C overflow. Division by zero must return zero outputs.

The command fails on incorrect results, ABI violations, excessive instruction
counts, or any sampled cycle regression against the selected baseline. Cycles
cover isolated helper execution and internal calls with both versions in
IWRAM. They exclude caller setup, interrupts, DMA contention, and whole-game
effects. This comparison measures algorithm improvements independently of the
earlier move from ROM to IWRAM.

The candidate normalizes the divisor in bounded shifts, computes only the
quotient bits that can be nonzero, and commits the proven leading bit directly.
Zero, one, smaller-dividend, and one-bit-quotient cases return early. The 32-bit
power-of-two path uses a shift and remainder mask. Signed wrappers keep both
result signs in one saved register, preserving aligned call frames.
