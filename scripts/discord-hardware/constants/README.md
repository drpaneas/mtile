# ARM constant materialization trial

This optional experiment compares `ldr` from a literal pool with two rotated
ARM immediates that produce the same `0x040000de` value. It uses the repository's
pinned native mGBA library and the runtime's `WAITCNT=0x4317` setting.

From the repository root, after building the headless mtile-player library:

```sh
python3 scripts/discord-hardware/constants/check.py --output-dir /tmp/mtile-constant-check
```

Requires Python 3, ARM GNU Binutils, a C compiler, and
`build/player/mgba/libmgba-core.a`. The retained output directory must be new
and outside the repository. Without `--output-dir`, artifacts are temporary.
`--mgba-source`, `--mgba-build`, `--tool-prefix`, and `--cc` select alternatives.
The script does not download dependencies or alter the input repository.

The assembler emits exactly 24 bytes: two 12-byte routines, including each
return and the literal's data word. The checker injects these at ROM offset
`0x1000`, copies them into IWRAM, and runs each variant 16,000 times. Every call
checks the result, r1-r12, stack pointer, condition flags and ARM return state.
The report retains assembly, object, binary, ROM, disassembly, native checker,
cycle results, compiler versions, and source/library hashes.

The initial trial measured ROM literal/immediate paths at 19/14 cycles and
IWRAM paths at 6/5 cycles. The current TinyGo output already materializes this
DMA register address with two immediates, so no production rewrite follows.
This isolated ARM result does not prove that arbitrary constants, Thumb code,
or whole games benefit. It excludes interrupts/DMA contention and is not a
physical-hardware measurement.
