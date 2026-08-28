# Four-channel conformance

## Truth ROMs

Active release gates:

```text
audioref-ch1  duty, sweep, envelope
audioref-ch2  duty and left/right routes
audioref-ch3  32/64 samples, banks, 25/50/75/100% levels
audioref-ch4  7/15-bit noise, clocks, envelope, routes
```

The FIFO A/B/all ROMs are retained research and are not Audio v2 gates.

## Three implementations

1. Direct-register truth ROM.
2. Managed target controller and generated cartridge.
3. Host/editor preview of the same compiled event semantics.

For each CH1–CH4 matrix case compare register-equivalent state, event timing,
fundamental/noise class, envelope/sweep contour, routing, stop, and restart.

## Host gates

- Exact register words for every accepted control.
- Exhaustive discrete-field encoding: every duty, route, sweep pace/direction/
  shift, envelope initial/pace/direction, wave level/bank, and accepted noise
  divisor/shift/width.
- `SOUNDCNT_X.7` is written before the first nonzero PSG register write after
  cold initialization, matching real-hardware access rules.
- Invalid request causes no write.
- Shared route/level mutation preserves unrelated channels.
- Safe CH3 bank loading and nibble order.
- Rational timing and long-loop drift.
- Exact 96-PPQN straight/triplet/dotted timing, straight-grid swing, and
  independent piano-roll snap versus recording quantization.
- Common/custom meter round trips and per-part tempo boundaries, including
  song-tempo inheritance and return to the 120 BPM idle default.
- Note-off, legato, part transitions, queue overflow, and owner policy.
- Strict schema unknown-field/kind/range rejection.
- Preview and target event trace equality.
- MONO/POLY2/POLY3 held-note restoration and exact physical lane recording.
- Raw-take re-quantization, parameter automation, loop boundaries, count-in,
  metronome, overdub/replace, and hotplug all-notes-off.
- Launchkey protocol golden bytes, input routing, relative encoders, LEDs/OLED,
  connection, and disconnect recovery through a fake CoreMIDI backend.
- Headless single-workspace layout bounds for composer, inspector, transport,
  take selector, and song order.
- Channel-aware pitch navigation plus rectangular multi-note copy, paste,
  move, delete, collision, and bounds behavior.

## Emulator and hardware gates

Capture every truth and managed case in pinned mGBA, then repeat on physical
GBA. Test simultaneous CH1–CH4, left/right routes, repeated start/stop, long
loops, and graphics coexistence. The active direct-register set contains 42
audible/silent cases: 12 CH1 plus 10 each for CH2–CH4. A manual PASS records
listener, date, device, cartridge/hash, route, and notes.

Capture silence is evaluated from AC energy rather than raw sample magnitude,
so a constant CH3 DAC hold level after the hardware length counter expires is
correctly classified as silent.
