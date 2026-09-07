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
    // access = 1 cycle, and enables the prefetch buffer; it leaves SRAM,
    // WS1, and WS2 at their defaults since this runtime uses none of them.
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
    strb    r3, [r1], #1
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
    strb    r3, [r1], #1
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
    strb    r3, [r1], #1
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
    strb    r3, [r1], #1
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
