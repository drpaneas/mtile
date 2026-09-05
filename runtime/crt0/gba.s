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
