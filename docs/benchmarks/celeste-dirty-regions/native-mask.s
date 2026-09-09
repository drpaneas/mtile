// Ideal supplied 20-row tile mask, low30 bits per row. No tracking cost.
// r0 framebuffer, r1 clear word or hidden VRAM, r2 mask pointer.
.section .iwram.mask_clear,"ax",%progbits
.syntax unified
.align 2
.arm
.global mtile_mask_clear
.type mtile_mask_clear,%function
mtile_mask_clear:
    stmdb sp!, {r4-r11, lr}
    sub sp, sp, #4
    mov r4, r0
    mov r5, r1
    mov r6, r2
    mov r7, #20
.Lmask_clear_band:
    ldr r8, [r6], #4
    mov r9, #0
    mvn r3, #0xc0000000
    cmp r8, r3
    moveq r8, #0
    moveq r10, #1920
    moveq r11, #1
    beq .Lmask_clear_copy
.Lmask_clear_next:
    cmp r8, #0
    beq .Lmask_clear_advance
.Lmask_clear_zero:
    tst r8, #1
    bne .Lmask_clear_start
    mov r8, r8, lsr #1
    add r9, r9, #8
    b .Lmask_clear_zero
.Lmask_clear_start:
    mov r10, #0
.Lmask_clear_ones:
    mov r8, r8, lsr #1
    add r10, r10, #8
    tst r8, #1
    bne .Lmask_clear_ones
    mov r11, #8
.Lmask_clear_copy:
    str r10, [sp]
    mov r2, r10
    add r0, r4, r9
    mov r3, r5
    mov r10, r5
    mov r12, r5
    mov lr, r5
.Lmask_clear_row:
    cmp r2, #16
    blo .Lmask_clear_tail
.Lmask_clear_words:
    stmia r0!, {r3, r10, r12, lr}
    sub r2, r2, #16
    cmp r2, #16
    bhs .Lmask_clear_words
.Lmask_clear_tail:
    cmp r2, #0
    stmiane r0!, {r3, r10}
    ldr r2, [sp]
    add r0, r0, #240
    sub r0, r0, r2
    subs r11, r11, #1
    bne .Lmask_clear_row
    ldr r10, [sp]
    add r9, r9, r10
    b .Lmask_clear_next
.Lmask_clear_advance:
    add r4, r4, #1920
    subs r7, r7, #1
    bne .Lmask_clear_band
    add sp, sp, #4
    ldmia sp!, {r4-r11, lr}
    bx lr
.size mtile_mask_clear, .-mtile_mask_clear

// Ideal supplied 20-row tile mask, low30 bits per row. No tracking cost.
// r0 framebuffer, r1 clear word or hidden VRAM, r2 mask pointer.
.section .iwram.mask_upload,"ax",%progbits
.syntax unified
.align 2
.arm
.global mtile_mask_upload
.type mtile_mask_upload,%function
mtile_mask_upload:
    stmdb sp!, {r4-r11, lr}
    sub sp, sp, #4
    mov r4, r0
    mov r5, r1
    mov r6, r2
    mov r7, #20
.Lmask_upload_band:
    ldr r8, [r6], #4
    mov r9, #0
    mvn r3, #0xc0000000
    cmp r8, r3
    moveq r8, #0
    moveq r10, #1920
    moveq r11, #1
    beq .Lmask_upload_copy
.Lmask_upload_next:
    cmp r8, #0
    beq .Lmask_upload_advance
.Lmask_upload_zero:
    tst r8, #1
    bne .Lmask_upload_start
    mov r8, r8, lsr #1
    add r9, r9, #8
    b .Lmask_upload_zero
.Lmask_upload_start:
    mov r10, #0
.Lmask_upload_ones:
    mov r8, r8, lsr #1
    add r10, r10, #8
    tst r8, #1
    bne .Lmask_upload_ones
    mov r11, #8
.Lmask_upload_copy:
    str r10, [sp]
    mov r2, r10
    add r0, r4, r9
    add r1, r5, r9
.Lmask_upload_row:
    cmp r2, #16
    blo .Lmask_upload_tail
.Lmask_upload_words:
    ldmia r0!, {r3, r10, r12, lr}
    stmia r1!, {r3, r10, r12, lr}
    sub r2, r2, #16
    cmp r2, #16
    bhs .Lmask_upload_words
.Lmask_upload_tail:
    cmp r2, #0
    ldmiane r0!, {r3, r10}
    stmiane r1!, {r3, r10}
    ldr r2, [sp]
    add r0, r0, #240
    sub r0, r0, r2
    add r1, r1, #240
    sub r1, r1, r2
    subs r11, r11, #1
    bne .Lmask_upload_row
    ldr r10, [sp]
    add r9, r9, r10
    b .Lmask_upload_next
.Lmask_upload_advance:
    add r4, r4, #1920
    add r5, r5, #1920
    subs r7, r7, #1
    bne .Lmask_upload_band
    add sp, sp, #4
    ldmia sp!, {r4-r11, lr}
    bx lr
.size mtile_mask_upload, .-mtile_mask_upload

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
