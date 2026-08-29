# GB Studio preset data

Mtile's built-in Game Boy channel presets adapt instrument names and hardware
parameters from GB Studio's `template.uge` music template.

- Project: [GB Studio](https://github.com/chrismaltby/gb-studio)
- Copyright: 2019–2026 Chris Maltby
- License: MIT

The imported values are traced through GB Studio's current preview path, not
only copied from labels:

- `appData/music/template.uge` is the factory instrument source;
- `hUGEDriver.asm` defines envelope, sweep, note-period, noise-polynomial, and
  subpattern execution semantics;
- hUGEDriver's exact 72-entry register table is retained as the CH1/CH2 source
  of truth. Its tracker note IDs are raw register indices, not conventional
  physical MIDI labels. Mtile stores conventional MIDI pitch, so CH3 indexes
  that table one octave higher for a 32-sample wave and two octaves higher for
  a 64-sample wave. Those offsets compensate the Wave channel's sample clock;
  without them CH3 sounds one or two physical octaves too low;
- GB Studio auditions an instrument by building a temporary song at C5 and
  running hUGEDriver through binjgb for 500 ms; and
- Mtile stores the resulting register state plus the bounded table rows. CH4
  first triggers from the played-note polynomial, then applies row zero without
  retriggering, matching hUGEDriver's LFSR/timer attack. Subsequent rows remain
  relative to the played note and are converted with hUGEDriver's
  `get_note_poly` at playback time. Macros advance at the original
  70,224-clock display-frame cadence. The custom duty vibrato keeps its exact
  eight-frame `++++----` period cycle; hUGEDriver's subpattern dispatcher
  intentionally bypasses the ordinary 1xx/2xx tick-zero guard.

The desktop signal is rendered at 48 kHz with integrated PSG edges and a
high-pass output stage so the monitor does not add 16 kHz point-sampling aliases
or asymmetric-pulse DC that are absent from binjgb and physical output.
Instrument auditions preserve the hardware attack; Mtile does not add a
software fade that changes the preset transient.

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the “Software”), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
