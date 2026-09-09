# Mtile audio v2

Audio v2 is a small, native Game Boy PSG music system for the Game Boy
Advance. It exposes exactly four hardware channels:

- CH1 pulse with sweep;
- CH2 pulse;
- CH3 programmable wave; and
- CH4 noise.

The GBA generates these voices in hardware. Mtile performs bounded register
updates at 256 Hz, so production audio has no software sample mixer, FIFO
stream, audio DMA, PCM samples, or underruns.

Start here:

- [`plan.md`](plan.md): active product and engineering plan.
- [`schema-v2.md`](schema-v2.md): strict four-channel asset contract.
- [`api-v2.md`](api-v2.md): beginner and native APIs.
- [`gba-hardware.md`](gba-hardware.md): the four real channel capabilities.
- [`editor.md`](editor.md): the single-screen Music Lab and performance model.
- [`conformance.md`](conformance.md): emulator and hardware verification.
- [`completion.md`](completion.md): current audit.

Direct Sound/PCM is unsupported. The superseded software mixer, PICO audio
import/conformance tools, and audio-v1 fixtures have been removed. Existing v1
assets are rejected explicitly; they are never silently converted or overwritten.

Hand-written Go cartridges should bind an `audio.Controller` and attach it
with `Context.AttachNativeAudio`. The old `AttachSFX`, `AttachAudioStream`,
software-player helpers, and compiler audio-v1 options no longer exist.
The supported Lua PSG API is unchanged; see [API v2](api-v2.md).

Celeste's source-decoding helper remains with that port's conversion tests.
It converts source material into PSG arrangements and is not part of the
compiler, editor, or embedded runtime.
