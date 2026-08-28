# Develop workspace plan

## Product promise

`Develop` is Mtile's single code-and-cartridge workstation. A beginner can
open a project, edit Lua, press **Build + Reload**, and immediately play the
result. An experienced developer can navigate several source files, inspect
the generated cartridge, diagnose build failures, and observe real GBA state
without leaving the editor.

The workspace stays native Ebitengine UI. It does not embed a browser, Node,
Monaco, an LSP server, or a Lua runtime in the cartridge.

## Scope

### 1. Project and source model

- Keep root `game.lua` as the default entry point for every existing project.
- Allow `project.json` to select another project-relative Lua entry point.
- Resolve static top-level `require("module")` declarations at build time.
- Search the requiring file's directory and then `src/`.
- Reject absolute paths, traversal, missing modules, and dependency cycles.
- Compile each module once into the cartridge's shared global namespace.
- Save all dirty source documents before a build when autosave is enabled;
  otherwise refuse to build stale disk contents.

### 2. Project explorer and documents

- Show real project files and directories, not mock entries.
- Open Lua files in tabs; preserve cursor, scroll, selection, and undo per tab.
- Display dirty state and support save, save-all, and close.
- Route known asset files to Sprite, Map, or Audio instead of displaying fake
  editors for formats Mtile already owns visually.
- Offer quick-open and keyboard-first navigation.

### 3. Code editor

- Preserve current syntax highlighting, selection, clipboard, undo/redo,
  search, diagnostics, and completion.
- Add document tabs, line/column metadata, active language, indentation and
  encoding indicators.
- Keep diagnostics actionable: file, line, column, and click-to-open.

### 4. Build and console system

- Emit truthful build phases from source resolution through ROM validation.
- Provide Problems, Output, Build, Runtime, and Generated Go views.
- Keep logs bounded and selectable, and never invent successful phases.
- Show ROM size plus static/preallocated EWRAM, free EWRAM headroom, and IWRAM
  use from the actual linker result.
- Select the Build log on every build request and keep a concise build/preview
  state above the embedded GBA screen.

### 5. Live GBA inspector

- Keep mGBA preview interactive and persistent beside source.
- Report real frame/input/audio state from the running helper.
- Expose cartridge resources from validated build/editor metadata.
- Expose the four Game Boy PSG channels from hardware registers.
- Add bounded preview-build telemetry for performance and named top-level
  watch values. Do not claim arbitrary locals or breakpoints.

### 6. Preferences and polish

- Persist native editor preferences under the user's configuration directory.
- Persist LCD preview, effects level, autosave-before-build, preview audio
  volume, and interface text size; every visible option must change real
  behavior.
- Retain the established Mtile GBA workstation theme, compact hierarchy,
  readable focus states, and immediate tab switching.

## Implementation status

All six milestones are implemented:

- static multi-file Lua resolution and module-aware diagnostics;
- real project explorer, document tabs, quick-open, per-document editing state,
  safe save-all, external-change protection, and asset routing;
- truthful build events plus Problems, Output, Build, Runtime, and exact
  Generated Go views;
- persistent interactive mGBA preview with real PSG/input/performance telemetry
  and bounded top-level watches;
- versioned native preferences with atomic persistence; and
- responsive layout guards for minimum, 1920×1200, 2560×1440, and 4K windows.

## Verification gates

Focused source-graph, build, emulator-protocol, preference, document-safety,
layout, and syntax-color tests cover the milestones. Opt-in integration tests
build real TinyGo cartridges and run them through the native mGBA helper to
prove that debug watches advance under input and that active PSG hardware is
reported. Use `MTILE_LIVE_INTEGRATION=1` and set `MTILE_PLAYER` to the
built helper to run those gates locally.

## Explicit non-goals

- No Direct Sound PCM authoring in the current PSG-only audio product.
- No runtime Lua interpreter.
- No fake source-language selector.
- No fabricated CPU, memory, channel, or debugger values.
- No arbitrary local-variable debugger or source breakpoints in this phase.
