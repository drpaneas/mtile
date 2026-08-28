# Release bundle

`build-bundle.sh` creates a desktop bundle containing:

- `mtile`: project CLI;
- `mtile-editor`: the Ebitengine IDE;
- `mtile-player`: the hidden headless mGBA helper;
- a pinned TinyGo toolchain;
- Mtile, mGBA, and TinyGo license notices.

The end user only needs a compatible Go installation. Lua parsing/transpiling
and the Mtile GBA runtime are embedded in the editor/CLI. mGBA does not need
to be installed and no second emulator window is opened.

The official TinyGo archive is pruned after verification to the compiler,
common source overlays, ARM/GBA device definitions, GBA target, Clang resource
headers, compiler-rt, and picolibc inputs used by Mtile. The release gate
must build a ROM with an empty cache after pruning; unrelated MCU definitions
must not be restored merely to make the bundle larger.

Maintainers should pass the official release asset's digest as
`TINYGO_SHA256` when producing a public artifact:

```sh
TINYGO_SHA256=... scripts/release/build-bundle.sh
```

Native mGBA helpers are built per target; run the script on each release OS and
architecture rather than cross-compiling that component.
