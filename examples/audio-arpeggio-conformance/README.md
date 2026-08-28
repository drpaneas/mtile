# Arpeggio conformance

> **Legacy audio v1 fixture.** This project remains runnable while native-GBA
> audio v2 is developed, but it is not a v2 example or release gate. See the
> [archived v1 documentation](../../docs/audio/legacy/v1/).

This deterministic cartridge is the target half of the local PICO-8/mGBA
arpeggio comparison. It plays four square-wave SFX on logical channel 0:

- 0.5 s: four-step fast arpeggio
- 1.5 s: four-step slow arpeggio
- 2.5 s: eight-step fast arpeggio across an aligned group boundary
- 4.0 s: eight-step slow arpeggio across an aligned group boundary

The six-second cycle needs no controller input, so an emulator recorder can
capture repeatable output. Build it from the repository root:

```sh
go run ./cmd/mtile build examples/audio-arpeggio-conformance/
```

The checked-in mGBA capture and measured PICO comparison are documented in
[`docs/audio/legacy/v1/conformance.md`](../../docs/audio/legacy/v1/conformance.md).
