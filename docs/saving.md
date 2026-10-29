# Saving games on a real GBA

Mtile provides cartridge-save APIs for game-defined menus. The dedicated
[Save Game example](../examples/save-game/README.md) has three independent
player files, five-letter names, Save and Continue, Save and Quit, and Load.
The example enables Start pause only during gameplay with `pause`;
its menus use Start normally. There are no system Save/Load/Exit actions.

Saving records progress chosen by the programmer: checkpoint, inventory,
position, collected items or score. It does not capture the running machine.
**Physical verification remains pending.** Emulator and fault-injection tests
establish software behavior; a recorded console/cartridge power-cycle run
establishes compatibility with that hardware combination.

## Hardware and storage

The API uses the 32 KiB SRAM/FRAM interface at `0x0E000000`, with byte accesses
and eight SRAM wait states. It requires a cartridge exposing this interface
or a compatible flash cartridge. EEPROM and Flash command protocols are not
implemented. The noinline read routine runs in IWRAM; linked saving ROMs
contain a word-aligned `SRAM_Vnnn` detection signature. Games without storage
calls omit the signature and save buffer. See the
[GBATEK SRAM/FRAM specification](https://mgba-emu.github.io/gbatek/#gba-cart-backup-sramfram).

There are **32 independent slots, numbered 0..31**. Each holds 64 signed 16:16
numbers (256 payload bytes). Each slot owns two 304-byte recovery records at
`slot * 1024` and `slot * 1024 + 512`. A write affects only the chosen slot;
unused slots and record gaps remain untouched. The two records within a slot
are backups of one file, not two player files. The shared staging buffer
uses 864 bytes of EWRAM (256 fields plus 608 scratch), regardless of slot count.
Record processing allocates no heap memory.

The original single-save API still uses **slot 0**, at the same offsets and
with the same record format. Existing saves remain readable with their
original key and field layout. Adding slots does not migrate game payloads.

## Lua API

| Call | Meaning and result |
| --- | --- |
| `save_set(index, number)` | Stage field 0..63. Returns true for an integer in-range index and numeric value; otherwise false. No cartridge write. |
| `save_get(index)` | Read a staged number; invalid indices return 0. Fields initially contain 0. |
| `save_write("game-v1" [, slot])` | Write, verify and commit all 64 staged fields to one slot. Omitted slot means 0. |
| `save_read("game-v1" [, slot])` | Read the newest complete recovery copy in one slot into all 64 fields. Omitted slot means 0. Every failure preserves staging. |
| `save_pending()` | Boolean: a successful write occurred within the last 150 engine polls (about five seconds at 30 Hz). This is a power-off reminder, not an SD flush probe. |

Slot expressions may be dynamic and evaluate once. Slots must be integer
numbers 0..31; invalid types, fractions or out-of-range values return `-5`
without touching SRAM, staging or the reminder. All slots share one RAM
staging buffer: a successful read replaces its contents. Slot selection is
explicit on every read/write; it does not change a hidden active slot.

Keys must be **literal strings of 1..32 bytes**, optionally parenthesized.
Dynamic keys and invalid literals fail every renderer with source-positioned
`SAVE_KEY`. Use a stable key identifying your game and payload version. All
player files can use the same key; **the slot selects the file**, while the
key checks the layout. Different keys do not create additional files in one
slot. Key comparison is exact, without hashing.

| Status | Meaning |
| --- | --- |
| `1` | Read succeeded, or write matched cartridge readback and committed. A read may recover the older valid copy in this slot. |
| `0` | Read found both records in this slot uniformly blank (00 or FF). |
| `-1` | Write verification failed, or runtime save storage was not attached. |
| `-2` | No valid record in this slot: damage, interrupted first save, or unsupported format. |
| `-3` | This slot's newest valid record has another key/layout. |
| `-4` | Invalid key in a direct Go call; Lua keys are checked at compile time. |
| `-5` | Invalid slot. |

Test `result == 1` explicitly: Lua treats **0 and negative numbers as true**.
A missing/incompatible save chip cannot reliably be identified by reads alone;
it may appear empty or corrupt. Writes return `-1` when readback disagrees.

```lua
local active_slot = 0 -- The game's file picker chooses 0, 1 or 2.
local score = 0

function store_progress()
  save_set(0, score)
  return save_write("my-game-v1", active_slot)
end

function restore_progress()
  local result = save_read("my-game-v1", active_slot)
  if result == 1 then
    local saved_score = save_get(0)
    if saved_score < 0 or saved_score > 999 then return -6 end
    score = saved_score
  end
  return result
end
```

The game controls the active slot, file names, New Game, overwrite confirmation,
and any copy/erase actions. Names can be encoded using `ord` into numeric
fields and reconstructed with `chr`, as in the example. To erase a file
atomically, commit a valid payload that your game recognizes as empty. Do not
clear raw commit markers: that could expose an older recovery copy.

The former `pause_menu` registration has been removed. Move those progress
functions into your own input/menu flow; no callback registration is needed.
Use button edges for writes: `btnp` repeats held buttons. Calls are synchronous;
write once per deliberate player action, never every frame. The API preserves
every raw fixed-point bit but does not serialize tables, strings, globals,
graphics or audio. The [renderer contract](render-modes.md#cartridge-saves)
is identical for both renderers.

## Failure and recovery

Each record contains a version, exact key, generation, all fields and CRC-32.
A write targets the other copy **within the selected slot**, invalidates and
checks its commit byte, writes and verifies the body, then commits last with
one byte. The newest valid copy remains untouched. Generation comparisons
handle uint32 wrap. Reads validate both copies, select the newest valid one,
then check the key. A newer different key returns `-3` even if the older copy
matches. Interrupted first writes may report corrupt rather than empty.

Failed writes preserve staging and the previous complete record; failed
reads preserve staging and never write SRAM. Games must validate all payload
meanings before changing live progress. The example uses `-6` for its own
payload validation failure; this is not a runtime save status.

An explicit write can replace corrupt data or a different key in that slot.
Request deliberate player intent before replacement; do not automatically
save defaults over an unreadable file. Other slots remain independent. CRC
is accidental-corruption detection, not authentication. Whole-chip failure
or loss of a flashcart's SD save file can still lose every slot.

## EZ-Flash Omega procedure

These instructions target the **original EZ-Flash Omega**, not the Definitive
Edition, whose backup behavior differs.

1. Back up `SAVER` on the SD card and use a distinct ROM filename for the example.
2. Build and copy `game.gba`, then launch with **CLEAN BOOT**. Try SAVE TYPE
   **AUTO**, or select SRAM if needed.
3. Choose an empty file, enter a name, select **END**, and press A. Wait until
   **PLEASE WAIT** clears and the file picker returns.
4. Open that file, play, press **Select**, and choose **Save and Quit**.
   Wait for the file picker before powering off or resetting.
5. Fully power off/on, open the same ROM and choose that named file. Check its
   progress. Create two more files and check all three remain independent.

The [official Omega manual](https://www.ezflash.cn/omega.html) specifies at least
2–3 seconds after saving and recommends counting to five. `save_pending()`
restarts on every successful write, advances during pause, and does not block
pause/resume. Failed writes do not clear or restart an earlier reminder.
The example displays its own waiting screen until the reminder expires.

The timer only measures elapsed engine polls. The API verifies cartridge-visible
bytes; it cannot observe microSD flush completion or all SD filesystem errors.
Recovery copies cannot protect the whole SD file or filesystem from an
interrupted flush. Do not deliberately cut power during an Omega SD write.
Keep the ROM filename unchanged between tests. Do not use flashcart save states;
the example saves through its own Lua code. Editor preview is useful for UI
checks but is not the physical persistence gate.

## Verification

Runtime tests cover all fields and all 32 slots, slot-zero compatibility,
invalid arguments, cold-context reads, CRC corruption, generation wrap,
failed writes/retries and byte-by-byte interruption. Nonzero-slot interruption
tests verify the target recovers and neighboring slots remain unchanged.
The reminder test calls `SaveWrite` directly, checks all 150 polls, retry
restart/failure behavior and immediate Start resume during the reminder.

Parser, compiler and generated-runtime tests cover dynamic slot evaluation,
argument adjustment, errors and Mode 0/4/AUTO selection. The
[paired ARM workload](benchmarks/render-modes/saving-slots-v2.md) exercises slot
selection and SRAM on the target; a separate playthrough creates three named
files and checks loading after a machine reset. The dynamic-name example uses
Mode 4 (AUTO falls back with `M0_PRINT_SHAPE`); the numeric save API and system
pause also have Mode 0 parity/cost coverage.

```sh
go test ./internal/luaparser ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

Record the real console, cartridge, firmware, SD card, ROM SHA-256 and renderer
in the [hardware results form](../examples/save-game/hardware-results.md).
Only mark that exact combination verified after its physical checks pass.
