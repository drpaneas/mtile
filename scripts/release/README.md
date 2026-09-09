# Building a desktop bundle

Run `build-bundle.sh` from a source checkout on the **same OS and architecture**
as the intended bundle. It creates a directory containing the CLI, editor,
headless mGBA helper, pinned TinyGo toolchain, notices, a standalone README,
and a `BUILD-INFO.txt` revision/toolchain record. It does not publish a release
or produce a signed installer.

The script accepts `darwin-amd64`, `darwin-arm64`, `linux-amd64`,
`linux-arm64`, and `windows-amd64`. These are script targets, not a claim that
every combination has completed release validation. CI currently runs on
Linux and macOS. Windows requires a shell with the script's Unix tools plus
a native CMake/compiler setup.

## Prerequisites

- Go 1.27.1+, Git, CMake, and a native C compiler.
- The editor's platform dependencies from [getting started](../../docs/getting-started.md).
- `curl`, `shasum`, `tar`, and `unzip` for archive download/verification/extraction.
- Initialized submodules: `git submodule update --init --recursive`.
- The independently verified SHA-256 digest for the intended official
  [TinyGo 0.42.0 archive](https://github.com/tinygo-org/tinygo/releases/tag/v0.42.0).

```sh
TINYGO_SHA256=YOUR_VERIFIED_64_CHARACTER_DIGEST scripts/release/build-bundle.sh
```

Pass an optional output directory as the first argument. The default `dist/`
is ignored by Git. The script rejects a missing/malformed digest or a
cross-target build before creating output, and checks the archive before
extracting it. Do not derive the expected digest from the same unverified
archive you are trying to authenticate.

## What is verified automatically

The build compiles both Go executables and the native helper, verifies the
TinyGo archive digest and executable version, and prunes compiler files to
Mtile's ARM/GBA inputs. It retains common source overlays, device definitions,
Clang headers, compiler-rt, and picolibc. It includes the supplied licenses for
TinyGo, mGBA, and fonts embedded in the editor.

A final smoke build runs the bundled CLI **outside the source checkout**,
creates a fresh project, and links a ROM with the bundled TinyGo explicitly
selected. That exercises the embedded runtime and the pruned compiler. The
smoke can reuse the host's compiler cache; it is not clean-machine sign-off.

## Before distributing a bundle

1. Build from an identified, clean revision and retain `BUILD-INFO.txt`.
2. Run the full CI gates and [hardware sign-off](../../docs/audio/hardware-signoff.md)
   required for the advertised release scope.
3. Move the bundle to a different directory on a clean user account/machine.
   Build a new ROM with empty caches and without system TinyGo or mGBA.
4. Open the editor; check all four workspaces, save/reopen, Build + Reload,
   input, and audio. Verify the Linux runtime libraries or Windows tooling on
   the platform you will advertise.
5. Review notices for all distributed Go/native dependencies and fonts, and
   make the exact pinned mGBA source plus any changes available as described in
   [its component notice](../../docs/third-party/mgba.md).
6. Archive the bundle, compute the final archive's SHA-256, and include it
   alongside platform requirements and known limitations in release notes.
   Complete platform signing/notarization separately when applicable.

The end user needs a compatible Go installation and the OS graphics/audio
libraries. They do not separately install TinyGo, Lua, mGBA, CMake, SDL, or Qt.
