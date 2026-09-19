# Idle, blend, and HBlank trials

These are opt-in, emulator-only research trials. They do not install a runtime
optimization or add a public Lua API. `idle-candidate.patch` is a **rejected**
prototype. Apply it only to an isolated copy.

From the repository root, after building the existing headless player:

```sh
python3 scripts/discord-hardware/idle-video/run.py video --out /tmp/mtile-video-trial
python3 scripts/discord-hardware/idle-video/run.py sleep --out /tmp/mtile-sleep-trial
```

Use an output directory outside the source tree. The sleep command copies the
repository, borrows `build` and `third_party` through read-only symlinks, builds
baseline/candidate TinyGo ROMs, runs the mandatory Go package gates, and runs
three paired game captures using the existing headless-player protocol. It
records the paired game's nonzero exit status as an experimental rejection.
It never patches the source repository. Go/TinyGo need access to their usual
build caches; the fixture uses TinyGo 0.42.0's ARM GBA target.

`capture.c` samples the emulator's CPU halted state every 127 cycles while
emulator idle-loop removal is disabled. This is an idle CPU proxy, not a power
or battery measurement. The 650-display-frame microprobe covers entry into the
presentation wait at each of 228 scanlines, including all current-VBlank cases,
and 120 two-scan schedules with early and late work. The 1,200-display-frame
audio run uses the production-shaped four-channel PSG coexistence cartridge.

The sleep candidate borrows the VCOUNT comparator only if it is not owned,
arms a wake before the deadline, uses a BIOS Halt wrapper with an ARM ABI call
boundary, and polls the remainder. It avoids BIOS IntrWait's shared flag latch:
the pinned TinyGo interrupt dispatcher writes `BIOS_IF` separately for each
source rather than accumulating it. The candidate still fails the broad native
gate: background-layers Mode4 stops publishing after its first frame. The
baseline continues. This is sufficient to reject it; the exact cause of that
failure has not been isolated. Positive microbenchmarks do not override it.

The `video` command uses five blend/window controls and four HBlank
source-lifetime controls. It compares the lower 24 RGB bits (mGBA places
internal backdrop flags in the high byte). Alpha controls use this emulator's
eight-bit blend output, so they establish which surface participates rather
than asserting hardware's exact five-bit quantization. WIN0 enables OBJ and
effects but masks BG0 inside the window, leaving the red OBJ opaque; outside,
BG0 is enabled and participates in the yellow blend. Windows restrict existing
layers; they do not create an OBJ-over-OBJ alpha target.

HBlank cost is the sum of mGBA DMA event `when` deltas for actual halfword
transfers. It excludes table generation, CPU-side setup, and initial trigger
latency. The table fixtures intentionally use constant red/blue entries to
make missing source rewind and mid-frame mutation unambiguous. The stable
variant resets the source during VBlank and keeps the active table immutable.
The double-buffered variant writes the inactive table and swaps at VBlank.

Decisions and measured data are in
`docs/benchmarks/discord-hardware/idle-video/summary.json`. E12 retains a proven
hardware limitation, E13 retains a safe research pattern, and both defer API
integration until an actual mTile scene demonstrates an overall benefit.
E14 preserves the existing nominal simulation clock: 30 updates equal one
`time()` second, while their 60 LCD scans occupy about 1.00456 wall seconds.
