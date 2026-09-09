# Native GBA audio v2 captures

This tree is reserved for evidence produced by the four direct-register Game
Boy PSG reference ROMs and the production v2 engine. The authoritative active
set is the CH1–CH4 list in `mgba/reference-captures.json`. Superseded FIFO and combined-reference captures have been removed.

`mgba/` contains deterministic internal mGBA 0.10.3 callback captures. Physical
hardware recordings must use a separate directory named for the tested GBA
model and must include the sign-off metadata required by
[`../../hardware-signoff.md`](../../hardware-signoff.md).

Rebuild, capture, and validate the complete emulator truth set with:

```sh
scripts/audio/capture_gba_reference_audio.sh
```
