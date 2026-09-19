.syntax unified
.arm
.text
.align 2
.global literal
literal:
 ldr r0, 1f
 bx lr
1: .word 0x040000de
.global immediate
immediate:
 mov r0, #0xde
 orr r0, r0, #0x04000000
 bx lr
