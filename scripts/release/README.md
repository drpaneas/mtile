# Preparing a desktop release

The **Desktop release** workflow builds one archive for each target. Choose
**Run workflow** and leave the release version empty for test downloads.
To prepare a release, select its branch and enter a version such as `v0.1.0`.
Once all five builds and offline checks pass, the workflow creates a GitHub
draft targeting the tested commit and attaches the verified archives and SHA-256 files.
Publishing the draft creates its tag if it is new. Pushing an existing version
tag matching `v*` also runs the workflow. The draft job checks that an existing
tag points to the tested commit before attaching downloads.

| Target | Download | What the user opens |
| --- | --- | --- |
| macOS, Apple Silicon | `mtile-darwin-arm64.zip` | Mtile.app |
| macOS, Intel | `mtile-darwin-amd64.zip` | Mtile.app |
| Windows, x64 | `mtile-windows-amd64.zip` | Mtile.exe |
| Linux, x64 | `mtile-linux-amd64.tar.gz` | Mtile |
| Linux, ARM64 | `mtile-linux-arm64.tar.gz` | Mtile |

Every archive includes the desktop editor, CLI, headless mGBA player, a
prebuilt Coin Quest cartridge, the editable starter, the Go SDK and TinyGo's
GBA compiler and libraries. The editable `examples/` tree sits beside the
desktop launcher on every platform, including beside `Mtile.app` on macOS.
The first launch creates the user's own project
under `mTile Projects/Coin Quest` in their home folder. The editor opens
the bundled cartridge straight away. On subsequent launches it builds the
user's saved project. Linux requires a graphical desktop with X11, OpenGL
and audio support.

## Build on a native host

Use the matching OS and architecture, a recursive checkout, Go 1.27.1 or a
later compatible 1.27 patch, CMake and a native C compiler. The workflow pins
Go 1.27.1. Install the editor's [build dependencies](../../docs/getting-started.md#requirements).
The packaging shell also uses Git, curl, tar, unzip, sha256sum or shasum,
and zip on macOS. Windows uses a Unix shell and a native CMake toolchain.

The [workflow](../../.github/workflows/desktop-release.yml) pins the SHA-256
of each official [TinyGo 0.42.0 archive](https://github.com/tinygo-org/tinygo/releases/tag/v0.42.0).
Set the matching digest before running the build. Set `TINYGO_ARCHIVE` to
reuse a previously downloaded official archive. The digest is checked in
both cases.

```sh
TINYGO_SHA256=YOUR_VERIFIED_PLATFORM_DIGEST sh scripts/release/build-bundle.sh
```

The optional first argument sets the output directory (default `dist/`).
The builder uses a fresh staging directory, builds Coin Quest through the
packaged CLI and private SDKs, and writes the archive and checksum. The
archive's `BUILD-INFO.txt` records the revision, dirty-tree status, platform,
compiler versions, TinyGo digest and mGBA revision. On macOS this record and
the notices are inside `Mtile.app/Contents/Resources/`.

## Verify the actual download

With Python 3.12 or later, point the verifier at the archive on the target
desktop. Keep the matching `.sha256` file beside it.

```sh
python3 scripts/release/verify_download.py dist/mtile-darwin-arm64.zip --launch
```

It checks the checksum, extracts to a new path containing spaces, opens the
desktop entry point without arguments, and waits for its child emulator. Each
run has an empty home, temporary directory, compiler caches and command search
path. It edits the user copy of `game.lua` from speed 2 to 3, exports with the
bundled CLI and replays the same Right input in both cartridges. The test
requires the explorer to move farther in the edited ROM.

The verifier also adds the packaged CLI to `PATH` and runs from the
extracted folder containing the launcher and `examples/`. With `--launch`
it checks that `mtile edit examples/hello` opens the Hello editor window.
It then checks that `mtile build examples/hello` produces a new cartridge
at `examples/hello/game.gba`.

Each desktop also builds `examples/imported-audio` five times from fresh
project copies, home directories and compiler caches. The verifier compares
the cartridges byte for byte. It plays the imported WAV and PSG note and checks
250 frames of stereo audio against the original recording from the packaged
emulator. The workflow compares the five platforms' ROM SHA-256
digests before it creates a draft. Use `--audio-digest FILE` to save the
digest when running the verifier locally.

Before creating or updating the draft, the workflow downloads the five candidate
archives from the packaging jobs and checks their standalone examples on native
Windows x64, Linux x64 and ARM64, and macOS Intel and Apple Silicon runners.
Each runner builds every standalone example three times offline with the bundled
CLI and toolchain. Each attempt starts with a fresh project copy, home, temporary
directory and compiler caches. The 20 examples run in four parallel groups per
platform. `smb3` requires an external NES cartridge and is outside the standalone
catalog.

The **Compare all 300 offline ROM builds** job lists each example's size and
SHA-256 in its summary. A missing build, a failed build, or a difference among
the 15 builds of any example blocks the draft. The draft also checks the five
archives against the SHA-256 values recorded by the catalog runners before
attaching them. To exercise the full check, run **Desktop release** with the
release version empty.

The workflow additionally denies network access with a macOS sandbox or a
separate Linux network namespace. On Windows it uses temporary outbound firewall
rules for the extracted executables (`--block-windows-network`, administrator). A Linux headless runner needs
a virtual display for `--launch`. Omitting that option exercises the same
edit, export and playback checks on a freshly instantiated CLI project.

On Linux, CI also runs `python3 scripts/release/verify_linux_gui.py ARCHIVE`
inside the offline namespace. On x64 the test types the movement speed change
in Develop and saves the Lua file. On ARM64 it opens an existing Coin Quest
project with an edited Lua file to exercise returning players' changes.
Both use the editor's **Export .gba** button and verify the new speed by
playing the exported ROM.

## Before distributing a bundle

Use a clean revision and run the project's [release checklist](../../docs/launch-checklist.md).
For a public macOS download, set `MTILE_MACOS_SIGN_IDENTITY` to the signing
identity in your keychain and `MTILE_MACOS_NOTARY_PROFILE` to your notarytool
credentials profile. Then run the following on macOS with the release version.

```sh
scripts/release/sign-macos.sh dist/mtile-darwin-arm64.zip signed 0.1.0
python3 scripts/release/verify_download.py signed/mtile-darwin-arm64.zip --launch
```

The script verifies the candidate checksum, signs the nested tools and app,
submits the ZIP to Apple when a profile is supplied, staples the accepted ticket
and writes the final ZIP and SHA-256 together. Use a separate output directory.
When distributing signed packages, supply the final signed archive and its
checksum in the packaging job before the offline checks and artifact upload.
For a local signing test, set the identity
to `-` and leave the notary profile unset.

For Windows use the release publisher's
Authenticode identity as appropriate. Run the verifier on the final signed
archive and test a browser-downloaded copy on a clean target desktop, including
keyboard input, audio and **Export .gba**. The normal build produces local
development candidates. Keep [third-party notices](../../docs/third-party/README.md)
and the pinned mGBA source available with distributed releases.
