# TinyGo component

Mtile release bundles contain a pinned TinyGo 0.41.1 compiler so users do
not install TinyGo separately. The host Go distribution remains an end-user
requirement because TinyGo uses its standard library and build metadata.

The release builder downloads the official platform archive, optionally
checks the maintainer-provided SHA-256 digest, verifies the executable's
reported version, and retains only the files needed by Mtile's custom GBA
target:

- the TinyGo executable;
- common TinyGo source overlays plus `device/arm` and `device/gba`;
- `targets/gameboy-advance.json`;
- Clang resource headers, compiler-rt builtins, and picolibc inputs.

TinyGo is distributed under the BSD 3-Clause license. Its complete notice is
shipped as `TINYGO-BSD-3-CLAUSE.txt`; retained compiler-rt files also carry
their upstream license. Mtile does not modify the TinyGo compiler.

TinyGo source and releases are available from
<https://github.com/tinygo-org/tinygo>.
