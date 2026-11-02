# Imported Direct Sound verification — November 2, 2026

This verifies the new importer and GBA playback path. It is **not a Butano
performance comparison**, a claim that all tracker features have been audited,
or physical-hardware listening sign-off. The formats and player correspond to
[Butano's Direct Sound import path](https://gvaliente.github.io/butano/import.html).

## Reproduction

The checked-in example contains an original MOD melody and WAV bell plus their
imported bank. Building it does not require mmutil:

```sh
MTILE_IMPORTED_AUDIO_INTEGRATION=1 \
MTILE_PLAYER=build/player/mtile-player \
go test ./internal/emulator -run '^TestImportedAudioROMConformance$' -count=1 -v
```

The test builds actual debug cartridges in both renderers and captures 240 LCD
refreshes with the repository's headless mGBA 0.10.3 helper. It checks:

- a non-silent imported MOD song;
- the 880 Hz WAV bell in the recorded signal;
- a simultaneous 440 Hz CH1 PSG note, verified spectrally and in hardware state;
- Timer 1 routing for both Direct Sound FIFOs while PSG uses Timer 0;
- stable two-refresh game update intervals after startup;
- a silent tail after stopping both imported music and effects; and
- exact Mode 0/Mode 4 pixels for the fixture.

Final results after heap-free audio-call classification:

| Fixture | Mode 4 | Mode 0 |
| --- | ---: | ---: |
| ROM bytes | 77,280 | 77,472 |
| EWRAM free | 249,540 | 249,308 |
| IWRAM used | 23,876 | 23,780 |
| Updates observed, including startup | 118 | 118 |
| Post-stop PCM RMS | 0 | 0 |

These resource figures describe this small fixture only. A large game and a
busy 32-channel module have different costs. PCM is serviced at LCD frequency,
not once per Lua update. The player uses fixed C/Go buffers; no playback
operation allocates in the Lua heap. Unused imported assets omit the player.

## Input formats and converter variants

The converter was pinned to BlocksDS mmutil
`afd0270861d91d5a45ee92839b8047a48a008202`. The vendored Maxmod GBA player is
from Butano `04374b034c76b82bfdc3aa6c38d652e5e7778a68`; local adaptations and
license are under `runtime/maxmod/`.

A second ROM imported and played these four inputs, switching every 45 game
updates. Each capture window contained 45 LCD refreshes at 48 kHz stereo:

| Format | Source used for this verification | RMS, signed PCM16 |
| --- | --- | ---: |
| MOD | Original mTile `theme.mod` fixture | 5,084.1 |
| XM | Butano `examples/audio/audio/mj_totsnuk01.xm` | 2,420.1 |
| S3M | Butano `examples/core/audio/soda7_xcopy_ohc.s3m` | 5,372.0 |
| IT | Butano `examples/audio_player/audio/battle_clean.it` | 597.3 |
| Stopped | All modules stopped | 0 |

The Butano music files and their recordings are **not redistributed here**.
These RMS values establish non-silent output and transport progress, not
musical quality or bit-identical output against a separate Butano ROM.
`results.json` retains the source pins and capture summaries.

Both current MAS `0x19` and the devkitPro-compatible MAS `0x18` layout were
imported, compiled and played. The latter also exercised the example's WAV
and PSG buttons, then stopped the module: its final half-second RMS was zero.
The matching player layout is selected from the validated bank version at
build time; no runtime format guessing is used.

## Other gates

The required transpiler/build/CLI tests, default runtime tests, importer and
asset tests, IDE tests, 14 website tests and four release-bundle tests pass.
Mode 0's four-layer runtime tests and the CI's focused streaming-map suite
also pass. An unrestricted combination of all streaming tags is not used for
the ordinary compact-map fixtures; the CI selects compatible tests explicitly.

Physical GBA playback, worst-case song CPU load, and tracker effect-by-effect
comparison remain separate checks. General CPU and memory budget requirements
continue to apply to imported audio.
