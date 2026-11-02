# Audio only

A blank screen and four sustained tracker voices. No controls or gameplay.
The original sine-wave test assets and bank are the same ones used in the
Butano comparison; this small listening example omits the benchmark's tick
counter, so its complete ROM hash is not a measured fixture's ROM hash.

```sh
mtile edit examples/audio-only
```

Build + Reload to listen. Building this already imported example needs no
converter. `pcm_music(1)` starts the four-voice module once and loops it.
The retained bank also contains one-, sixteen- and thirty-two-voice modules
and a WAV bell so the benchmark can reproduce its workload matrix. See
the manifest for their stable public IDs.

The corresponding minimal Butano program uses normal public APIs:

```cpp
#include "bn_core.h"
#include "bn_music_item.h"

int main() {
    bn::core::init();
    bn::core::set_skip_frames(1);
    bn::music_item(1).play();
    while(true) bn::core::update();
}
```

Its audio directory must contain the same source files in the same sorted
order; the reproducible harness builds and verifies both banks. Open
[the illustrated measurements](../../docs/audio/imported-performance.md) for CPU,
ROM, both RAM banks, heap/GC checks and the 32-voice timing limitation.
The benchmark C++ fixture and generation scripts live in
`scripts/butano-bench/pcm/`. All included audio is original procedural test
material, under the repository MIT license.
