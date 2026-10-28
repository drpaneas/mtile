# Shared pause menu

Every newly built Mtile game includes the same fixed system overlay. Press
**Start** to pause; press it again to resume. The centered panel has a centered
**PAUSED** heading and exactly three items: **Load**, **Save**, **Exit**. There
is no Resume row or controls footer. Developers cannot disable, replace,
reposition, restyle, or change the items through Lua or project configuration.

![The fixed pause overlay over a frozen game.](../examples/tidewalk/pause-menu.png)

The game remains visible around the panel. Game updates, drawing, the game
clock and button-repeat ticks stop; music continues in the background.
Up/Down selects a row and A activates it. B also resumes. Held menu buttons
must be released before they can affect the game again. Start is reserved
for this menu in generated games.

| Item | Action |
| --- | --- |
| Load | Restore the game's latest successful save through its progress function. |
| Save | Replace the game's one logical save through its progress function. |
| Exit | End the session, stop sound and show **Power off / or reset**. |

## One save, with recovery

There is **one logical save per game**, not a list of slots. Load restores
that save; it does not offer a file picker or rewind to an earlier manual save.
The two SRAM records are alternating recovery copies of the same save. If
one is damaged or an interrupted write leaves it incomplete, the runtime can
read the remaining valid copy. A key identifies the game's field layout; it
does not create another slot. Saving again replaces the current progress.

Load stays in the menu and reports success, no save, damaged data, a different
key/layout, or a read failure. A failed `save_read` leaves the staged fields
unchanged. The game must only apply values after a successful, validated read.

Exit does not save automatically. Choose Save first to keep changed progress.
It ends the running game until the console/emulator is reset. It does not
return to an EZ-Flash launcher or close a desktop emulator. If a successful
save is still within its five-second Omega wait, the ended panel first
shows **PLEASE WAIT**, then the power-off instruction.

After successful saving, resuming is blocked for five seconds. Another
successful save restarts the wait. An error remains visible beside the
wait reminder from any previous successful save. The timer advances in the
menu independently of the suspended game's clock, and is a reminder rather
than a probe of the flash cartridge's SD write completion.

## Connect game progress

Pausing, resuming and Exit work in every game without registration. Save and
Load are greyed out until the game registers its progress functions. These
functions select the values to save and restore; they cannot customize the
system menu. The feature stores game progress, not a snapshot of the entire
running machine.

```lua
score = 0

function store_progress()
  save_set(0, score)
  return save_write("my-game-v1")
end

function restore_progress()
  local result = save_read("my-game-v1")
  if result == 1 then score = save_get(0) end
  return result
end

function _init()
  pause_menu("store_progress", "restore_progress")
end
```

`pause_menu(save_function, load_function)` must be one direct, unconditional
statement in `_init`. Both arguments must be literal names of ordinary
zero-argument functions. Each function must return exactly one numeric
[save status](saving.md#lua-api). Invalid registrations fail compilation with
`PAUSE_MENU`; nonnumeric or fractional returned statuses report failure.
The callbacks are bound at build time and never run automatically per frame.
Both participate in renderer selection, including their transitive helpers.

Save reports success only for status 1. Validate your game's field meanings
before applying saved values. The dedicated
[Save Game example](../examples/save-game/README.md) connects both actions.

## Renderer and hardware behavior

The overlay uses one private 64x64 4bpp hardware sprite. The normal panel is
64x48, centered at (120,80); feedback can expand it to 64x64. Its pixels,
colors and layout are independent of the game's palette, camera and clipping.
It leaves background planes and framebuffers untouched, and preserves the
relative order of the game's sprites. Start restores borrowed tiles, OAM,
five OBJ palette entries and display/blend controls exactly.

About 5 KiB of fixed Context RAM holds backups and the prepared UI. No Lua
heap is required. To guarantee an intact frozen scene, Mode 0 reserves one
OAM entry, 64 scanline rendering cycles and the last 2 KiB of OBJ tiles. Game
limits are 127 OAM entries, 1,162 cycles on the worst scanline, and legacy `spr`/`sspr`
source IDs 0..479. Text uses physical OBJ cells 448..479; the optional packed-sprite
cache uses 416..447. The authored text-sheet IDs remain 480..511 for
compatibility; this change does not stamp text into authored cells 448..479.

The UI uses physical OBJ palette indices 241..245. Mode 0 rejects 8bpp sprite
sources containing those indices, text in those indices, or sprite mutations
whose color cannot be proven to avoid them. The current proof scans the
whole relevant sprite source sheet conservatively. Resident 4bpp color
variants may use at most 15 game palette banks. Background colors remain
unrestricted. Forced Mode 0 reports a source-positioned error; AUTO falls
back to Mode 4. These checks apply even to games without save callbacks.
See the full [renderer contract](render-modes.md).

[ARM parity and interaction checks](benchmarks/render-modes/pause-menu-v1.md)
exercise the overlay and restoration. Physical GBA/EZ-Flash power-cycle
verification remains pending; follow the [Omega procedure](saving.md#ez-flash-omega-procedure).
