# Maxmod in mTile

Source: the BlocksDS Maxmod fork vendored by Butano at
04374b034c76b82bfdc3aa6c38d652e5e7778a68. ISC license in COPYING.

Local changes: use Timer 1 (Timer 0 belongs to the PSG clock); leave all
SOUNDCNT writes to runtime/hw/sound_control.go; avoid shifting a 32-bit mask
by 32 when initializing the full 32-channel mixer. The mTile bridge uses
fixed storage and never calls the allocating mmInitDefault entry point.

For LLVM: use C23 for upstream static assertions and labels; the assembly is
preprocessed as .S, two-operand MUL is written with three operands, and macro
register-name comparisons use .ifnc instead of GNU register arithmetic.
