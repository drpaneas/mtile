03002870 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject>:
 3002870:	e92d4ff0 	push	{r4, r5, r6, r7, r8, r9, sl, fp, lr}
 3002874:	e24dd00c 	sub	sp, sp, #12
 3002878:	e59f7250 	ldr	r7, [pc, #592]	@ 3002ad0 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x260>
 300287c:	e3a05fbd 	mov	r5, #756	@ 0x2f4
 3002880:	e3855b16 	orr	r5, r5, #22528	@ 0x5800
 3002884:	e7d73005 	ldrb	r3, [r7, r5]
 3002888:	e3530000 	cmp	r3, #0
 300288c:	0a000001 	beq	3002898 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x28>
 3002890:	e3a00000 	mov	r0, #0
 3002894:	ea000012 	b	30028e4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x74>
 3002898:	e1a02000 	mov	r2, r0
 300289c:	e3a00eae 	mov	r0, #2784	@ 0xae0
 30028a0:	e3a03004 	mov	r3, #4
 30028a4:	e3800a05 	orr	r0, r0, #20480	@ 0x5000
 30028a8:	e7d76000 	ldrb	r6, [r7, r0]
 30028ac:	e3a00000 	mov	r0, #0
 30028b0:	e3560000 	cmp	r6, #0
 30028b4:	1a00000a 	bne	30028e4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x74>
 30028b8:	e3a060ff 	mov	r6, #255	@ 0xff
 30028bc:	e3a0beff 	mov	fp, #4080	@ 0xff0
 30028c0:	e3866cff 	orr	r6, r6, #65280	@ 0xff00
 30028c4:	e38bba1f 	orr	fp, fp, #126976	@ 0x1f000
 30028c8:	e0018006 	and	r8, r1, r6
 30028cc:	e38b300c 	orr	r3, fp, #12
 30028d0:	e288a003 	add	sl, r8, #3
 30028d4:	e00a4003 	and	r4, sl, r3
 30028d8:	e3a03001 	mov	r3, #1
 30028dc:	e3540a0e 	cmp	r4, #57344	@ 0xe000
 30028e0:	9a000003 	bls	30028f4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x84>
 30028e4:	e1a01003 	mov	r1, r3
 30028e8:	e28dd00c 	add	sp, sp, #12
 30028ec:	e8bd4ff0 	pop	{r4, r5, r6, r7, r8, r9, sl, fp, lr}
 30028f0:	e12fff1e 	bx	lr
 30028f4:	e5970ad8 	ldr	r0, [r7, #2776]	@ 0xad8
 30028f8:	e3500a0e 	cmp	r0, #57344	@ 0xe000
 30028fc:	92600a0e 	rsbls	r0, r0, #57344	@ 0xe000
 3002900:	91540000 	cmpls	r4, r0
 3002904:	9a000007 	bls	3002928 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xb8>
 3002908:	e1a00004 	mov	r0, r4
 300290c:	e1a09001 	mov	r9, r1
 3002910:	e1a06002 	mov	r6, r2
 3002914:	eb000118 	bl	3002d7c <__ARMv5LongLdrPcThunk_(*github.com/drpaneas/mtile/runtime.Heap).prepareAllocation>
 3002918:	e31000ff 	tst	r0, #255	@ 0xff
 300291c:	0a000009 	beq	3002948 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xd8>
 3002920:	e1a03000 	mov	r3, r0
 3002924:	eaffffd9 	b	3002890 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x20>
 3002928:	e3a00fb7 	mov	r0, #732	@ 0x2dc
 300292c:	e3800b12 	orr	r0, r0, #18432	@ 0x4800
 3002930:	e19700b0 	ldrh	r0, [r7, r0]
 3002934:	e1500006 	cmp	r0, r6
 3002938:	0afffff2 	beq	3002908 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x98>
 300293c:	e3500b02 	cmp	r0, #2048	@ 0x800
 3002940:	3a000007 	bcc	3002964 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xf4>
 3002944:	ea00000a 	b	3002974 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x104>
 3002948:	e3a00fb7 	mov	r0, #732	@ 0x2dc
 300294c:	e1a02006 	mov	r2, r6
 3002950:	e1a01009 	mov	r1, r9
 3002954:	e3800b12 	orr	r0, r0, #18432	@ 0x4800
 3002958:	e19700b0 	ldrh	r0, [r7, r0]
 300295c:	e3500b02 	cmp	r0, #2048	@ 0x800
 3002960:	2a000003 	bcs	3002974 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x104>
 3002964:	e0879180 	add	r9, r7, r0, lsl #3
 3002968:	e5f93adc 	ldrb	r3, [r9, #2780]!	@ 0xadc
 300296c:	e3130003 	tst	r3, #3
 3002970:	0a000002 	beq	3002980 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x110>
 3002974:	e3a03002 	mov	r3, #2
 3002978:	e7c73005 	strb	r3, [r7, r5]
 300297c:	eaffffc3 	b	3002890 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x20>
 3002980:	e5993004 	ldr	r3, [r9, #4]
 3002984:	e597cad8 	ldr	ip, [r7, #2776]	@ 0xad8
 3002988:	e3a06fb7 	mov	r6, #732	@ 0x2dc
 300298c:	e3540000 	cmp	r4, #0
 3002990:	e3866b12 	orr	r6, r6, #18432	@ 0x4800
 3002994:	e18730b6 	strh	r3, [r7, r6]
 3002998:	0a00003c 	beq	3002a90 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x220>
 300299c:	e5973ad4 	ldr	r3, [r7, #2772]	@ 0xad4
 30029a0:	e59f612c 	ldr	r6, [pc, #300]	@ 3002ad4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x264>
 30029a4:	e59f512c 	ldr	r5, [pc, #300]	@ 3002ad8 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x268>
 30029a8:	e3530000 	cmp	r3, #0
 30029ac:	01a05006 	moveq	r5, r6
 30029b0:	e3a06000 	mov	r6, #0
 30029b4:	e085300c 	add	r3, r5, ip
 30029b8:	e3130003 	tst	r3, #3
 30029bc:	1a000023 	bne	3002a50 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1e0>
 30029c0:	e358000d 	cmp	r8, #13
 30029c4:	e98d0005 	stmib	sp, {r0, r2}
 30029c8:	3a00000e 	bcc	3002a08 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x198>
 30029cc:	e248600d 	sub	r6, r8, #13
 30029d0:	e1a00003 	mov	r0, r3
 30029d4:	e58d1000 	str	r1, [sp]
 30029d8:	e356000f 	cmp	r6, #15
 30029dc:	23a0600f 	movcs	r6, #15
 30029e0:	e0486006 	sub	r6, r8, r6
 30029e4:	e2866002 	add	r6, r6, #2
 30029e8:	e006600b 	and	r6, r6, fp
 30029ec:	e1a0b00c 	mov	fp, ip
 30029f0:	e2866010 	add	r6, r6, #16
 30029f4:	e1a01006 	mov	r1, r6
 30029f8:	eb0000e1 	bl	3002d84 <__ARMv5LongLdrPcThunk___aeabi_memclr4>
 30029fc:	e59d1000 	ldr	r1, [sp]
 3002a00:	e99d0005 	ldmib	sp, {r0, r2}
 3002a04:	e1a0c00b 	mov	ip, fp
 3002a08:	e04a3006 	sub	r3, sl, r6
 3002a0c:	e3530004 	cmp	r3, #4
 3002a10:	3a00000e 	bcc	3002a50 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1e0>
 3002a14:	e0853006 	add	r3, r5, r6
 3002a18:	e046e008 	sub	lr, r6, r8
 3002a1c:	e1a0a001 	mov	sl, r1
 3002a20:	e1a0b00c 	mov	fp, ip
 3002a24:	e083000c 	add	r0, r3, ip
 3002a28:	e3e03003 	mvn	r3, #3
 3002a2c:	e1c3300e 	bic	r3, r3, lr
 3002a30:	e2838004 	add	r8, r3, #4
 3002a34:	e1a01008 	mov	r1, r8
 3002a38:	eb0000d1 	bl	3002d84 <__ARMv5LongLdrPcThunk___aeabi_memclr4>
 3002a3c:	e59d0004 	ldr	r0, [sp, #4]
 3002a40:	e59d2008 	ldr	r2, [sp, #8]
 3002a44:	e1a0c00b 	mov	ip, fp
 3002a48:	e1a0100a 	mov	r1, sl
 3002a4c:	e0886006 	add	r6, r8, r6
 3002a50:	e1540006 	cmp	r4, r6
 3002a54:	9a00000d 	bls	3002a90 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x220>
 3002a58:	e0443006 	sub	r3, r4, r6
 3002a5c:	e0856006 	add	r6, r5, r6
 3002a60:	e1a0a000 	mov	sl, r0
 3002a64:	e1a08002 	mov	r8, r2
 3002a68:	e1a0500c 	mov	r5, ip
 3002a6c:	e086600c 	add	r6, r6, ip
 3002a70:	e1a00006 	mov	r0, r6
 3002a74:	e1a06001 	mov	r6, r1
 3002a78:	e1a01003 	mov	r1, r3
 3002a7c:	eb0000c0 	bl	3002d84 <__ARMv5LongLdrPcThunk___aeabi_memclr4>
 3002a80:	e1a0c005 	mov	ip, r5
 3002a84:	e1a0000a 	mov	r0, sl
 3002a88:	e1a01006 	mov	r1, r6
 3002a8c:	e1a02008 	mov	r2, r8
 3002a90:	e3a060f2 	mov	r6, #242	@ 0xf2
 3002a94:	e589c004 	str	ip, [r9, #4]
 3002a98:	e1c910b2 	strh	r1, [r9, #2]
 3002a9c:	e3a01001 	mov	r1, #1
 3002aa0:	e3e05019 	mvn	r5, #25
 3002aa4:	e08c3004 	add	r3, ip, r4
 3002aa8:	e3866c5a 	orr	r6, r6, #23040	@ 0x5a00
 3002aac:	e1811102 	orr	r1, r1, r2, lsl #2
 3002ab0:	e2455a05 	sub	r5, r5, #20480	@ 0x5000
 3002ab4:	e1b760b6 	ldrh	r6, [r7, r6]!	@ <UNPREDICTABLE>
 3002ab8:	e5c91000 	strb	r1, [r9]
 3002abc:	e2861001 	add	r1, r6, #1
 3002ac0:	e7873005 	str	r3, [r7, r5]
 3002ac4:	e3a03000 	mov	r3, #0
 3002ac8:	e1c710b0 	strh	r1, [r7]
 3002acc:	eaffff84 	b	30028e4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x74>
 3002ad0:	02001ba0 	.word	0x02001ba0
 3002ad4:	02009600 	.word	0x02009600
 3002ad8:	02017600 	.word	0x02017600
