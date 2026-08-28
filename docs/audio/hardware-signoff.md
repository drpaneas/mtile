# Physical GBA sign-off

## Required setup

Record console/model, flash cartridge, headphones/speakers, build commit, ROM
SHA-256, TinyGo version, date, and listener. Use the same ROM hashes captured
in mGBA.

Build the hash-verified PSG-only handoff bundle with:

```sh
scripts/audio/build_hardware_signoff.sh
```

The set contains `audioref-ch1` through `audioref-ch4`, the managed
`audiopsg` cartridge, and `audiopsgcoexist`. It never builds a Direct Sound,
FIFO, or PCM cartridge. By default the bundle is written to
`build/audio-hardware-signoff/` and contains `SHA256SUMS`, `BUILD-INFO.txt`,
`CASES.json`, and a ready-to-fill `RESULTS.md`. Recheck an existing bundle with
`scripts/audio/verify_hardware_signoff.sh`.

After the physical run, check each completed line, replace each `PASS / FAIL`
with exactly `PASS` or `FAIL`, select exactly one final decision, and run:

```sh
scripts/audio/verify_physical_signoff.py build/audio-hardware-signoff
```

This is the authoritative physical gate. It rejects missing listener/device
metadata, a changed or incomplete 42-case matrix, unchecked stop/restart or
managed-controller checks, any explicit failure, and an ambiguous final
decision. The ordinary bundle verifier accepts a blank or partial form but
still validates its structure.

## Matrix

For both direct-register and managed-controller cartridges verify:

- CH1: 12 cases covering every duty, both sweep directions, slow/fast and
  small/large sweep representatives, envelope down/up, and left/right;
- CH2: 10 cases covering every duty, envelope down/up, hardware length, and
  left/right/both;
- CH3: 10 cases covering 32/64 modes, both start banks, nibble order, hardware
  length, and every output level including mute;
- CH4: 10 cases covering 7/15-bit width, divisor/shift endpoints, envelope
  down/up, hardware length, and left/right;
- each channel stop/restart;
- all four channels simultaneously;
- song loops and part transitions for at least ten minutes; and
- sprite-heavy and dense-map gameplay coexistence.

Listen for missing notes, wrong pitch, route reversal, clicks on normal
trigger/stop, stuck notes, unexpected reset, timing drift, and graphics/audio
interference.

Cold-boot `audiopsg.gba` before any other audio ROM and confirm its first note
is present. This specifically verifies the real-GBA rule that `SOUNDCNT_X.7`
must be enabled before the first PSG register write. On
`audiopsgcoexist.gba`, wait for the visible `SELF TEST: PASS` and
`HW CHANNELS SEEN: 15`, then leave it running for ten minutes while listening.
The score remains live for the entire soak and permanently latches `FAIL` if a
later cycle-budget breach, queue error, rejected command, or frame skip occurs.
The repository's automated preflight runs the same ROM unthrottled until at
least 153,600 scheduler services (ten minutes at 256 Hz) have completed; this
does not replace the real-time listening run on physical hardware.

## PASS record

Fill the generated bundle's `RESULTS.md`; it lists every direct-register case,
stop/restart, cold-start, visible self-score, ten-minute run, listener/device,
and recording paths. A physical PASS is valid only when every checkbox is
completed, `verify_physical_signoff.py` reports PASS, and the ROM hashes still
pass `verify_hardware_signoff.sh`.

FIFO/PCM is not tested because production Audio v2 does not use it.
