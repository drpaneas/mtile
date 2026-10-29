# Defer indexed-store root arguments

Follow-up to commit `84c6d9a`, measured with unchanged Lua and assets. Move the
nonallocating array-slot store to the generated callsite, before constructing
the variadic root arguments. The original LHS/RHS evaluation and root-budget
analysis remain in place. A separate rooted helper preserves hash insertion,
growth and collection without repeating the array lookup.

Non-numeric keys and numeric keys outside 1..256 cannot use the runtime's
array storage. A cheap caller guard sends them directly to the rooted helper.
Fractional keys within that interval still undergo the runtime's integer test.
No Lua API, renderer eligibility, runtime representation or assets change.

Every result uses 60 warm-up ticks and two independent 600-tick captures.

| Physics | Mode 0 before → after median cycles | Mode 4 before → after median cycles |
| --- | ---: | ---: |
| 32 entities | 172,936 → 116,528 | 198,108 → 139,219 |
| 64 entities | 347,989 → 232,304 | 369,075 → 254,259 |
| 128 entities | 695,189 → 463,856 | 713,971 → 484,339 |
| 64 entities with drag | 344,534 → 234,710 | 367,445 → 258,123 |

128 entities now sustain the target two-refresh cadence (29.864 Hz) in both
renderers. Mode 0 median/p95/maximum are **463,856 / 463,884 / 463,924** cycles;
Mode 4 **484,339 / 484,349 / 484,381**. The 30 Hz budget is 561,792 cycles.
Mode 0 leaves approximately 17.4% of that budget free. Linked EWRAM and IWRAM
usage are unchanged; physics ROMs increase by 1,356–1,516 bytes.

All four physics fixtures and three controls (`baseline`, `mixed_scene`,
`scroll_chunk_cross`) were rebuilt in both renderers: **14 cases, 28 captures,
16,800 ticks**, all matching the independent pixel and state-checksum oracle.
Both repeated medians agree in every case. The six control cases retain exactly
the same median/p95/maximum as commit `84c6d9a`. This is a targeted follow-up;
the preceding full 38-workload run remains a separate historical record.

Two additional Mode 0 probes perform 64 existing hash-slot writes and reads
per update, including the nonallocating array attempt's failure path:

| Hash-backed table keys | Before median | After median | Change |
| --- | ---: | ---: | ---: |
| 257..320 | 180,449 | 165,345 | 8.4% fewer cycles |
| 1..64, but no allocated array slots | 176,270 | 183,260 | **4.0% more cycles** |

The smaller keys require the checked runtime attempt before preparing the
rooted fallback. That caller work has a measured cost on this shape. Both
versions sustain 29.864 Hz; every image and checksum in these **4,800 additional
ticks** matches an independent constant-backdrop/integer-sum reference.
This tradeoff remains visible; there is no universal speedup claim.

The 128-entity instruction profile attributes about 694,894 useful/other
instruction cycles per tick before and 463,555 after. Exclusive leaf cost for
`gluTableSet` disappears from the measured hot loop (previously 81,217 cycles);
`memset` falls from 49,124 to 598 cycles, and `__aeabi_memclr` from 11,392 to
154. Checked table access remains a major hotspot. These are diagnostic
120-update profiles, distinct from the 600-tick release statistics above.

- [Full results](results.json), [oracle checks](verification.json), [hash-probe checks](hash-verification.json),
  [environment](environment.json), [Go gates](checks.json), [engine patch](engine.patch).
- Exact profiles: [before](profiles/before/profile.json), [after](profiles/after/profile.json).
- Probe sources/results: [large keys](hash_large_keys/game.lua),
  [small keys](hash_small_keys/game.lua),
  [large-key results](hash_large_keys/results.json),
  [small-key results](hash_small_keys/results.json).
- [Raw evidence](evidence.tar.gz) retains all capture hashes, per-PC counters,
  DWARF inline frames, generated Go and hashes identifying the locally built ROM/ELF pairs.
  [SHA-256 manifest](sha256.json) covers the records in this directory.

The new compiler regression first failed on the committed implementation.
Additional generated-code tests exercise single evaluation and LHS/key/RHS
order, full tagged values, integer/string/fractional hash fallbacks and a
multi-assignment where an array store precedes hash growth that forces GC.

Reproduce the canonical subset with `scripts/butano-bench/recheck.py`, using
the previous follow-up's raw captures as `--baseline` and the original generated
projects as `--projects`; select
`--only '^(physics_|baseline$|mixed_scene$|scroll_chunk_cross$)'`.
Run `verify.py` on its output. Probe sources can be built with `driver.go` at
the baseline revision and again with the saved engine patch; their expected
checksum is `(tick * 64) % 16001`. All measurements use pinned mGBA; physical
GBA confirmation remains outstanding.

## Review cleanup

Uncached field stores now call `gluTableSetRooted` directly: their keys are
strings, so an array-slot attempt would always fail. The unused `gluTableSet`
wrapper was removed. Regression coverage checks this lowering and exercises
the generated indexed store with a full root stack.

The evidence and engine patch above retain the original measured snapshot.
None of its 14 generated fixtures calls the removed wrapper, and rebuilding
the 128-entity Mode 0 ROM after this cleanup produces identical bytes. The
[cleanup verification](uncached-field-cleanup.json) records that check and
the fresh required gates.
