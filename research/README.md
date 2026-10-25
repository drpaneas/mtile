# Research projects

These projects preserve porting and renderer experiments. They are outside the
public teaching examples and may have additional requirements or known failures.
The optional local `examples/celeste` checkout remains separate and untracked.

| Project | Status |
| --- | --- |
| [celeste-mode0](celeste-mode0/README.md) | Dedicated public Mode 0 adaptation and benchmark input |
| [celeste-parallax](celeste-parallax/README.md) | Private tile-canvas comparison port; read its opt-in requirements |
| loop | Historical puzzle-game port and authored-pixel import fixture; not a supported runnable example |

`loop/game.lua` still makes multiple explicit BG0 map calls in one frame,
which the current common map contract rejects. Its artwork is retained for
import tests; restoring the game requires a deliberate map/render migration.
No working-game claim is made for it.
