# TinyGo component

Mtile desktop bundles include TinyGo 0.42.0 and the Go SDK used to build
the release. TinyGo uses the included Go standard library and build tools.
The archive contains the Go license in `licenses/GO-BSD-3-CLAUSE.txt`.

The release builder downloads the official platform archive, requires and
checks the maintainer-provided SHA-256 digest, verifies the executable's
reported version, and retains only the files needed by Mtile's custom GBA
target:

- the TinyGo executable;
- common TinyGo source overlays plus `device/arm` and `device/gba`;
- `targets/gameboy-advance.json` and `targets/gameboy-advance.s`, which supplies the cartridge header during linking;
- Clang resource headers, compiler-rt builtins, and picolibc inputs.

TinyGo is distributed under the BSD 3-Clause license. Its complete notice is
shipped as `TINYGO-BSD-3-CLAUSE.txt`; retained compiler-rt files also carry
their upstream license. Mtile does not modify the TinyGo compiler.

TinyGo source and releases are available from
<https://github.com/tinygo-org/tinygo>.
