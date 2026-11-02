// vram_strb stores the low byte of \val at \addr without an 8-bit store.
// Mode 4 draws into BG VRAM, which writes an 8-bit store's byte to both
// halves of its halfword, so the byte is merged into the aligned halfword.
// \val's upper bits are ignored. Clobbers \t1, \t2 and the flags.
.macro vram_strb val, addr, t1, t2
    bic     \t1, \addr, #1
    ldrh    \t2, [\t1]
    tst     \addr, #1
    eoreq   \t1, \val, \t2
    andeq   \t1, \t1, #0xFF
    eoreq   \t2, \t2, \t1
    andne   \t2, \t2, #0xFF
    orrne   \t2, \t2, \val, lsl #8
    bic     \t1, \addr, #1
    strh    \t2, [\t1]
.endm

.section    .init
.global     _start
.align
.arm

_start:
    b       start_vector

    // Supply the 156-byte GBA header field from your local toolchain.
    .incbin "mtile-gba-logo.bin"

    // Game title
    .byte 0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00

    // Game code
    .byte 0x00,0x00,0x00,0x00

    // Maker code
    .byte 0x00,0x00

    // Fixed value
    .byte 0x96

    // Main unit code
    .byte 0x00

    // Device type (0x00 retail, 0x80 debug)
    .byte 0x00

    // Reserved
    .byte 0x00,0x00,0x00,0x00,0x00,0x00,0x00

    // Software version
    .byte 0x00

    // Complement check
    .byte 0x51

    // Reserved area
    .space 98

start_vector:
    // Configure WAITCNT for fast GamePak ROM access before executing any
    // more ROM-resident code. WAITCNT resets to its slowest wait states
    // (WS0 first/second access = 4/2 cycles) with the prefetch buffer
    // disabled, so every instruction fetched from ROM (i.e. this entire
    // program) runs several times slower than necessary until this is set.
    // 0x4317 selects WS0 first access = 3 cycles, WS0 second (sequential)
    // access = 1 cycle, and enables the prefetch buffer. It also selects
    // SRAM = 8 cycles, WS1 = 4/4, and WS2 = 8/8; these are not all reset
    // defaults. No EWRAM timing/overclock register is changed.
    // This is the standard GBA homebrew fast-ROM WAITCNT value.
    ldr     r0, =0x04000204                 // REG_WAITCNT
    ldr     r1, =0x4317
    strh    r1, [r0]

    // Configure stacks
    mov     r0, #0x12                      // Switch to IRQ Mode
    msr     cpsr, r0
    ldr     sp, =_stack_top_irq            // Set IRQ stack
    mov     r0, #0x1f                      // Switch to System Mode
    msr     cpsr, r0
    ldr     sp, =_stack_top                // Set user stack

    // Configure interrupt handler
    mov     r0, #0x4000000                 // REG_BASE
    ldr     r1, =handleInterruptARM
    str     r1, [r0, #-4]                  // actually storing to 0x03007FFC due to mirroring

    // Enable interrupts
    mov     r1, #1
    str     r1, [r0, #0x208]               // 0x04000208 Interrupt Master Enable

    // MTILE_ZERO_EWRAM
    // TinyGo preinit only clears IWRAM .bss. Zero .ewram (NOLOAD) before main.
    ldr     r0, =_ewram_start
    ldr     r1, =_ewram_end
    mov     r2, #0
.Lzero_ewram:
    cmp     r0, r1
    bhs     .Lzero_ewram_done
    str     r2, [r0], #4
    b       .Lzero_ewram
.Lzero_ewram_done:
    // Jump to user code (switching to Thumb mode).
    // The literal pool must sit after this bx. A .ltorg on the done label
    // makes bhs execute _ewram_end as code; at heapSize 56 KB that word is
    // 0x0202db74 (andeq sp, r2, #imm) and mGBA reports PC=E92D4830.
    ldr     r3, =main
    bx      r3
    .ltorg

// Small interrupt handler that immediately jumps to a function defined in the
// program (in Thumb) for further processing.
handleInterruptARM:
    ldr     r0, =handleInterrupt
    bx      r0
    .ltorg

// The caller masks IME across its predicate check and this BIOS Halt. An
// enabled pending request wakes HALT even with IME=0; restoring IME then
// services it. Video, sound, timers and DMA continue while the CPU sleeps.
.section .text.mtile_halt_display_wait,"ax",%progbits
.align 2
.arm
.global mtile_halt_display_wait
.type mtile_halt_display_wait,%function
mtile_halt_display_wait:
    swi     0x020000
    bx      lr
.size mtile_halt_display_wait, .-mtile_halt_display_wait

// Copy an opaque span list into an equally aligned 240-pixel framebuffer.
// r0/r1 are EWRAM raster/framebuffer bases, r2 is the packed four-byte
// Offset/Length/Y span array, and r3 is its count. This common unshifted
// map path pays one call/stack frame for the whole layer. Clipped or shifted
// maps retain the general Go path. Bytes outside spans remain untouched.
.section .iwram.map_raster_copy,"ax",%progbits
.align 2
.arm
.global mtile_copy_map_raster_full
.type mtile_copy_map_raster_full,%function
mtile_copy_map_raster_full:
    stmdb   sp!, {r4-r11, lr}
    mov     r4, r0
    mov     r5, r1
    mov     r6, r2
    movs    r7, r3
    beq     .Lmap_raster_done
.Lmap_raster_next:
    ldrh    r0, [r6]
    ldrb    r2, [r6, #2]
    add     r6, r6, #4
    add     r1, r5, r0
    add     r0, r4, r0
.Lmap_raster_prefix:
    cmp     r2, #0
    beq     .Lmap_raster_advance
    tst     r1, #3
    beq     .Lmap_raster_words16
    ldrb    r3, [r0], #1
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    sub     r2, r2, #1
    b       .Lmap_raster_prefix
.Lmap_raster_words16:
    cmp     r2, #16
    blo     .Lmap_raster_words4
    ldmia   r0!, {r8-r11}
    stmia   r1!, {r8-r11}
    sub     r2, r2, #16
    b       .Lmap_raster_words16
.Lmap_raster_words4:
    cmp     r2, #4
    blo     .Lmap_raster_tail
    ldr     r3, [r0], #4
    str     r3, [r1], #4
    sub     r2, r2, #4
    b       .Lmap_raster_words4
.Lmap_raster_tail:
    cmp     r2, #0
    beq     .Lmap_raster_advance
    ldrb    r3, [r0], #1
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    subs    r2, r2, #1
    bne     .Lmap_raster_tail
.Lmap_raster_advance:
    subs    r7, r7, #1
    bne     .Lmap_raster_next
.Lmap_raster_done:
    ldmia   sp!, {r4-r11, lr}
    bx      lr
.size mtile_copy_map_raster_full, .-mtile_copy_map_raster_full

// Width-240 raster with arbitrary camera/clip/cell-subrectangle bounds.
// r0/r1/r2/r3: source base, framebuffer base, spans, count.
// Stack arguments: destination origin offset, x0, x1, y0, y1 (source pixels).
// The caller guarantees allocated spans, source-base alignment and clipped
// destination addresses. Every unaligned copy combines aligned source words
// inside the source allocation; writes never cross an opaque run's boundary.
.section .iwram.map_raster_shifted,"ax",%progbits
.align 2
.arm
.global mtile_copy_map_raster_shifted
.type mtile_copy_map_raster_shifted,%function
mtile_copy_map_raster_shifted:
    stmdb   sp!, {r4-r11, lr}
    mov     r4, r0
    ldr     r12, [sp, #36]
    add     r5, r1, r12
    mov     r6, r2
    movs    r7, r3
    beq     .Lmap_shift_done
    ldr     r10, [sp, #48]
    ldr     r11, [sp, #52]
.Lmap_shift_next:
    ldrb    r3, [r6, #3]
    cmp     r3, r10
    blt     .Lmap_shift_advance
    cmp     r3, r11
    bge     .Lmap_shift_done
    ldrh    r0, [r6]
    ldrb    r2, [r6, #2]
    rsb     lr, r3, r3, lsl #4
    mov     lr, lr, lsl #4              // y * 240
    sub     r12, r0, lr                 // run's starting x
    ldr     r8, [sp, #40]
    ldr     r9, [sp, #44]
    add     r3, r12, r2
    cmp     r3, r9
    movgt   r3, r9
    cmp     r12, r8
    movlt   r12, r8
    subs    r2, r3, r12
    ble     .Lmap_shift_advance
    add     r0, lr, r12
    add     r1, r5, r0
    add     r0, r4, r0
.Lmap_shift_prefix:
    cmp     r2, #0
    beq     .Lmap_shift_advance
    tst     r1, #3
    beq     .Lmap_shift_alignment
    ldrb    r3, [r0], #1
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    sub     r2, r2, #1
    b       .Lmap_shift_prefix
.Lmap_shift_alignment:
    ands    lr, r0, #3
    bne     .Lmap_shift_unaligned
.Lmap_shift_words16:
    cmp     r2, #16
    blo     .Lmap_shift_words4
    ldmia   r0!, {r3, r8, r9, r12}
    stmia   r1!, {r3, r8, r9, r12}
    sub     r2, r2, #16
    b       .Lmap_shift_words16
.Lmap_shift_words4:
    cmp     r2, #4
    blo     .Lmap_shift_tail
    ldr     r3, [r0], #4
    str     r3, [r1], #4
    sub     r2, r2, #4
    b       .Lmap_shift_words4
.Lmap_shift_unaligned:
    cmp     r2, #4
    blo     .Lmap_shift_tail
    mov     r12, lr, lsl #3
    bic     r0, r0, #3
    ldr     r3, [r0], #4
    cmp     r12, #8
    beq     .Lmap_shift_merge8
    cmp     r12, #16
    beq     .Lmap_shift_merge16
    b       .Lmap_shift_merge24

// Camera alignment is constant throughout a run. Immediate shifts avoid
// ARM7's extra cycle for register-controlled shifts, and each loop emits
// two words while carrying the last aligned source word into the next loop.
// Every source row ends on a four-byte boundary. For n >= 8, rounding the
// eight copied bytes up to an aligned look-ahead word stays inside that row,
// even when the extra bytes after the opaque run are transparent. This keeps
// only 0-3 byte writes at the tail instead of 4-7 costly EWRAM byte writes.
.macro map_shift_merge name, right, left
.Lmap_shift_\name:
    cmp     r2, #8
    blo     .Lmap_shift_\name\()_single
.Lmap_shift_\name\()_pair:
    ldr     r8, [r0], #4
    mov     r9, r3, lsr #\right
    orr     r9, r9, r8, lsl #\left
    str     r9, [r1], #4
    ldr     r3, [r0], #4
    mov     r9, r8, lsr #\right
    orr     r9, r9, r3, lsl #\left
    str     r9, [r1], #4
    sub     r2, r2, #8
    cmp     r2, #8
    bhs     .Lmap_shift_\name\()_pair
.Lmap_shift_\name\()_single:
    cmp     r2, #4
    blo     .Lmap_shift_restore
    ldr     r8, [r0], #4
    mov     r9, r3, lsr #\right
    orr     r9, r9, r8, lsl #\left
    str     r9, [r1], #4
    sub     r2, r2, #4
    b       .Lmap_shift_restore
.endm
    map_shift_merge merge8, 8, 24
    map_shift_merge merge16, 16, 16
    map_shift_merge merge24, 24, 8
.purgem map_shift_merge
.Lmap_shift_restore:
    sub     r0, r0, #4
    add     r0, r0, r12, lsr #3         // restore the exact byte pointer
.Lmap_shift_tail:
    cmp     r2, #0
    beq     .Lmap_shift_advance
    ldrb    r3, [r0], #1
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    subs    r2, r2, #1
    bne     .Lmap_shift_tail
.Lmap_shift_advance:
    add     r6, r6, #4
    subs    r7, r7, #1
    bne     .Lmap_shift_next
.Lmap_shift_done:
    ldmia   sp!, {r4-r11, lr}
    bx      lr
.size mtile_copy_map_raster_shifted, .-mtile_copy_map_raster_shifted

// C/AAPCS memset used by TinyGo's __aeabi_memclr/__aeabi_memset helpers.
// LLVM recognizes Go zeroing loops and lowers them back to this libc symbol;
// optimizing the Go loop alone can therefore leave byte-at-a-time ROM code.
// r0=destination (also returned), r1=byte value, r2=byte count. Only caller-
// saved registers change. Prefix/tail stores never touch bytes outside the
// requested range, and every word/multiple store has a word-aligned address.
.section .iwram.memset,"ax",%progbits
.align 2
.arm
.global memset
.type memset,%function
memset:
    mov     r3, r0
    cmp     r2, #0
    bxeq    lr
    and     r1, r1, #255
.Lmemset_align:
    tst     r3, #3
    beq     .Lmemset_words
    strb    r1, [r3], #1
    subs    r2, r2, #1
    bxeq    lr
    b       .Lmemset_align
.Lmemset_words:
    orr     r1, r1, r1, lsl #8
    orr     r1, r1, r1, lsl #16
    mov     r12, r1
    cmp     r2, #16
    blo     .Lmemset_word_tail
.Lmemset_block:
    stmia   r3!, {r1, r12}
    stmia   r3!, {r1, r12}
    sub     r2, r2, #16
    cmp     r2, #16
    bhs     .Lmemset_block
.Lmemset_word_tail:
    cmp     r2, #4
    blo     .Lmemset_byte_tail
    str     r1, [r3], #4
    sub     r2, r2, #4
    b       .Lmemset_word_tail
.Lmemset_byte_tail:
    cmp     r2, #0
    bxeq    lr
.Lmemset_byte:
    strb    r1, [r3], #1
    subs    r2, r2, #1
    bne     .Lmemset_byte
    bx      lr
.size memset, .-memset

// Shared C/AAPCS memcpy/memmove. Keeping memmove's overlap semantics for both
// entry points is harmless for memcpy's non-overlapping contract. Equal
// source/destination alignment admits aligned LDM/STM bursts; mismatched
// alignment retains byte copies without reading outside either allocation.
// r0=destination/return value, r1=source, r2=count. Preserve r4-r11 and lr.
.section .iwram.memmove,"ax",%progbits
.align 2
.arm
.global memcpy
.global memmove
.type memcpy,%function
.type memmove,%function
memcpy:
memmove:
    cmp     r2, #0
    bxeq    lr
    cmp     r0, r1
    bxeq    lr
    mov     r3, r0
    sub     r12, r0, r1
    cmp     r12, r2
    blo     .Lmemmove_backward
    eor     r12, r0, r1
    tst     r12, #3
    bne     .Lmemmove_forward_bytes
.Lmemmove_forward_align:
    tst     r3, #3
    beq     .Lmemmove_forward_words
    ldrb    r12, [r1], #1
    strb    r12, [r3], #1
    subs    r2, r2, #1
    bxeq    lr
    b       .Lmemmove_forward_align
.Lmemmove_forward_words:
    cmp     r2, #16
    blo     .Lmemmove_forward_word_tail
    push    {r4-r7}
.Lmemmove_forward_block:
    ldmia   r1!, {r4-r7}
    stmia   r3!, {r4-r7}
    sub     r2, r2, #16
    cmp     r2, #16
    bhs     .Lmemmove_forward_block
    pop     {r4-r7}
.Lmemmove_forward_word_tail:
    cmp     r2, #4
    blo     .Lmemmove_forward_byte_tail
    ldr     r12, [r1], #4
    str     r12, [r3], #4
    sub     r2, r2, #4
    b       .Lmemmove_forward_word_tail
.Lmemmove_forward_byte_tail:
    cmp     r2, #0
    bxeq    lr
.Lmemmove_forward_bytes:
    ldrb    r12, [r1], #1
    strb    r12, [r3], #1
    subs    r2, r2, #1
    bne     .Lmemmove_forward_bytes
    bx      lr
.Lmemmove_backward:
    add     r1, r1, r2
    add     r3, r3, r2
    eor     r12, r1, r3
    tst     r12, #3
    bne     .Lmemmove_backward_bytes
.Lmemmove_backward_align:
    tst     r3, #3
    beq     .Lmemmove_backward_words
    ldrb    r12, [r1, #-1]!
    strb    r12, [r3, #-1]!
    subs    r2, r2, #1
    bxeq    lr
    b       .Lmemmove_backward_align
.Lmemmove_backward_words:
    cmp     r2, #16
    blo     .Lmemmove_backward_word_tail
    push    {r4-r7}
.Lmemmove_backward_block:
    ldmdb   r1!, {r4-r7}
    stmdb   r3!, {r4-r7}
    sub     r2, r2, #16
    cmp     r2, #16
    bhs     .Lmemmove_backward_block
    pop     {r4-r7}
.Lmemmove_backward_word_tail:
    cmp     r2, #4
    blo     .Lmemmove_backward_byte_tail
    ldr     r12, [r1, #-4]!
    str     r12, [r3, #-4]!
    sub     r2, r2, #4
    b       .Lmemmove_backward_word_tail
.Lmemmove_backward_byte_tail:
    cmp     r2, #0
    bxeq    lr
.Lmemmove_backward_bytes:
    ldrb    r12, [r1, #-1]!
    strb    r12, [r3, #-1]!
    subs    r2, r2, #1
    bne     .Lmemmove_backward_bytes
    bx      lr
.size memcpy, .-memcpy
.size memmove, .-memmove

// ARM7TDMI has no divide instruction. TinyGo/LLVM lower / and % to EABI
// helpers from compiler-rt, which live in .text and therefore run from
// 16-bit ROM. The inner long-division loop is then wait-stated on every
// iteration. These normalized shift/subtract loops replace those archive objects
// (--gc-sections drops the ROM copies) and execute from IWRAM, the same
// placement used for memset/memcpy. Each width is its own section so a
// cartridge that never divides 64-bit values does not keep the wider loop.
//
// 32-bit: r0=numerator, r1=denominator. Returns r0=quotient and, for
// *divmod, r1=remainder. 64-bit: r0:r1 numerator, r2:r3 denominator
// (low:high); returns r0:r1 quotient and r2:r3 remainder. Division by
// zero returns 0. Signed remainder follows the dividend (toward zero).
.section .iwram.uidiv,"ax",%progbits
.align 2
.arm
.global __aeabi_uidiv
.global __udivsi3
.global __aeabi_uidivmod
.type __aeabi_uidiv,%function
.type __udivsi3,%function
.type __aeabi_uidivmod,%function
__aeabi_uidiv:
__udivsi3:
__aeabi_uidivmod:
    cmp     r1, #0
    beq     .Luidiv_zero
    cmp     r1, #1
    moveq   r1, #0
    bxeq    lr
    cmp     r0, r1
    blo     .Luidiv_small
    // If d > n/2, subtracting it once completes the division. Comparing
    // against the shifted numerator also covers d >= 2^31 without overflow.
    cmp     r1, r0, lsr #1
    subhi   r1, r0, r1
    movhi   r0, #1
    bxhi    lr
    sub     r2, r1, #1
    tst     r1, r2
    beq     .Luidiv_pow2
    mov     r2, r1
    mov     r3, #1
    // Normalize the divisor in five bounded steps. Each accepted shift
    // stays <= n, so it cannot discard a high bit.
    cmp     r2, r0, lsr #16
    movls   r2, r2, lsl #16
    movls   r3, r3, lsl #16
    cmp     r2, r0, lsr #8
    movls   r2, r2, lsl #8
    movls   r3, r3, lsl #8
    cmp     r2, r0, lsr #4
    movls   r2, r2, lsl #4
    movls   r3, r3, lsl #4
    cmp     r2, r0, lsr #2
    movls   r2, r2, lsl #2
    movls   r3, r3, lsl #2
    cmp     r2, r0, lsr #1
    movls   r2, r2, lsl #1
    movls   r3, r3, lsl #1
    // Normalization proves the leading quotient bit is one. The earlier
    // n < 2*d exit proves a second bit remains, so enter the loop after it.
    sub     r0, r0, r2
    mov     r12, r3
    lsr     r2, r2, #1
    lsr     r3, r3, #1
.Luidiv_loop:
    cmp     r0, r2
    subhs   r0, r0, r2
    orrhs   r12, r12, r3
    lsr     r2, r2, #1
    lsrs    r3, r3, #1
    bne     .Luidiv_loop
    mov     r1, r0
    mov     r0, r12
    bx      lr
.Luidiv_pow2:
    // A power-of-two divisor needs only its bit index and a remainder mask.
    mov     r3, #0
    cmp     r1, #65536
    movhs   r1, r1, lsr #16
    addhs   r3, r3, #16
    cmp     r1, #256
    movhs   r1, r1, lsr #8
    addhs   r3, r3, #8
    cmp     r1, #16
    movhs   r1, r1, lsr #4
    addhs   r3, r3, #4
    cmp     r1, #4
    movhs   r1, r1, lsr #2
    addhs   r3, r3, #2
    cmp     r1, #2
    movhs   r1, r1, lsr #1
    addhs   r3, r3, #1
    and     r2, r0, r2
    lsr     r0, r0, r3
    mov     r1, r2
    bx      lr
.Luidiv_small:
    mov     r1, r0
    mov     r0, #0
    bx      lr
.Luidiv_zero:
    mov     r0, #0
    mov     r1, #0
    bx      lr
.size __aeabi_uidivmod, .-__aeabi_uidivmod
.size __aeabi_uidiv, .-__aeabi_uidiv
.size __udivsi3, .-__udivsi3

.section .iwram.idiv,"ax",%progbits
.align 2
.arm
.global __aeabi_idiv
.global __divsi3
.global __aeabi_idivmod
.type __aeabi_idiv,%function
.type __divsi3,%function
.type __aeabi_idivmod,%function
__aeabi_idiv:
__divsi3:
__aeabi_idivmod:
    // Keep both result signs in one saved register: quotient in bit 31,
    // remainder in bit 0. Two saved words keep the call frame aligned.
    stmdb   sp!, {r4, lr}
    eor     r4, r0, r1
    bic     r4, r4, #1
    orr     r4, r4, r0, lsr #31
    cmp     r0, #0
    rsblt   r0, r0, #0
    cmp     r1, #0
    rsblt   r1, r1, #0
    bl      __aeabi_uidivmod
    tst     r4, #1
    rsbne   r1, r1, #0
    cmp     r4, #0
    rsblt   r0, r0, #0
    ldmia   sp!, {r4, lr}
    bx      lr
.size __aeabi_idivmod, .-__aeabi_idivmod
.size __aeabi_idiv, .-__aeabi_idiv
.size __divsi3, .-__divsi3

.section .iwram.uldiv,"ax",%progbits
.align 2
.arm
.global __aeabi_uldivmod
.type __aeabi_uldivmod,%function
__aeabi_uldivmod:
    orrs    r12, r1, r3
    bne     .Luldiv_wide
    // The narrow case needs only a return address save. Reserve an aligned
    // eight-byte frame before calling the 32-bit EABI helper.
    str     lr, [sp, #-8]!
    mov     r1, r2
    bl      __aeabi_uidivmod
    mov     r2, r1
    mov     r1, #0
    mov     r3, #0
    ldr     lr, [sp], #8
    bx      lr
.Luldiv_wide:
    orrs    r12, r2, r3
    beq     .Luldiv_zero
    cmp     r1, r3
    cmpeq   r0, r2
    blo     .Luldiv_small
    cmp     r3, #0
    cmpeq   r2, #1
    moveq   r2, #0
    bxeq    lr
    // d > n/2 implies a quotient of one, without computing 2*d.
    mov     r12, r0, lsr #1
    orr     r12, r12, r1, lsl #31
    cmp     r3, r1, lsr #1
    cmpeq   r2, r12
    bhi     .Luldiv_one
    stmdb   sp!, {r4-r7}
    mov     r12, #1
    // Shift by 32 first when both high-word comparisons prove it fits.
    cmp     r3, #0
    cmpeq   r2, r1
    movls   r3, r2
    movls   r2, #0
    addls   r12, r12, #32
    mov     r6, r0, lsr #16
    orr     r6, r6, r1, lsl #16
    cmp     r3, r1, lsr #16
    cmpeq   r2, r6
    movls   r3, r3, lsl #16
    orrls   r3, r3, r2, lsr #16
    movls   r2, r2, lsl #16
    addls   r12, r12, #16
    mov     r6, r0, lsr #8
    orr     r6, r6, r1, lsl #24
    cmp     r3, r1, lsr #8
    cmpeq   r2, r6
    movls   r3, r3, lsl #8
    orrls   r3, r3, r2, lsr #24
    movls   r2, r2, lsl #8
    addls   r12, r12, #8
    mov     r6, r0, lsr #4
    orr     r6, r6, r1, lsl #28
    cmp     r3, r1, lsr #4
    cmpeq   r2, r6
    movls   r3, r3, lsl #4
    orrls   r3, r3, r2, lsr #28
    movls   r2, r2, lsl #4
    addls   r12, r12, #4
    mov     r6, r0, lsr #2
    orr     r6, r6, r1, lsl #30
    cmp     r3, r1, lsr #2
    cmpeq   r2, r6
    movls   r3, r3, lsl #2
    orrls   r3, r3, r2, lsr #30
    movls   r2, r2, lsl #2
    addls   r12, r12, #2
    mov     r6, r0, lsr #1
    orr     r6, r6, r1, lsl #31
    cmp     r3, r1, lsr #1
    cmpeq   r2, r6
    movls   r3, r3, lsl #1
    orrls   r3, r3, r2, lsr #31
    movls   r2, r2, lsl #1
    addls   r12, r12, #1
    // The normalized divisor fits, and q >= 2, so the first quotient bit
    // can be committed without a trial subtraction or a loop branch.
    subs    r0, r0, r2
    sbc     r1, r1, r3
    mov     r4, #1
    mov     r5, #0
    lsrs    r3, r3, #1
    rrx     r2, r2
    sub     r12, r12, #1
.Luldiv_loop:
    // The final subtraction carry is the next quotient bit. Commit the
    // trial remainder only when unsigned n >= d, then shift that carry
    // through the two quotient words.
    subs    r6, r0, r2
    sbcs    r7, r1, r3
    movhs   r0, r6
    movhs   r1, r7
    adcs    r4, r4, r4
    adc     r5, r5, r5
    lsrs    r3, r3, #1
    rrx     r2, r2
    subs    r12, r12, #1
    bne     .Luldiv_loop
    mov     r2, r0
    mov     r3, r1
    mov     r0, r4
    mov     r1, r5
    ldmia   sp!, {r4-r7}
    bx      lr
.Luldiv_one:
    subs    r2, r0, r2
    sbc     r3, r1, r3
    mov     r0, #1
    mov     r1, #0
    bx      lr
.Luldiv_small:
    mov     r2, r0
    mov     r3, r1
    mov     r0, #0
    mov     r1, #0
    bx      lr
.Luldiv_zero:
    mov     r0, #0
    mov     r1, #0
    mov     r2, #0
    mov     r3, #0
    bx      lr
.size __aeabi_uldivmod, .-__aeabi_uldivmod

.section .iwram.ldiv,"ax",%progbits
.align 2
.arm
.global __aeabi_ldivmod
.type __aeabi_ldivmod,%function
__aeabi_ldivmod:
    stmdb   sp!, {r4, lr}
    eor     r4, r1, r3
    bic     r4, r4, #1
    orr     r4, r4, r1, lsr #31
    cmp     r1, #0
    bge     .Lldiv_nabs
    rsbs    r0, r0, #0
    rsc     r1, r1, #0
.Lldiv_nabs:
    cmp     r3, #0
    bge     .Lldiv_dabs
    rsbs    r2, r2, #0
    rsc     r3, r3, #0
.Lldiv_dabs:
    bl      __aeabi_uldivmod
    cmp     r4, #0
    bge     .Lldiv_qsign
    rsbs    r0, r0, #0
    rsc     r1, r1, #0
.Lldiv_qsign:
    tst     r4, #1
    beq     .Lldiv_rsign
    rsbs    r2, r2, #0
    rsc     r3, r3, #0
.Lldiv_rsign:
    ldmia   sp!, {r4, lr}
    bx      lr
.size __aeabi_ldivmod, .-__aeabi_ldivmod

// Composite a full identity-palette, key-zero 8x8 cell into the EWRAM
// framebuffer. r0 is a four-byte-aligned contiguous source; r1 is an
// arbitrarily aligned destination with a 240-byte row stride. Opaque rows
// use words/halfwords without unaligned accesses. Mixed rows retain every
// transparent destination byte, including the bytes before/after the cell.
.section .iwram.mapcell,"ax",%progbits
.syntax unified
.align 2
.arm
.global mtile_copy_map_cell8_key0
.type mtile_copy_map_cell8_key0,%function
mtile_copy_map_cell8_key0:
    stmdb   sp!, {r4-r9}
    mov     r2, #8
    ldr     r5, .Lmapcell_ones
    mov     r6, r5, lsl #7
.Lmapcell_row:
    ldmia   r0!, {r3, r12}
    orrs    r4, r3, r12
    beq     .Lmapcell_next
    sub     r4, r3, r5
    bic     r4, r4, r3
    sub     r7, r12, r5
    bic     r7, r7, r12
    orr     r4, r4, r7
    tst     r4, r6
    bne     .Lmapcell_mixed
    tst     r1, #3
    streq   r3, [r1]
    streq   r12, [r1, #4]
    beq     .Lmapcell_next
    tst     r1, #1
    beq     .Lmapcell_halfwords
    vram_strb r3, r1, r8, r9
    mov     r4, r3, lsr #8
    strh    r4, [r1, #1]
    mov     r4, r3, lsr #24
    orr     r4, r4, r12, lsl #8
    strh    r4, [r1, #3]
    mov     r4, r12, lsr #8
    strh    r4, [r1, #5]
    mov     r4, r12, lsr #24
    add     r7, r1, #7
    vram_strb r4, r7, r8, r9
    b       .Lmapcell_next
.Lmapcell_halfwords:
    strh    r3, [r1]
    mov     r4, r3, lsr #16
    strh    r4, [r1, #2]
    strh    r12, [r1, #4]
    mov     r4, r12, lsr #16
    strh    r4, [r1, #6]
    b       .Lmapcell_next
.Lmapcell_mixed:
    // Store each nonzero source byte; zero keeps the destination pixel.
    mov     r4, r1
    mov     r7, #4
.Lmapcell_mixed_first:
    tst     r3, #255
    beq     .Lmapcell_mixed_first_skip
    vram_strb r3, r4, r8, r9
.Lmapcell_mixed_first_skip:
    add     r4, r4, #1
    mov     r3, r3, lsr #8
    subs    r7, r7, #1
    bne     .Lmapcell_mixed_first
    mov     r7, #4
.Lmapcell_mixed_second:
    tst     r12, #255
    beq     .Lmapcell_mixed_second_skip
    vram_strb r12, r4, r8, r9
.Lmapcell_mixed_second_skip:
    add     r4, r4, #1
    mov     r12, r12, lsr #8
    subs    r7, r7, #1
    bne     .Lmapcell_mixed_second
.Lmapcell_next:
    add     r1, r1, #240
    subs    r2, r2, #1
    bne     .Lmapcell_row
    ldmia   sp!, {r4-r9}
    bx      lr
.Lmapcell_ones:
    .word   0x01010101
.size mtile_copy_map_cell8_key0, .-mtile_copy_map_cell8_key0

// Batch copy/fill of opaque packed spans. X,Y,Length,Offset are uint16.
// Length bit15 marks a constant-colour span; Offset then stores the colour.
// Source and each copy span offset are 4-aligned; framebuffer stride is 240.
// Uniform spans read no source pixels. GBA source addresses have bit31 clear.
// r0/r1/r2/r3: packed pixels, framebuffer, spans, span count.
// Stack: originOffset,x0,x1,y0,y1. Bounds are source pixel coordinates.
// Spans are sorted by Y. Destination bounds have already been clipped.
// Word loads stay inside the span's bytes, without relying on padding.
.section .iwram.map_packed,"ax",%progbits
.syntax unified
.align 2
.arm
.global mtile_copy_map_packed_spans
.type mtile_copy_map_packed_spans,%function
mtile_copy_map_packed_spans:
    stmdb   sp!, {r4-r11, lr}
    mov     r4, r0
    ldr     r12, [sp, #36]
    add     r5, r1, r12
    mov     r6, r2
    movs    r7, r3
    beq     .Lmap_packed_done
    ldr     r10, [sp, #48]
    ldr     r11, [sp, #52]
.Lmap_packed_next:
    ldrh    r3, [r6, #2]
    cmp     r3, r10
    blt     .Lmap_packed_advance
    cmp     r3, r11
    bge     .Lmap_packed_done
    ldrh    r12, [r6]
    ldrh    r2, [r6, #4]
    ldrh    r0, [r6, #6]
    tst     r2, #0x8000
    bic     r2, r2, #0x8000
    orrne   r0, r0, #0x80000000
    addeq   r0, r4, r0
    subeq   r0, r0, r12
    rsb     lr, r3, r3, lsl #4
    mov     lr, lr, lsl #4
    ldr     r8, [sp, #40]
    ldr     r9, [sp, #44]
    add     r3, r12, r2
    cmp     r3, r9
    movgt   r3, r9
    cmp     r12, r8
    movlt   r12, r8
    subs    r2, r3, r12
    ble     .Lmap_packed_advance
    add     r1, r5, lr
    add     r1, r1, r12
    cmp     r0, #0
    blt     .Lmap_packed_uniform
    add     r0, r0, r12
.Lmap_packed_prefix:
    cmp     r2, #0
    beq     .Lmap_packed_advance
    tst     r1, #3
    beq     .Lmap_packed_alignment
    ldrb    r3, [r0], #1
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    sub     r2, r2, #1
    b       .Lmap_packed_prefix
.Lmap_packed_alignment:
    ands    lr, r0, #3
    bne     .Lmap_packed_unaligned
.Lmap_packed_words16:
    cmp     r2, #16
    blo     .Lmap_packed_words4
    ldmia   r0!, {r3, r8, r9, r12}
    stmia   r1!, {r3, r8, r9, r12}
    sub     r2, r2, #16
    b       .Lmap_packed_words16
.Lmap_packed_words4:
    cmp     r2, #4
    blo     .Lmap_packed_tail
    ldr     r3, [r0], #4
    str     r3, [r1], #4
    sub     r2, r2, #4
    b       .Lmap_packed_words4
.Lmap_packed_unaligned:
    cmp     r2, #8
    blo     .Lmap_packed_tail
    mov     r12, lr, lsl #3
    bic     r0, r0, #3
    ldr     r3, [r0], #4
    cmp     r12, #8
    beq     .Lmap_packed_merge8
    cmp     r12, #16
    beq     .Lmap_packed_merge16
    b       .Lmap_packed_merge24
.macro map_packed_merge name, right, left, offset
.Lmap_packed_\name:
    cmp     r2, #20
    blo     .Lmap_packed_\name\()_small
.Lmap_packed_\name\()_bulk:
    ldmia   r0!, {r8, r9, r12, lr}
    mov     r3, r3, lsr #\right
    orr     r3, r3, r8, lsl #\left
    mov     r8, r8, lsr #\right
    orr     r8, r8, r9, lsl #\left
    mov     r9, r9, lsr #\right
    orr     r9, r9, r12, lsl #\left
    mov     r12, r12, lsr #\right
    orr     r12, r12, lr, lsl #\left
    stmia   r1!, {r3, r8, r9, r12}
    mov     r3, lr
    sub     r2, r2, #16
    cmp     r2, #20
    bhs     .Lmap_packed_\name\()_bulk
.Lmap_packed_\name\()_small:
    cmp     r2, #8
    blo     .Lmap_packed_\name\()_restore
.Lmap_packed_\name\()_word:
    ldr     r8, [r0], #4
    mov     r9, r3, lsr #\right
    orr     r9, r9, r8, lsl #\left
    str     r9, [r1], #4
    mov     r3, r8
    sub     r2, r2, #4
    cmp     r2, #8
    bhs     .Lmap_packed_\name\()_word
.Lmap_packed_\name\()_restore:
    sub     r0, r0, #4
    add     r0, r0, #\offset
    b       .Lmap_packed_tail
.endm
    map_packed_merge merge8, 8, 24, 1
    map_packed_merge merge16, 16, 16, 2
    map_packed_merge merge24, 24, 8, 3
.purgem map_packed_merge
.Lmap_packed_tail:
    cmp     r2, #0
    beq     .Lmap_packed_advance
    ldrb    r3, [r0], #1
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    subs    r2, r2, #1
    bne     .Lmap_packed_tail
    b       .Lmap_packed_advance
.Lmap_packed_uniform:
    and     r3, r0, #255
.Lmap_packed_fill_prefix:
    tst     r1, #3
    beq     .Lmap_packed_fill_words
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    subs    r2, r2, #1
    beq     .Lmap_packed_advance
    b       .Lmap_packed_fill_prefix
.Lmap_packed_fill_words:
    orr     r3, r3, r3, lsl #8
    orr     r3, r3, r3, lsl #16
    mov     r8, r3
    mov     r9, r3
    mov     r12, r3
    cmp     r2, #16
    blo     .Lmap_packed_fill_word
.Lmap_packed_fill_bulk:
    stmia   r1!, {r3, r8, r9, r12}
    sub     r2, r2, #16
    cmp     r2, #16
    bhs     .Lmap_packed_fill_bulk
.Lmap_packed_fill_word:
    cmp     r2, #4
    blo     .Lmap_packed_fill_tail
    str     r3, [r1], #4
    sub     r2, r2, #4
    b       .Lmap_packed_fill_word
.Lmap_packed_fill_tail:
    cmp     r2, #0
    beq     .Lmap_packed_advance
.Lmap_packed_fill_byte:
    vram_strb r3, r1, r8, r9
    add     r1, r1, #1
    subs    r2, r2, #1
    bne     .Lmap_packed_fill_byte
.Lmap_packed_advance:
    add     r6, r6, #8
    subs    r7, r7, #1
    bne     .Lmap_packed_next
.Lmap_packed_done:
    ldmia   sp!, {r4-r11, lr}
    bx      lr
.size mtile_copy_map_packed_spans, .-mtile_copy_map_packed_spans

// r0 aligned destination, r1 packed word, r2 word count.
.section .iwram.fill_words,"ax",%progbits
.syntax unified
.align 2
.arm
.global mtile_fill_words
.type mtile_fill_words,%function
mtile_fill_words:
    cmp     r2, #0
    bxeq    lr
    stmdb   sp!, {r4-r9}
    mov     r3, r1
    mov     r4, r1
    mov     r5, r1
    mov     r6, r1
    mov     r7, r1
    mov     r8, r1
    mov     r9, r1
    cmp     r2, #16
    blo     .Lfill_words_tail
.Lfill_words_bulk:
    stmia   r0!, {r1, r3-r9}
    stmia   r0!, {r1, r3-r9}
    sub     r2, r2, #16
    cmp     r2, #16
    bhs     .Lfill_words_bulk
.Lfill_words_tail:
    cmp     r2, #0
    beq     .Lfill_words_done
.Lfill_words_word:
    str     r1, [r0], #4
    subs    r2, r2, #1
    bne     .Lfill_words_word
.Lfill_words_done:
    ldmia   sp!, {r4-r9}
    bx      lr
.size mtile_fill_words, .-mtile_fill_words
