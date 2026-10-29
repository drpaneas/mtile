# Save Game: physical results

Status: **PENDING — no physical run recorded**

Keep this template pending. Record actual runs separately for each
console/cart/firmware combination. Emulator reset results are not physical
power-cycle verification.

| Detail | Value |
| --- | --- |
| Date and tester | |
| GBA model | |
| Cartridge model/revision | |
| Kernel and firmware | |
| SD card | |
| Build commit and local changes | |
| Renderer (this dynamic-name example uses Mode 4) | |
| ROM SHA-256 (`shasum -a 256 game.gba`) | |
| Launch mode / save type | |
| Photo/video or notes | |

- [ ] Fresh SRAM shows three NEW GAME files.
- [ ] Start selects a file and finishes naming; no system pause appears in menus.
- [ ] Holding Start while entering gameplay does not immediately pause.
- [ ] During gameplay, a fresh Start shows only centered P A U S E D over a frozen scene.
- [ ] A second Start resumes with intact graphics and input.
- [ ] Empty file opens name registration; A adds, B deletes, END registers.
- [ ] Registration waits then returns to the file picker with the correct name.
- [ ] Create ALPHA, BETA and GAMMA with distinct progress in files 1, 2 and 3.
- [ ] Save and Continue writes once while A is held and resumes after the reminder.
- [ ] Save and Quit returns to the picker after the reminder clears.
- [ ] Load Last Save discards unsaved movement in the selected file.
- [ ] Quit Without Saving leaves the cartridge file unchanged.
- [ ] After waiting, full power-off/on restores all three names and their own progress.
- [ ] Ten save/power-cycle repetitions preserve the newest data in each file.
- [ ] Updating one file leaves the other two unchanged.
- [ ] Erase cancellation preserves the file.
- [ ] Confirmed erase affects only that file; it stays empty after power-off/on.
- [ ] Recreating an erased file preserves the other files.
- [ ] The saving and erase-confirmation screens keep system pause disabled.
- [ ] All three files survive an overnight power-off.

Final decision: **PENDING / PASS / FAIL** (select only after the run).

Unexpected behavior and exact reproduction:

Do not test interruption by cutting power during the original Omega's SD
flush. Automated byte-level tests exercise record recovery and slot isolation;
direct SRAM/FRAM interruption needs a separate suitable cartridge and result.
