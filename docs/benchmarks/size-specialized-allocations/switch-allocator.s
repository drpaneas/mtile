03002870 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject>:
 3002870:	e92d4ff0 	push	{r4, r5, r6, r7, r8, r9, sl, fp, lr}
 3002874:	e24dd00c 	sub	sp, sp, #12
 3002878:	e59f6280 	ldr	r6, [pc, #640]	@ 3002b00 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x290>
 300287c:	e3a07fbd 	mov	r7, #756	@ 0x2f4
 3002880:	e3877b16 	orr	r7, r7, #22528	@ 0x5800
 3002884:	e7d62007 	ldrb	r2, [r6, r7]
 3002888:	e3520000 	cmp	r2, #0
 300288c:	0a000001 	beq	3002898 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x28>
 3002890:	e3a00000 	mov	r0, #0
 3002894:	ea00000f 	b	30028d8 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x68>
 3002898:	e1a05000 	mov	r5, r0
 300289c:	e3a00eae 	mov	r0, #2784	@ 0xae0
 30028a0:	e3a02004 	mov	r2, #4
 30028a4:	e3800a05 	orr	r0, r0, #20480	@ 0x5000
 30028a8:	e7d63000 	ldrb	r3, [r6, r0]
 30028ac:	e3a00000 	mov	r0, #0
 30028b0:	e3530000 	cmp	r3, #0
 30028b4:	1a000007 	bne	30028d8 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x68>
 30028b8:	e3a030ff 	mov	r3, #255	@ 0xff
 30028bc:	e3a02001 	mov	r2, #1
 30028c0:	e3833cff 	orr	r3, r3, #65280	@ 0xff00
 30028c4:	e001b003 	and	fp, r1, r3
 30028c8:	e28ba003 	add	sl, fp, #3
 30028cc:	e3ca4003 	bic	r4, sl, #3
 30028d0:	e3540a0e 	cmp	r4, #57344	@ 0xe000
 30028d4:	9a000003 	bls	30028e8 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x78>
 30028d8:	e1a01002 	mov	r1, r2
 30028dc:	e28dd00c 	add	sp, sp, #12
 30028e0:	e8bd4ff0 	pop	{r4, r5, r6, r7, r8, r9, sl, fp, lr}
 30028e4:	e12fff1e 	bx	lr
 30028e8:	e5960ad8 	ldr	r0, [r6, #2776]	@ 0xad8
 30028ec:	e3500a0e 	cmp	r0, #57344	@ 0xe000
 30028f0:	92600a0e 	rsbls	r0, r0, #57344	@ 0xe000
 30028f4:	91540000 	cmpls	r4, r0
 30028f8:	9a000006 	bls	3002918 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xa8>
 30028fc:	e1a00004 	mov	r0, r4
 3002900:	e1a08001 	mov	r8, r1
 3002904:	eb000128 	bl	3002dac <__ARMv5LongLdrPcThunk_(*github.com/drpaneas/mtile/runtime.Heap).prepareAllocation>
 3002908:	e31000ff 	tst	r0, #255	@ 0xff
 300290c:	0a000009 	beq	3002938 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xc8>
 3002910:	e1a02000 	mov	r2, r0
 3002914:	eaffffdd 	b	3002890 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x20>
 3002918:	e3a00fb7 	mov	r0, #732	@ 0x2dc
 300291c:	e3800b12 	orr	r0, r0, #18432	@ 0x4800
 3002920:	e19600b0 	ldrh	r0, [r6, r0]
 3002924:	e1500003 	cmp	r0, r3
 3002928:	0afffff3 	beq	30028fc <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x8c>
 300292c:	e3500b02 	cmp	r0, #2048	@ 0x800
 3002930:	3a000006 	bcc	3002950 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xe0>
 3002934:	ea000009 	b	3002960 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xf0>
 3002938:	e3a00fb7 	mov	r0, #732	@ 0x2dc
 300293c:	e1a01008 	mov	r1, r8
 3002940:	e3800b12 	orr	r0, r0, #18432	@ 0x4800
 3002944:	e19600b0 	ldrh	r0, [r6, r0]
 3002948:	e3500b02 	cmp	r0, #2048	@ 0x800
 300294c:	2a000003 	bcs	3002960 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xf0>
 3002950:	e0868180 	add	r8, r6, r0, lsl #3
 3002954:	e5f82adc 	ldrb	r2, [r8, #2780]!	@ 0xadc
 3002958:	e3120003 	tst	r2, #3
 300295c:	0a000002 	beq	300296c <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xfc>
 3002960:	e3a02002 	mov	r2, #2
 3002964:	e7c62007 	strb	r2, [r6, r7]
 3002968:	eaffffc8 	b	3002890 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x20>
 300296c:	e5982004 	ldr	r2, [r8, #4]
 3002970:	e5969ad8 	ldr	r9, [r6, #2776]	@ 0xad8
 3002974:	e3a03fb7 	mov	r3, #732	@ 0x2dc
 3002978:	e3540000 	cmp	r4, #0
 300297c:	e3833b12 	orr	r3, r3, #18432	@ 0x4800
 3002980:	e18620b3 	strh	r2, [r6, r3]
 3002984:	0a00004d 	beq	3002ac0 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x250>
 3002988:	e5962ad4 	ldr	r2, [r6, #2772]	@ 0xad4
 300298c:	e59f3170 	ldr	r3, [pc, #368]	@ 3002b04 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x294>
 3002990:	e59fc170 	ldr	ip, [pc, #368]	@ 3002b08 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x298>
 3002994:	e3a07000 	mov	r7, #0
 3002998:	e3520000 	cmp	r2, #0
 300299c:	01a0c003 	moveq	ip, r3
 30029a0:	e08c2009 	add	r2, ip, r9
 30029a4:	e3120003 	tst	r2, #3
 30029a8:	1a000036 	bne	3002a88 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x218>
 30029ac:	e3540010 	cmp	r4, #16
 30029b0:	8a000013 	bhi	3002a04 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x194>
 30029b4:	e24b3001 	sub	r3, fp, #1
 30029b8:	e1a07004 	mov	r7, r4
 30029bc:	e1a03123 	lsr	r3, r3, #2
 30029c0:	e3530003 	cmp	r3, #3
 30029c4:	8a00000e 	bhi	3002a04 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x194>
 30029c8:	e28f7000 	add	r7, pc, #0
 30029cc:	e797f103 	ldr	pc, [r7, r3, lsl #2]
 30029d0:	030029f8 	.word	0x030029f8
 30029d4:	030029f0 	.word	0x030029f0
 30029d8:	030029e8 	.word	0x030029e8
 30029dc:	030029e0 	.word	0x030029e0
 30029e0:	e3a03000 	mov	r3, #0
 30029e4:	e582300c 	str	r3, [r2, #12]
 30029e8:	e3a03000 	mov	r3, #0
 30029ec:	e5823008 	str	r3, [r2, #8]
 30029f0:	e3a03000 	mov	r3, #0
 30029f4:	e5823004 	str	r3, [r2, #4]
 30029f8:	e3a03000 	mov	r3, #0
 30029fc:	e1a07004 	mov	r7, r4
 3002a00:	e5823000 	str	r3, [r2]
 3002a04:	e04a2007 	sub	r2, sl, r7
 3002a08:	e58d0008 	str	r0, [sp, #8]
 3002a0c:	e58d1004 	str	r1, [sp, #4]
 3002a10:	e58d5000 	str	r5, [sp]
 3002a14:	e3520010 	cmp	r2, #16
 3002a18:	3a00000a 	bcc	3002a48 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1d8>
 3002a1c:	e04b1007 	sub	r1, fp, r7
 3002a20:	e08c0007 	add	r0, ip, r7
 3002a24:	e1a0a00c 	mov	sl, ip
 3002a28:	e241100d 	sub	r1, r1, #13
 3002a2c:	e0800009 	add	r0, r0, r9
 3002a30:	e3c1100f 	bic	r1, r1, #15
 3002a34:	e2815010 	add	r5, r1, #16
 3002a38:	e1a01005 	mov	r1, r5
 3002a3c:	eb0000dc 	bl	3002db4 <__ARMv5LongLdrPcThunk___aeabi_memclr4>
 3002a40:	e0857007 	add	r7, r5, r7
 3002a44:	ea000000 	b	3002a4c <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1dc>
 3002a48:	e1a0a00c 	mov	sl, ip
 3002a4c:	e0441007 	sub	r1, r4, r7
 3002a50:	e3510004 	cmp	r1, #4
 3002a54:	3a000007 	bcc	3002a78 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x208>
 3002a58:	e2411004 	sub	r1, r1, #4
 3002a5c:	e08a0007 	add	r0, sl, r7
 3002a60:	e3c11003 	bic	r1, r1, #3
 3002a64:	e0800009 	add	r0, r0, r9
 3002a68:	e2815004 	add	r5, r1, #4
 3002a6c:	e1a01005 	mov	r1, r5
 3002a70:	eb0000cf 	bl	3002db4 <__ARMv5LongLdrPcThunk___aeabi_memclr4>
 3002a74:	e0857007 	add	r7, r5, r7
 3002a78:	e59d5000 	ldr	r5, [sp]
 3002a7c:	e59d1004 	ldr	r1, [sp, #4]
 3002a80:	e59d0008 	ldr	r0, [sp, #8]
 3002a84:	e1a0c00a 	mov	ip, sl
 3002a88:	e1540007 	cmp	r4, r7
 3002a8c:	9a00000b 	bls	3002ac0 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x250>
 3002a90:	e08c3007 	add	r3, ip, r7
 3002a94:	e0442007 	sub	r2, r4, r7
 3002a98:	e1a0a005 	mov	sl, r5
 3002a9c:	e1a05001 	mov	r5, r1
 3002aa0:	e1a07000 	mov	r7, r0
 3002aa4:	e0833009 	add	r3, r3, r9
 3002aa8:	e1a01002 	mov	r1, r2
 3002aac:	e1a00003 	mov	r0, r3
 3002ab0:	eb0000bf 	bl	3002db4 <__ARMv5LongLdrPcThunk___aeabi_memclr4>
 3002ab4:	e1a01005 	mov	r1, r5
 3002ab8:	e1a00007 	mov	r0, r7
 3002abc:	e1a0500a 	mov	r5, sl
 3002ac0:	e3a030f2 	mov	r3, #242	@ 0xf2
 3002ac4:	e5889004 	str	r9, [r8, #4]
 3002ac8:	e1c810b2 	strh	r1, [r8, #2]
 3002acc:	e3a01001 	mov	r1, #1
 3002ad0:	e3e07019 	mvn	r7, #25
 3002ad4:	e0892004 	add	r2, r9, r4
 3002ad8:	e3833c5a 	orr	r3, r3, #23040	@ 0x5a00
 3002adc:	e1811105 	orr	r1, r1, r5, lsl #2
 3002ae0:	e2477a05 	sub	r7, r7, #20480	@ 0x5000
 3002ae4:	e1b630b3 	ldrh	r3, [r6, r3]!	@ <UNPREDICTABLE>
 3002ae8:	e5c81000 	strb	r1, [r8]
 3002aec:	e2831001 	add	r1, r3, #1
 3002af0:	e7862007 	str	r2, [r6, r7]
 3002af4:	e3a02000 	mov	r2, #0
 3002af8:	e1c610b0 	strh	r1, [r6]
 3002afc:	eaffff75 	b	30028d8 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x68>
 3002b00:	02001ba0 	.word	0x02001ba0
 3002b04:	02009600 	.word	0x02009600
 3002b08:	02017600 	.word	0x02017600
