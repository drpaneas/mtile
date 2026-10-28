# Save Game: physical results

Status: **PENDING — no physical run recorded**

Fill this in after testing the actual ROM. Emulator results do not count as
physical verification. Use a separate record for each console/cart/firmware
combination and renderer; keep the original pending template in the example.

| Detail | Value |
| --- | --- |
| Date and tester | |
| GBA model | |
| Cartridge model/revision | |
| Kernel and firmware | |
| SD card | |
| Build commit and local changes | |
| Renderer | |
| ROM SHA-256 (`shasum -a 256 game.gba`) | |
| Launch mode / save type | |
| Photo/video or notes | |

- [ ] First boot with a fresh example save reports NO SAVE YET.
- [ ] Start shows the fixed centered PAUSED / Load / Save / Exit overlay over the frozen scene.
- [ ] A second Start resumes and restores the background and game input.
- [ ] The shared menu Save action reports SAVED; holding A writes once.
- [ ] The menu wait clears after five seconds; another successful Save restarts it.
- [ ] The menu Load action restores position, coin location, and COINS/SAVES.
- [ ] After waiting five seconds, full power-off/on restores the same data.
- [ ] Ten save/power-cycle repetitions restore each newest save.
- [ ] Unsaved movement is discarded after restarting.
- [ ] Select changes only RAM; menu Load still restores the previous save.
- [ ] Save after Select intentionally replaces the old run.
- [ ] Exit ends the session and shows the power-off/reset instruction.
- [ ] The saved run survives an overnight power-off.
- [ ] Repeated with both `-renderer mode0` and `-renderer mode4` ROMs.

Final decision: **PENDING / PASS / FAIL** (select one only after the run).

Unexpected behavior and exact reproduction:

Do not perform power-cut-during-write tests on the original Omega's SD card.
Byte-level interruption and corruption recovery are covered by automated
tests; testing direct SRAM/FRAM interruption requires a separate suitable
cartridge and its own result record.
