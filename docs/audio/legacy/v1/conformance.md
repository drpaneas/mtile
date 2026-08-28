# Legacy PICO-8 audio conformance

> Archived research evidence. PICO-8 compatibility is not a native GBA audio
> v2 release requirement.

## Scope and sign-off state

This gate compares Mtile cartridge output with PICO-8 0.2.6b across the
complete PICO audio surface, in this order:

1. all 8 instruments at pitches 0..63;
2. all note effects 0..7, including transition direction and short-speed
   arpeggio cadence;
3. `noiz`, `buzz`, both `detune`, `reverb`, and `dampen` levels;
4. complete two-note melodies at every speed 1..255.

The complete external software gate passes 870/870 cases: 512 instrument and
pitch cases, 11 note-effect cases, 92 filter cases, and all 255 speeds.
Physical GBA listening is a separate final gate and is not implied by this
report.

## Supplied PICO-8 cart corpus

The isolated 870-case matrix is complemented by the complete supplied
`Pico-8/` album corpus. The index contains 229 carts (163 text carts and 66 PNG
carts), 289 distinct music graph roots, 155 MP3 references, 156 album-track
records, and 153 cart/reference pairs. The hard audit imports, validates, and
packed-renders every one of the 229 carts and all 289 roots; none fail.

The mastering-tolerant comparison measures active duration, phrasing,
pitch-class content, and log-band timbre. Multipart albums are also compared
cart by cart so short crossfades and edits do not shift every later part. In the
final arranged run:

- median effective chroma similarity is 0.97567;
- median effective band similarity is 0.96772;
- 103/153 pairs are at least 0.95 on both metrics;
- 150/153 pairs are at least 0.90 on both metrics.

The other three aggregate lows are not unresolved engine mismatches:

- Tunes Vol. 1 track 12's supplied `Village.mp3` does not match its cart. A
  fresh internal PICO-8 recording agrees with Mtile at 0.9747/0.9279, while
  the same PICO recording agrees with the supplied MP3 at only 0.652/0.766.
- The Last Bot track 4 is a multipart mastering/locator case. Its direct first
  cart comparison is 0.9750/0.9762 even though the aggregate locator reports
  0.7848/0.8786.
- Celeste track 9's segmented locator chooses a poor multipart boundary, while
  its ordinary full-track comparison is 0.9629/0.9660.

Pico Mix track 13 (`Slipways`) is interactive rather than a static graph walk.
Reference-aligned pattern boundaries recover the authored performance order
`0, 3, 6, 8, 11, 15, 15, 20` and exact loop counts. The reconstructed export is
1956.7805 seconds versus 1956.4141 active reference seconds and reaches
0.9031/0.9414 on the segment-aware comparison. This arrangement is declarative
and regression-tested in the pairing tool rather than hand-edited into a local
report.

The corpus exposed and now covers four interactions absent from isolated
single-note fixtures: custom transposition below pitch 0, fractional low-volume
custom fades, same-pitch changes between custom-instrument IDs, and the
finite-versus-looping retrigger rule at music-row boundaries.

A separate repeated-organ capture covers the interaction deliberately excluded
from isolated fresh-process cases: PICO-8 carries oscillator phase across
ordinary note cells and explicit same-channel retriggers. Mtile now updates
compatible owned voices in place. The generated Zelda capture's PICO-reference
chroma/timbre scores consequently improved from 0.9733/0.9559 to
0.9918/0.9702 while retaining zero sample jumps above 0.10.

## Isolated recording method

PICO-8 exports its own mixer through `extcmd("audio_rec")`; every isolated
instrument pitch starts in a fresh PICO-8 process so oscillator phase from a
previous note cannot enter the reference. The generated Mtile project is
built as a GBA cartridge, played in mGBA 0.10.3, and recorded from mGBA's
internal `mAVStream.postAudioFrame` callback.

Neither side uses a microphone, an operating-system loopback device, or
whole-system recording. YouTube, music players, notifications, and ambient
sound therefore cannot enter either WAV.

PICO-8 and the GBA stream use different sample rates and do not promise the
same oscillator phase. The gate consequently compares physical duration,
fundamental pitch or noise centroid, normalized harmonic/band signature, gain,
effect trajectory, and envelope. It is semantic synthesis conformance, not a
claim that the PCM files are bit-identical.

## Instrument and pitch matrix

Periodic instruments pass when every isolated pitch is within 15 cents of its
PICO recording, normalized harmonic correlation is at least 0.95, and RMS gain
is within 3%. Noise uses two-second observations and compares spectral
centroid within 450 cents, 24-band spectral correlation of at least 0.96, and
gain within 15%.

| Instrument | Pitches | Failures | Worst pitch/centroid delta | Minimum timbre correlation | Median gain |
| --- | ---: | ---: | ---: | ---: | ---: |
| Triangle | 64 | 0 | 10.85 cents | 0.99878 | 1.0027x |
| Tilted saw | 64 | 0 | 11.26 cents | 0.97719 | 1.0027x |
| Saw | 64 | 0 | 11.23 cents | 0.97601 | 0.9977x |
| Square | 64 | 0 | 12.95 cents | 0.98611 | 0.9929x |
| Pulse | 64 | 0 | 12.95 cents | 0.99008 | 0.9992x |
| Organ | 64 | 0 | 9.87 cents | 0.99013 | 1.0070x |
| Noise | 64 | 0 | 445.19 cents | 0.96977 | 1.0004x |
| Phaser | 64 | 0 | 10.11 cents | 0.99806 | 1.0024x |

The complete matrix is 512/512 passing isolated comparisons. Every row was
added only after its waveform produced a complete 64-case `result.json` with
no failures.

## Note effects

The transition-aware fixture covers `none`, upward and downward `slide`,
`vibrato`, `drop`, both fades, both arpeggios, and both arpeggios again at
speed 8. Pitch trajectories require at least 0.94 correlation and at most
0.35-semitone RMSE; fade and slide envelopes require at least 0.95 correlation
and at most 0.12 normalized RMSE.

All 11/11 cases pass. The weakest pitch correlation is 0.97387 (vibrato), the
largest pitch RMSE is 0.31968 semitone (slide up), the weakest measured
envelope correlation is 0.99939, and the largest envelope RMSE is 0.01083.
The external trajectory fixture uses one harmonic-rich periodic source because
these effects modify the shared pitch/gain timeline rather than waveform
synthesis. A target-core cross-product regression separately runs every one of
the 8 instruments with every one of the 8 effects to completion.

## Filters

Filter work is measured independently from the instrument matrix because
`buzz` and `detune` have instrument-specific synthesis paths while reverb and
dampen are post-synthesis channel filters. The consolidated gate contains:

- all 64 noise pitches with `noiz`, isolated by process;
- `buzz` on all 8 instruments;
- `detune` levels 1 and 2 on all 8 instruments;
- `reverb` levels 1 and 2 on an isolated triangle note and tail;
- `dampen` levels 1 and 2 on a long saw note.

All 92/92 cases pass across the 29 groups, including the special
triangle/organ/phaser detune paths and both stateful-filter levels. Reverb
level 1 matches PICO-8's measured 33/64 feedback while level 2 uses 1/2. The
restart-safe runner writes a machine-readable `summary.json` and refuses to
sign off missing or failed groups.

## Complete speed range

The speed fixture plays the same two-note melody at every integer speed from 1
through 255. A case passes only when duration differs by at most 5 ms, the note
transition by at most 4 ms, both stable pitches by at most 18 cents, and gain
by at most 5%.

All 255/255 speeds pass. Across the complete range, the worst absolute duration
delta is 2.595 ms, the worst transition delta is 3.552 ms, the worst low-note
pitch delta is 16.144 cents, and the worst high-note delta is 11.556 cents.
This verifies the exact PICO export clock of 183 samples at 22,050 Hz per speed
unit and Mtile's retained `499712/3675` target-sample ratio.

## Reproduce

The commands below create temporary PICO carts, generated Mtile projects,
ROMs, both internal recordings, per-case JSON, and aggregate summaries. They
require a local licensed PICO-8 installation and mGBA with GDB support.

```sh
scripts/audio/run_pico_instruments.sh /tmp/mtile-pico-instruments
scripts/audio/run_pico_effects.sh /tmp/mtile-pico-effects
scripts/audio/run_pico_filters.sh /tmp/mtile-pico-filters
scripts/audio/run_pico_speeds.sh /tmp/mtile-pico-speeds
scripts/audio/check_pico_conformance.py \
  --instruments /tmp/mtile-pico-instruments \
  --effects /tmp/mtile-pico-effects \
  --filters /tmp/mtile-pico-filters \
  --speeds /tmp/mtile-pico-speeds \
  --json /tmp/mtile-pico-conformance.json
```

All runners fail nonzero on a comparison failure. Instrument and filter runs
are restart-safe: already complete passing groups are retained, partial
isolated references resume in place, and failed single-capture filter attempts
are archived before a clean retry.

The checked-in aggregate from the completed run is
[`captures/pico8-conformance-summary.json`](../../captures/pico8-conformance-summary.json).
The concise supplied-corpus record is
[`captures/pico8-corpus-summary.json`](../../captures/pico8-corpus-summary.json).

To reproduce the cart corpus locally:

```sh
python3 scripts/audio/inventory_pico8_corpus.py \
  --converted-dir /tmp/mtile-pico8-carts Pico-8 \
  /tmp/mtile-pico8-inventory.json
python3 scripts/audio/pair_pico8_corpus.py \
  /tmp/mtile-pico8-inventory.json /tmp/mtile-pico8-pairs.json
python3 scripts/audio/run_pico8_corpus.py \
  /tmp/mtile-pico8-pairs.json /tmp/mtile-pico8-run
python3 scripts/audio/audit_pico8_corpus.py \
  /tmp/mtile-pico8-pairs.json /tmp/mtile-pico8-audit
```
