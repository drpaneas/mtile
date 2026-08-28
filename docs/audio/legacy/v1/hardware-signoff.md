# Legacy audio v1 physical-GBA sign-off

> Historical Direct Sound sign-off procedure. The active PSG-plus-Direct-Sound
> procedure is in `../../hardware-signoff.md`.

## Status

**Pending physical hardware.** Host tests, allocation checks, exact mGBA cycle
extraction, every graphics coexistence matrix, and both strengthened ten-minute
mGBA production-profile soaks pass. Emulator results do not establish audible
continuity, electrical output behavior, or DMA interaction on a real console.

The sign-off is complete only when the exact ROM hashes below have been tested
and the result record is filled with observed facts. Do not copy emulator PASS
results into the physical columns.

## Build and identify the ROMs

From the repository root:

```sh
scripts/audio/build_hardware_signoff.sh
```

The command allocation-checks and prints byte counts plus SHA-256 for:

- `runtime/audiospike.gba`: cold-start, silence, stream, stop, and restart probe.
- `runtime/audiohardwarestandard.gba`: four-synth production profile.
- `runtime/audiohardware.gba`: two-synth-plus-one-looping-PCM production profile.

Current candidate built on 2026-08-23:

| ROM | Bytes | SHA-256 |
| --- | ---: | --- |
| `audiospike.gba` | 29,820 | `50487f263bc892944b5c2099158d6fcf0e9117e7293ee7267c02b38c58fd9719` |
| `audiohardwarestandard.gba` | 58,180 | `6448dbcb508b6bc4479699caf68d1226f50dfdb9b47af3d53aae3c1627baf025` |
| `audiohardware.gba` | 58,860 | `6dd56cb119658bba16bf53a47ace6164542c32b2e2a3066df4c2fed6aa29df09` |

Record the printed hashes in the result table before flashing. Any rebuild
after a code change creates a different candidate and invalidates an older
result.

## Procedure

Use a real GBA-family console and flash cartridge. Disable any flash-cart speed
patches or audio filters. Record the console model, flash cart, power source,
and listening output.

1. Cold-boot `audiospike.gba`. Verify the stream tone starts and its pitch is
   stable while the Mode 4 animation remains correct.
2. Press B for active silence, L to resume the incrementing stream, Start to
   stop/reset/restart, and Select to stop. Repeat the transitions several
   times. Listen for stale bursts, loud terminal samples, clicks, stalls, or
   pitch changes.
3. Cold-boot `audiohardwarestandard.gba`. Do not press controls during the
   timed run. Listen continuously and watch the moving marker until the screen
   reports `PASS` after ten minutes. Any `FAIL`, discontinuity, pitch change,
   or visual corruption fails this profile.
4. Repeat step 3 with `audiohardware.gba`. Confirm both synth voices and the
   looping PCM layer remain audible and stable for the complete run.
5. After each successful timed run, press Start and confirm clean restart, then
   Select and confirm clean stop. A is an intentional starvation test and
   restarts timing when service resumes; exercise it only after recording the
   uninterrupted PASS.

Test with the console speaker and, when the hardware provides it, headphones.
Other music playing on the computer is irrelevant to cartridge generation but
can mask listening observations if the same speakers or headphones are used.

## Result record

| Field | Observation |
| --- | --- |
| Date/time/time zone | pending |
| Tester | pending |
| Console model/revision | pending |
| Flash cartridge/firmware | pending |
| Power source | pending |
| Speaker result | pending |
| Headphone result / not available | pending |
| `audiospike.gba` bytes + SHA-256 | pending |
| Probe cold-start/silence/restart/stop | pending |
| `audiohardwarestandard.gba` bytes + SHA-256 | pending |
| Four-synth visible ten-minute PASS | pending |
| Four-synth audible continuity/pitch | pending |
| Four-synth Mode 4/DMA3 visuals | pending |
| `audiohardware.gba` bytes + SHA-256 | pending |
| Synth+PCM visible ten-minute PASS | pending |
| Synth+PCM audible continuity/pitch | pending |
| Synth+PCM Mode 4/DMA3 visuals | pending |
| Start restart / Select stop | pending |
| Notes or failure reproduction | pending |

When every row is factual and both production ROMs pass, copy the completed
record into this file (or a dated sibling), update
[`plan.md`](plan.md#definition-of-done-for-the-whole-audio-initiative), and
remove the pending-hardware qualification from the release documentation.
