# Saving games on a real GBA

Use the dedicated [Save Game example](../examples/save-game/README.md) to try
cartridge saving. It saves a few game-defined values and restores them on
startup. You choose what represents progress: a checkpoint, inventory,
position, collected items, or high score.

**Physical verification is pending.** The implementation targets GBA
SRAM-compatible save memory; the automated tests described below establish
software behavior. Only a recorded real-device power-cycle run establishes
compatibility with a particular console and cartridge.

## Hardware and storage

The first implementation uses the 32 KiB SRAM/FRAM interface. It needs a
cartridge providing that interface, or a flash cartridge configured to
support it. It does not implement EEPROM or Flash command protocols. A ROM
file on its own cannot add save storage to a cartridge without it.

SRAM/FRAM uses byte accesses at `0x0E000000`, with eight SRAM wait states.
The read routine runs in IWRAM, with inlining disabled. The linked ROM
contains a word-aligned `SRAM_Vnnn` detection signature. Games without save
calls omit this signature and save buffer. See the
[GBATEK SRAM/FRAM specification](https://mgba-emu.github.io/gbatek/#gba-cart-backup-sramfram).

Mtile stores 64 signed 16:16 numbers (256 bytes of payload). It uses two
304-byte records at SRAM offsets 0 and 512; the rest of the cartridge save
memory is untouched. These records are two recovery copies of **one saved
game**, not multiple player profiles or namespaces. The staging buffer uses
864 bytes of EWRAM (256 staging plus 608 scratch) only in games that call
the save API. Record processing needs no heap allocation.

## Lua API

| Call | Meaning and result |
| --- | --- |
| `save_set(index, number)` | Stage a number in field 0..63. Returns true on success, false for a noninteger/out-of-range/nonnumeric index or a nonnumeric value. Does not write the cartridge. |
| `save_get(index)` | Read one staged number. Invalid indices return 0. Fields start at 0. |
| `save_write("unique-game-v1")` | Write all 64 staged fields, verify them, then commit. Returns a numeric status below. |
| `save_read("unique-game-v1")` | Load all 64 fields from the newest complete record. Every unsuccessful read leaves the staging buffer unchanged. Returns a numeric status below. |

Keys must be **literal strings of 1..32 bytes**, optionally parenthesized.
Dynamic keys and invalid literals fail compilation in every renderer with
source-positioned `SAVE_KEY`. Pick a unique, stable key that includes your
game's save-layout version. Changing field meanings requires explicit
migration or a new key; a new key detects old data rather than interpreting
it as the new layout. The key is stored and compared exactly, not hashed.

| Status | Meaning |
| --- | --- |
| `1` | Read succeeded, or write completed and matched cartridge readback. A read may have recovered the older valid copy. |
| `0` | Read found no save: both records are uniformly blank (00 or FF). |
| `-1` | Write verification failed, or save storage was not attached by the runtime. |
| `-2` | Read found no valid record: damaged, interrupted first save, or unsupported record format. |
| `-3` | The newest valid record has another game's/layout's key. |
| `-4` | Invalid key in a direct Go call; invalid Lua keys are compile errors. |

Test statuses explicitly: Lua considers **0 and negative numbers true**.
An absent or incompatible save chip cannot reliably be identified by reads
alone; it may look empty or corrupt. A write returns `-1` when readback
does not match, rather than claiming the hardware is present.

```lua
function _init()
  if save_read("my-game-v1") == 1 then
    -- Check application-specific ranges before applying saved values.
    score = save_get(0)
  else
    score = 0
  end
end

function store_checkpoint()
  save_set(0, score)
  local result = save_write("my-game-v1")
  if result == 1 then
    -- Show "Saved" and follow the cartridge's power-off procedure.
  else
    -- Keep playing, show failure, and offer another explicit save attempt.
  end
end
```

The [shared pause menu](pause-menu.md) provides Save and Load in every game
that registers its progress callbacks, plus Start pause/resume and Exit for every game.

Read/write calls are synchronous. Trigger writes once per intentional player
action, not every frame. `btnp` has held-button repeat; the shared menu uses
button edges so holding A performs just one write.
The API preserves every raw fixed-point bit, including fractions and
negative values. It does not serialize tables, strings, globals, graphics,
audio, or a running machine. Both renderers have the same save semantics;
see the [renderer contract](render-modes.md#cartridge-saves).

## Failure and recovery

Each record contains a format version, exact key, generation counter, all
64 fields, and CRC-32 covering the metadata and payload. The next write
chooses the other record, invalidates and checks its commit byte, writes
and verifies its body, then writes a one-byte commit marker last. The
newest valid record is not modified. Generations compare across uint32 wrap.

On loading, Mtile verifies both records and selects the newest valid one.
An interrupted write or a detected damaged newest record can therefore
leave the previous complete save available. A failed first save may leave
no valid save at all. Failed writes do not change the game's staging buffer.
CRC detects accidental corruption; it is not authentication or a guarantee
against every possible corruption. Complete storage failure can lose both
copies.

Loading never writes storage. A deliberate `save_write` can replace corrupt
data or another key, while keeping the newest valid record until the new
write commits. Do not autosave after detecting damage or a key mismatch:
let the player choose whether to replace the saved game. Validate your
game's field ranges as well as checking the API status. Failed restoration
should not replace a running game's values with defaults.

## EZ-Flash Omega procedure

These instructions are for the **original EZ-Flash Omega**. The Definitive
Edition and other models have different backup behavior.

1. Back up `SAVER` on your SD card and use a distinct filename for this example.
2. Build the example and copy `game.gba` to the card.
3. Select **CLEAN BOOT**. Try SAVE TYPE **AUTO**; select SRAM if necessary.
4. Press Start, select Save, and press A. The shared menu's wait reminder
   clears after five seconds; wait for that change before resuming or powering off.
5. Fully power off and on, launch the same ROM, and check the restored data.

The [official Omega manual](https://www.ezflash.cn/omega.html) specifies at
least 2–3 seconds after saving and recommends counting to five. Mtile's
success result verifies cartridge-visible bytes; it cannot observe whether
the Omega's microSD flush has completed or report all SD filesystem errors.
The shared menu's five-second timer is a reminder, not a flush probe;
each successful write restarts it, and an error is never cleared by its expiry.
Two recovery records inside a save file cannot protect the filesystem or
the whole file from an interrupted SD write. Do not deliberately cut power
during that flush to test recovery.

Keep the same ROM filename during this test. Do not use the cartridge's
SAVE/LOAD state menu: the example saves through its own Lua code. The
built-in editor preview is useful for testing gameplay but is not the
physical persistence gate, and this feature does not add preview save-file
management.

## Verification

Automated runtime tests cover all 64 raw numeric fields, blank storage,
bad arguments/keys, exact key comparison, cold-context loading, repeated
writes, generation wrap, each corrupted record byte, write failures and
retry, and interruption at every byte store during first saves and record
reuse. Compiler/parser tests cover the API, source positions, argument
adjustment, generated execution, and Mode 0/4/AUTO selection.

The [paired ARM renderer check](benchmarks/render-modes/saving-v1.md) supplies
pixel-parity and timing evidence. It exercises the SRAM interface in mGBA;
an ARM playthrough also verifies the example's save/load and machine-reset
restoration in Mode 4, Mode 0, and AUTO. Neither substitutes for physical
hardware.

Run the required software gates from the repository root:

```sh
go test ./internal/luaparser ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

For physical verification, record the exact ROM SHA-256, build, console,
cartridge, firmware, SD card, and renderer using the example's
[result form](../examples/save-game/hardware-results.md). Check initial
save, reload, repeated overwrites, ten full power cycles, unsaved changes,
intentional new-game replacement, and an overnight power-off. Repeat with
both renderer builds. Only mark the recorded combination verified after
all checks pass. Interruption tests on direct SRAM/FRAM cartridges require
a separate test setup; the original Omega test follows its SD flush rules.
