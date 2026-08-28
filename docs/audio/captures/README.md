# Audio captures

## Status

Files at this directory's root are legacy audio-v1 evidence. Native CH1–CH4
evidence lives only under [`v2/`](v2/README.md). Legacy PICO/Direct Sound
captures must not be cited as native-v2 conformance or hardware sign-off; the
v1 specification is archived in [`../legacy/v1/`](../legacy/v1/).

`pico8-conformance-summary.json` is the strict machine-readable aggregate from
the completed PICO-8 0.2.6b versus mGBA 0.10.3 internal-audio gate. It records
870/870 passing cases: 512 instrument/pitch, 11 note-effect, 92 filter, and 255
speed comparisons. The licensed PICO-8 reference WAVs remain local; the JSON
contains measurements and no copyrighted audio.

`mgba-arpeggio-conformance.wav` is deterministic emulator output from
`examples/audio-arpeggio-conformance/game.gba`. It was captured from mGBA
0.10.3's internal `mAVStream` audio callback as stereo signed 16-bit PCM at the
GBA sound-bias rate of 32,768 Hz. It is not a microphone, loopback-device, or
whole-system recording.

The file contains 197,038 stereo frames (6.013 seconds) and has SHA-256
`cab9500bb47ee45288b66c7d3f5a722393559fefcbe07ec61aac2d90d558f95e`.

The file contains one six-second cartridge cycle: four-step fast, four-step
slow, eight-step fast across an aligned group boundary, then eight-step slow
across the same boundary. See
[`../legacy/v1/conformance.md`](../legacy/v1/conformance.md) for the
comparison with local PICO-8 reference recordings.

`mgba-stereo-routing.wav` is deterministic output from
`runtime/cmd/audiostereocapture`. The ROM sends a 512 Hz square to FIFO A/left
and a 768 Hz triangle to FIFO B/right using the provisional two-voice profile.
The file contains 65,368 stereo signed 16-bit frames at 32,768 Hz and has
SHA-256
`a0ced77f9207289744d8ddce8b5c85e40c5fc76be8ebfef6a24a62f496320eb0`.

The analyzer aligns the 3,184-frame startup prefix and compares one settled
second with a host render from the exact profile package used by the ROM. Both
own-channel correlations are 1.000000; both swapped correlations and the
recorded inter-channel correlation are 0.000000. Captured/reference gains are
1.000000 on both sides, and the full settled capture contains no paired silent
frames. A second independent capture produced the same SHA-256.

Reproduce the ROM build, internal mGBA callback capture, reference render, and
analysis with:

```sh
MGBA_SOURCE=/path/to/mgba-0.10.3 \
  scripts/audio/extract_audiostereocapture.sh
```

The capture utility uses a temporary ad-hoc-signed copy of the installed mGBA
0.10.3 executable so the checked-in callback shim can load; it does not modify
the installed app or observe microphone, loopback, or whole-system audio.

## Native-GBA v2 captures

The direct-register suite is documented in
[`../reference-roms.md`](../reference-roms.md). Active evidence under
[`v2/mgba/`](v2/mgba/) contains only the CH1, CH2, CH3, and CH4 entries listed
by `reference-captures.json`. Only captures tied to those truth ROMs or the
production v2 engine can become v2 truth-table evidence.
