# Native audio reference ROMs

## Active PSG truth set

```text
runtime/cmd/audioref-ch1
runtime/cmd/audioref-ch2
runtime/cmd/audioref-ch3
runtime/cmd/audioref-ch4
```

These cartridges bypass the managed controller and write GBA registers
directly. Each case advances automatically every two seconds and supports
manual previous/next/restart/stop controls.

| ROM | Cases |
| --- | --- |
| CH1 | 12: four duties, four sweep contours, envelope down/up, left/right |
| CH2 | 10: four duties, envelope down/up, hardware length, left/right/both |
| CH3 | 10: 32/64 mode, both banks, every level including mute, nibble order, length |
| CH4 | 10: 7/15-bit modes, divisor/shift endpoints, both envelopes, length, left/right |

Build all reference ROMs with `make -C runtime audioref-gba`. Capture with
`scripts/audio/capture_gba_reference_audio.sh`; analyze with
`scripts/audio/analyze_gba_reference_capture.py`.

The analyzer measures mean-centred (AC) energy. This matters for CH3: after its
hardware length expires, the wave DAC may hold a constant non-zero sample.
That DC level is silent and must not be reported as a continuing tone.

Every release compares the direct ROM, managed cartridge, and host preview,
then repeats the matrix on physical GBA.

`scripts/audio/build_hardware_signoff.sh` creates a hash-verified PSG-only
bundle with the six required ROMs, a machine-readable case table, build
metadata, and a generated `RESULTS.md` containing all 42 checkpoints.

## Deferred research ROMs

`audioref-fifoa`, `audioref-fifob`, and `audioref-all` were completed while
investigating full GBA audio. They remain reproducible evidence but are not
linked by production Audio v2 and do not gate its release. Their presence does
not imply PCM/Direct Sound support.
