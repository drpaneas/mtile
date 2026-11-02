---
name: mTile PCM Editor
description: Native audio preparation and companion-bank import in the existing mTile editor.
colors:
  music: "#ff8570"
  selection: "#52d9d0"
  danger: "#ff5f68"
  background: "#100f1c"
  panel: "#232139"
  border: "#7771a2"
  section-border: "#43405b"
  text: "#ebe7fa"
  dim: "#c2bbdb"
  button-face: "#1c1b34"
  button-active: "#323068"
  button-hover: "#2b2946"
  button-pressed: "#272643"
typography:
  body:
    fontFamily: "Go Sans"
    fontSize: "13px"
    fontWeight: 400
  label:
    fontFamily: "Go Sans"
    fontSize: "11px"
    fontWeight: 400
rounded:
  square: "0px"
spacing:
  field-inset: "10px"
  panel-inset: "20px"
  column-gap: "24px"
components:
  button:
    backgroundColor: "{colors.button-face}"
    textColor: "{colors.text}"
    typography: "{typography.body}"
    rounded: "{rounded.square}"
    height: "36px"
  button-active:
    backgroundColor: "{colors.button-active}"
    textColor: "{colors.music}"
    rounded: "{rounded.square}"
  input:
    backgroundColor: "{colors.background}"
    textColor: "{colors.text}"
    typography: "{typography.body}"
    rounded: "{rounded.square}"
    height: "36px"
  quality-option:
    backgroundColor: "{colors.background}"
    textColor: "{colors.text}"
    typography: "{typography.label}"
    rounded: "{rounded.square}"
    height: "44px"
---

# Design System: mTile PCM Editor

## Overview

**Creative North Star: "Native audio import workbench"**

This document records the PCM Editor only. It extends the incumbent Go/Ebitengine editor without introducing a separate visual identity. The authority is the shared palette, fonts and button rendering in [draw_util.go](../../../cmd/editor/draw_util.go) and [theme.go](../../../cmd/editor/theme.go), together with [PCM state](../../../cmd/editor/pcm_editor.go), [drawing](../../../cmd/editor/pcm_editor_draw.go), [batch state](../../../cmd/editor/pcm_editor_batch.go) and [batch drawing](../../../cmd/editor/pcm_editor_batch_draw.go). Code remains authoritative if this snapshot drifts.

The top-bar **PCM** button opens a focused panel. Its left column normally shows project audio; selecting multiple files temporarily replaces it with an import queue, then an imported-file receipt. The right column shows the current file or saved call. Quiet dark surfaces keep filenames, shared settings, format changes and the next action readable. The reviewed compact and desktop batch renders had no material visual issues.

**Key Characteristics:**

- Existing native editor chrome and measured Go Sans text.
- Project library outside batch review; temporary queue with per-file inspection during review.
- Shared WAV settings and one confirmed import for the complete batch.
- Source-derived size comparisons, reversible edge trim and original/prepared audition.
- Actual bank size and copyable Lua calls.

## Colors

The shared violet neutrals carry structure; small coral and cyan accents carry action and selection.

### Primary

- **Music coral** (`music`): PCM navigation, action borders and active action labels.
- **Selection cyan** (`selection`): chosen quality, selected library/queue entry, focused input and busy status.
- **Danger red** (`danger`): delete and confirm-delete actions.

### Neutral

- **Deep background / panel** (`background`, `panel`): recessed fields and library within the enclosing tool panel.
- **Control / section borders** (`border`, `section-border`): interactive edges versus quiet container divisions.
- **Main / secondary ink** (`text`, `dim`): instructions and commands versus metadata and supporting explanations.
- **Button faces** (`button-face`, `button-active`, `button-hover`, `button-pressed`): the existing tactile button states.

## Typography

Use the shared Go Sans body and small faces; PCM headings and Lua-call displays also use the body face. The frontmatter records the default Comfort size. Existing Compact/Comfort/Large preferences supply body sizes (12/13/14 px) and small sizes (10/11/12 px). Do not add an independent PCM scale or substitute the game's pixel font. Text fits by measured width; paragraphs wrap, while paths, names and option labels truncate to their available width.

## Layout

The centered panel is capped at (1320 × 800 px), with width `min(1320, screenWidth-32)` and height `min(800, screenHeight-72)`. The left list is at most (280 px) wide; the right body follows after the column gap. This remains a two-column native desktop tool at the reviewed (1024 × 900) and (1920 × 1200) sizes; layout tests also cover (2560 × 1440). There is no separate mobile layout.

The header holds **MMUTIL…** and **Close**. **1 / Choose audio** accepts one or several files. Batch review becomes **2 / Review batch (i/N)**, with **Queue (N)**, **Remove**, **Add files** and **Prepared total** on the left; completion becomes **3 / Imported N files** with an **Imported** list and actual bank size. A single-file selection retains **2 / Prepare for GBA** and **3 / Import complete**. Outside batch review, the project library has **Import audio** and **Bank: … / 24 MiB limit** at its foot.

Primary import/delete actions anchor to the lower right; **Prepared WAV** and **Original WAV** audition sit together at the lower left during preparation. Library and queue rows have a (54 px) pitch; quality options are (44 px) high on a (50 px) pitch. Wrapped advice follows the selected file's size summary, with a full-width (32 px) shared trim/restore button below when eligible WAVs exist. The underlying palette, typography and panel geometry remain unchanged.

## Elevation & Depth

Flat fills and one-pixel rectangular strokes define depth; there are no shadows or animated lifts. Opening the panel dims the surrounding editor with a black overlay (alpha 205/255). Shared clean chrome supplies immediate hover and press changes.

## Shapes

Keep square corners throughout: panel, fields, options, library selection, command box and buttons. Use the existing single-stroke borders and fixed rectangle hit areas.

## Components

- **Choose:** native **Browse…** allows multiple selection on macOS, Windows and Linux; pasted full paths, one per line, provide the fallback. **Next: Prepare** reviews the complete selection. Repeated source paths are deduplicated; initially colliding basenames receive suffixes such as `_2`. The converter chooser is session-scoped; `mmutil` is needed to update a bank, while normal builds reuse the saved bank without it.
- **Queue and selection:** multiple files replace the project library with a temporary queue. Select a row to rename that file, inspect its format/size/advice and audition original/prepared WAV. **Add files** appends sources; **Remove** excludes the selected queued file without deleting a project asset. Preserve edited names across row and shared-setting changes; duplicate edited names within one kind must be resolved before import.
- **Shared preparation:** show the selected file's original/prepared duration, rate, bit depth, channels and sizes. **ALL WAVs: Balanced** is the default (mono, up to 16 kHz, 8-bit); **Smallest** uses up to 8 kHz; **Keep Source** accepts original 8/16-bit integer PCM. Cyan outlines mark selection. Each preset's size and percentage describe the **selected file**, using preparation's frame rounding, header and padding rules; **Prepared total** separately adds all staged WAV and tracker bytes. Neither predicts the final bank. Preset changes apply to every queued WAV. Tracker notes/instruments remain intact; select a WAV row to change the shared controls in a mixed queue.
- **Advice:** recommend another preset only when it saves at least (1 KiB and 10%) versus the current prepared WAV. Balanced suggests Smallest; Keep Source suggests Balanced. Give the alternative's size, saved bytes and percentage, then invite an audition. A prepared duration of at least (30 seconds) or size of at least (256 KiB) earns ordinary advisory text, never an import-error treatment. These figures do not predict the final bank size.
- **Optional shared trim:** analyze original samples before mono mixing and dither, checking every channel against a peak of (0.001 full scale, approximately −60 dBFS). Only leading/trailing quiet runs of at least (100 ms) qualify; retain (20 ms) at each cut and preserve interior pauses. **Trim eligible WAVs (N)** is opt-in and becomes **Restore quiet edges / all queued WAVs**; both reprepare from untouched external masters. Explain changed trigger timing and duration. Skip ineligible, all-quiet and authored `smpl` loop-marked WAVs, plus tracker modules. Keep Source disables shared trimming and restores original timing. Single-file trim uses the same rules with **Trim quiet edges** / **Restore quiet edges** labels.
- **Import and use:** **Import N files** saves the complete selection under `assets/audio/direct/` in one converter invocation and one bank/manifest publication. Existing-name replacements require **Replace + Import** confirmation and preserve IDs. Single-file actions remain **Import + Save** / **Replace + Save**. Completion lists each file's actual `sample(id)` or `pcm_music(id)`; **Copy Lua call** copies the selected call and **Copy all calls** produces a commented reference list, not executable playback of every sound. Explain event-triggered WAV calls versus tracker calls in `_init()`; **Build + Reload** plays the cartridge mix. **Original WAV** and **Prepared WAV** each audition up to fifteen seconds during preparation.
- **Library and deletion:** show each name with its stable public call. Existing assets expose **Delete asset**, then **Confirm delete**; replacement likewise requires a second action. Deletion reserves the removed slot and leaves every other public ID unchanged. Reimporting the same name restores that slot. These saves are immediate disk operations outside editor undo. The external source remains untouched; the project stores the prepared WAV, so retain the external master for later quality changes.
- **Errors and maintenance:** failed rows stay visible as **Needs attention** and disable import with **Fix N file(s)**; fix their shared preset/name or remove them before importing. Never publish a partial subset; failed conversion leaves the previous bank playable. Background preparation, analysis and audition decoding return through `pcmJobResult`; `pcmPollJob` applies UI/player changes on the editor loop, one job at a time. [Batch preparation](../../../internal/audioimport/batch.go) stages waveforms to disk sequentially; queue metadata stays in memory and only the selected waveform is loaded for review. Closing the wizard cleans its completed temporary workspace. Real-converter tests opt in with `MTILE_PCM_MMUTIL`; GPU screenshots require `mtile_editor_bench` and `MTILE_PCM_SCREENSHOTS`. The sidecar's HTML previews illustrate native primitives; generated tonal ramps are swatch previews, not additional editor tokens.

Preparation and analysis are offline authoring work; no analyzer enters the GBA loop. Smaller WAV data primarily saves cartridge space; it does not reduce the fixed GBA mixer RAM reservation or guarantee less CPU work per voice. Always show actual compiled bank size after import. See [audio semantics and workflow](../../audio/samples.md), [analysis](../../../internal/audioimport/analysis.go), [advice](../../../cmd/editor/pcm_editor_advice.go), [preparation](../../../internal/audioimport/prepare.go) and [stable-ID deletion](../../../internal/audioimport/library.go) for maintained behavior.

## Do's and Don'ts

### Do:

- **Do** reuse the native editor's shared fonts, colors and button primitives.
- **Do** keep the active library or queue, selected-file context and next action visible together.
- **Do** distinguish source size, prepared size and actual bank size.
- **Do** preserve external sources and every unaffected public Lua ID.
- **Do** make edge trimming reversible and require an explicit choice before changing timing.
- **Do** distinguish settings shared by all WAVs from the selected file's sizes, name and audition.

### Don't:

- **Don't** imply that a WAV recording becomes tracker music or that smaller source files remove fixed mixer costs.
- **Don't** hide converter warnings or publish worker results directly into UI state.
- **Don't** present long/large advisories as import errors or remove interior pauses.
- **Don't** import a partial batch while failed rows remain or copy every call as an executable playback block.
- **Don't** treat generated sidecar ramps or HTML previews as new native design authority.
