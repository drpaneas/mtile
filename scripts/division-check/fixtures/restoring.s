// Frozen pre-normalization Mtile helpers for the optional performance gate.
// Intentionally retain the original implementation and stack frames.
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
    // d >= 2^31: 32-bit n < 2d, so the quotient is 0 or 1. The restore
    // loop cannot hold 2d in a 32-bit remainder.
    tst     r1, #0x80000000
    subne   r1, r0, r1
    movne   r0, #1
    bxne    lr
    mov     r2, r0
    mov     r0, #0
    mov     r3, #0
    mov     r12, #32
.Luidiv_loop:
    lsl     r3, r3, #1
    orr     r3, r3, r2, lsr #31
    lsl     r2, r2, #1
    lsl     r0, r0, #1
    cmp     r3, r1
    subhs   r3, r3, r1
    addhs   r0, r0, #1
    subs    r12, r12, #1
    bne     .Luidiv_loop
    mov     r1, r3
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
    eor     r12, r0, r1
    stmdb   sp!, {r4, r12, lr}
    mov     r4, r0
    cmp     r0, #0
    rsblt   r0, r0, #0
    cmp     r1, #0
    rsblt   r1, r1, #0
    bl      __aeabi_uidivmod
    cmp     r4, #0
    rsblt   r1, r1, #0
    ldmia   sp!, {r4, r12, lr}
    cmp     r12, #0
    rsblt   r0, r0, #0
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
    stmdb   sp!, {r4-r7, lr}
    orrs    r12, r1, r3
    bne     .Luldiv_wide
    mov     r1, r2
    bl      __aeabi_uidivmod
    mov     r2, r1
    mov     r1, #0
    mov     r3, #0
    ldmia   sp!, {r4-r7, lr}
    bx      lr
.Luldiv_wide:
    orrs    r12, r2, r3
    beq     .Luldiv_zero
    cmp     r1, r3
    cmpeq   r0, r2
    blo     .Luldiv_small
    tst     r3, #0x80000000
    beq     .Luldiv_setup
    subs    r2, r0, r2
    sbc     r3, r1, r3
    mov     r0, #1
    mov     r1, #0
    ldmia   sp!, {r4-r7, lr}
    bx      lr
.Luldiv_small:
    mov     r2, r0
    mov     r3, r1
    mov     r0, #0
    mov     r1, #0
    ldmia   sp!, {r4-r7, lr}
    bx      lr
.Luldiv_setup:
    mov     r6, r0
    mov     r7, r1
    mov     r0, #0
    mov     r1, #0
    mov     r4, #0
    mov     r5, #0
    mov     r12, #64
.Luldiv_loop:
    lsl     r5, r5, #1
    orr     r5, r5, r4, lsr #31
    lsl     r4, r4, #1
    orr     r4, r4, r7, lsr #31
    lsl     r7, r7, #1
    orr     r7, r7, r6, lsr #31
    lsl     r6, r6, #1
    lsl     r1, r1, #1
    orr     r1, r1, r0, lsr #31
    lsl     r0, r0, #1
    cmp     r5, r3
    cmpeq   r4, r2
    blo     .Luldiv_next
    subs    r4, r4, r2
    sbc     r5, r5, r3
    adds    r0, r0, #1
    adc     r1, r1, #0
.Luldiv_next:
    subs    r12, r12, #1
    bne     .Luldiv_loop
    mov     r2, r4
    mov     r3, r5
    ldmia   sp!, {r4-r7, lr}
    bx      lr
.Luldiv_zero:
    mov     r0, #0
    mov     r1, #0
    mov     r2, #0
    mov     r3, #0
    ldmia   sp!, {r4-r7, lr}
    bx      lr
.size __aeabi_uldivmod, .-__aeabi_uldivmod

.section .iwram.ldiv,"ax",%progbits
.align 2
.arm
.global __aeabi_ldivmod
.type __aeabi_ldivmod,%function
__aeabi_ldivmod:
    stmdb   sp!, {r4, r5, lr}
    eor     r4, r1, r3
    mov     r5, r1
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
    cmp     r5, #0
    bge     .Lldiv_rsign
    rsbs    r2, r2, #0
    rsc     r3, r3, #0
.Lldiv_rsign:
    ldmia   sp!, {r4, r5, lr}
    bx      lr
.size __aeabi_ldivmod, .-__aeabi_ldivmod
