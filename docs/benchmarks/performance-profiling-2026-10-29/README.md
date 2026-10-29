# Physics and streamed-map profiling, 2026-10-29

See the [investigation and results](../../performance-improvements-2026-10-29.md)
for causes, fixes, remaining limits and the comparison with the original Butano
baseline. This record tests internal mTile changes against unchanged Lua/assets.

- [Summary](summary.json): 38 workloads, 74 renderer cases, 148 captures and
  88,800 measured ticks. Every pixel hash and state checksum passes the
  independent oracle; both repetition medians agree for every case.
- [Full before/after results](results.json) and [flat comparison](comparison.csv).
  Ten cases improve; the other 64 retain exactly the same median/p95/maximum.
- [Oracle checks](verification.json), [Go gates](checks.json),
  [toolchain](toolchain.json), and the exact [engine patch](engine.patch).
- Profiles for physics 64: [before](profiles/physics_64/before/profile.json),
  [after](profiles/physics_64/after/profile.json). Profiles for diagonal chunk
  crossing: [before](profiles/scroll_chunk_cross/before/profile.json),
  [after](profiles/scroll_chunk_cross/after/profile.json). Percentages of useful
  instruction cost are exclusive inline-leaf attribution; event cycles remain
  separate. The after profiles use exactly the same ROM hashes as the recheck.
- [Raw evidence](evidence.tar.gz): per-tick captures, source/asset hashes,
  per-PC counters, DWARF inline stacks and hashes identifying the locally built ROM/ELF pairs.
  [SHA-256 manifest](sha256.json) covers the records in this directory.
- [Local-array diagnostic control](local-array-control/summary.json) and
  [its Lua source](local-array-control/game.lua). This control rewrites four
  bindings and is separate from the unchanged-source engine results.

Mode 0 maintains two-refresh cadence in 35/36 compatible cases (previously
33/36); Mode 4 in 22/38 (previously 20/38). Physics with 128 global-array entities
still exceeds the 561,792-cycle budget. These are pinned-mGBA measurements;
physical GBA validation remains outstanding.

The generic runtime tests, Mode 0 tests with one/four backgrounds and focused
forced-streaming pixel tests pass. A whole-suite forced-streaming run also
includes resident-only generic fixtures: their existing initialization failures
were reproduced from the untouched baseline and saved in the
[baseline log](known-forced-streaming-fixture-failures.log).

Reproduce with [`profile.py` and `recheck.py`](../../../scripts/butano-bench/README.md).
The original baseline measurements and Butano ROMs were preserved. The original
report's numeric-array description was corrected after generated-code inspection;
none of its measured data was changed.
