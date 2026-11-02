# Third-party components

Mtile's own code is [MIT licensed](../../LICENSE). Included dependencies,
fonts, and tools retain their upstream licenses.

| Component | Used for | Notice |
| --- | --- | --- |
| TinyGo | Native GBA compilation; included in desktop bundles | [Component notice](tinygo.md), [license](TinyGo-LICENSE.txt) |
| mGBA | Headless editor preview; pinned Git submodule | [Component notice](mgba.md), [upstream license](https://github.com/mgba-emu/mgba/blob/1c61b54208ca6266129d0f2394c04bd8c44f98c5/LICENSE) |
| Maxmod (BlocksDS fork) | Imported tracker music and WAV effects on GBA | [Component notice](maxmod.md), [ISC license](../../runtime/maxmod/COPYING) |
| JetBrains Mono | Code editor font | [SIL Open Font License](../../fonts/JetBrainsMono/OFL.txt) |
| OpenDyslexic / Nerd Font variants | Editor UI fonts | [Supplied license](../../fonts/OpenDyslexic/LICENSE.txt), [font notes](../../fonts/OpenDyslexic/README.md) |
| PICO-8 wide | Included font asset | [CC0 notice](../../fonts/pico-8-wide.otf/license.txt) |

The [root Go module](../../go.mod) and [runtime module](../../runtime/go.mod)
record library dependencies; their licenses remain in the corresponding
module source. Release builders must retain applicable notices for the
actual distributed components. The bundled compiler also includes upstream
compiler-rt and picolibc files and their license material.

The default palette is [Aurora](https://lospec.com/palette-list/aurora), by
DawnBringer. The Lua API is inspired by [PIGO8](https://drpaneas.com/pigo8/)
and PICO-8. Historical research notices in this directory describe their own
scope and do not imply that all researched components ship in Mtile.
