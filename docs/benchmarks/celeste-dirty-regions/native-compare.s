// Full 38400-byte aligned framebuffer versus hidden VRAM, no shadow.
.section .iwram.compare_frame_words,"ax",%progbits
.syntax unified
.align 2
.arm
.global mtile_compare_frame_words
.type mtile_compare_frame_words,%function
mtile_compare_frame_words:
    add r2, r0, #38400
    stmdb sp!, {r2, r4-r11, lr}
.Lcompare_words_loop:
    ldmia r0!, {r4-r11}
    ldmia r1, {r2, r3, r12, lr}
    cmp r4, r2
    strne r4, [r1, #0]
    cmp r5, r3
    strne r5, [r1, #4]
    cmp r6, r12
    strne r6, [r1, #8]
    cmp r7, lr
    strne r7, [r1, #12]
    add r1, r1, #16
    ldmia r1, {r2, r3, r12, lr}
    cmp r8, r2
    strne r8, [r1, #0]
    cmp r9, r3
    strne r9, [r1, #4]
    cmp r10, r12
    strne r10, [r1, #8]
    cmp r11, lr
    strne r11, [r1, #12]
    add r1, r1, #16
    ldr r2, [sp]
    cmp r0, r2
    blo .Lcompare_words_loop
    ldmia sp!, {r2, r4-r11, lr}
    bx lr
.size mtile_compare_frame_words, .-mtile_compare_frame_words

// Full 38400-byte aligned framebuffer versus hidden VRAM, no shadow.
.section .iwram.compare_frame_blocks,"ax",%progbits
.syntax unified
.align 2
.arm
.global mtile_compare_frame_blocks
.type mtile_compare_frame_blocks,%function
mtile_compare_frame_blocks:
    add r2, r0, #38400
    stmdb sp!, {r2, r4-r11, lr}
.Lcompare_blocks_loop:
    ldmia r0!, {r4-r11}
    ldmia r1, {r2, r3, r12, lr}
    cmp r4, r2
    cmpeq r5, r3
    cmpeq r6, r12
    cmpeq r7, lr
    add r1, r1, #16
    ldmia r1, {r2, r3, r12, lr}
    cmpeq r8, r2
    cmpeq r9, r3
    cmpeq r10, r12
    cmpeq r11, lr
    sub r1, r1, #16
    stmiane r1!, {r4-r11}
    addeq r1, r1, #32
    ldr r2, [sp]
    cmp r0, r2
    blo .Lcompare_blocks_loop
    ldmia sp!, {r2, r4-r11, lr}
    bx lr
.size mtile_compare_frame_blocks, .-mtile_compare_frame_blocks

