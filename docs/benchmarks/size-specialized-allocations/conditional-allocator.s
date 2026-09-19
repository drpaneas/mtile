03002870 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject>:
 3002870:	e92d4ff0 	push	{r4, r5, r6, r7, r8, r9, sl, fp, lr}
 3002874:	e24dd00c 	sub	sp, sp, #12
 3002878:	e59f7290 	ldr	r7, [pc, #656]	@ 3002b10 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x2a0>
 300287c:	e3a06fbd 	mov	r6, #756	@ 0x2f4
 3002880:	e3866b16 	orr	r6, r6, #22528	@ 0x5800
 3002884:	e7d73006 	ldrb	r3, [r7, r6]
 3002888:	e3530000 	cmp	r3, #0
 300288c:	0a000001 	beq	3002898 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x28>
 3002890:	e3a00000 	mov	r0, #0
 3002894:	ea000012 	b	30028e4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x74>
 3002898:	e1a02000 	mov	r2, r0
 300289c:	e3a00eae 	mov	r0, #2784	@ 0xae0
 30028a0:	e3a03004 	mov	r3, #4
 30028a4:	e3800a05 	orr	r0, r0, #20480	@ 0x5000
 30028a8:	e7d75000 	ldrb	r5, [r7, r0]
 30028ac:	e3a00000 	mov	r0, #0
 30028b0:	e3550000 	cmp	r5, #0
 30028b4:	1a00000a 	bne	30028e4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x74>
 30028b8:	e3a050ff 	mov	r5, #255	@ 0xff
 30028bc:	e3a09eff 	mov	r9, #4080	@ 0xff0
 30028c0:	e3855cff 	orr	r5, r5, #65280	@ 0xff00
 30028c4:	e3899a1f 	orr	r9, r9, #126976	@ 0x1f000
 30028c8:	e001a005 	and	sl, r1, r5
 30028cc:	e389300c 	orr	r3, r9, #12
 30028d0:	e28a8003 	add	r8, sl, #3
 30028d4:	e0084003 	and	r4, r8, r3
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
 300290c:	e1a0b001 	mov	fp, r1
 3002910:	e1a05002 	mov	r5, r2
 3002914:	eb000128 	bl	3002dbc <__ARMv5LongLdrPcThunk_(*github.com/drpaneas/mtile/runtime.Heap).prepareAllocation>
 3002918:	e31000ff 	tst	r0, #255	@ 0xff
 300291c:	0a000009 	beq	3002948 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xd8>
 3002920:	e1a03000 	mov	r3, r0
 3002924:	eaffffd9 	b	3002890 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x20>
 3002928:	e3a00fb7 	mov	r0, #732	@ 0x2dc
 300292c:	e3800b12 	orr	r0, r0, #18432	@ 0x4800
 3002930:	e19700b0 	ldrh	r0, [r7, r0]
 3002934:	e1500005 	cmp	r0, r5
 3002938:	0afffff2 	beq	3002908 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x98>
 300293c:	e3500b02 	cmp	r0, #2048	@ 0x800
 3002940:	3a000007 	bcc	3002964 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0xf4>
 3002944:	ea00000a 	b	3002974 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x104>
 3002948:	e3a00fb7 	mov	r0, #732	@ 0x2dc
 300294c:	e1a02005 	mov	r2, r5
 3002950:	e1a0100b 	mov	r1, fp
 3002954:	e3800b12 	orr	r0, r0, #18432	@ 0x4800
 3002958:	e19700b0 	ldrh	r0, [r7, r0]
 300295c:	e3500b02 	cmp	r0, #2048	@ 0x800
 3002960:	2a000003 	bcs	3002974 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x104>
 3002964:	e087b180 	add	fp, r7, r0, lsl #3
 3002968:	e5fb3adc 	ldrb	r3, [fp, #2780]!	@ 0xadc
 300296c:	e3130003 	tst	r3, #3
 3002970:	0a000002 	beq	3002980 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x110>
 3002974:	e3a03002 	mov	r3, #2
 3002978:	e7c73006 	strb	r3, [r7, r6]
 300297c:	eaffffc3 	b	3002890 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x20>
 3002980:	e59b3004 	ldr	r3, [fp, #4]
 3002984:	e5975ad8 	ldr	r5, [r7, #2776]	@ 0xad8
 3002988:	e3a06fb7 	mov	r6, #732	@ 0x2dc
 300298c:	e3540000 	cmp	r4, #0
 3002990:	e3866b12 	orr	r6, r6, #18432	@ 0x4800
 3002994:	e18730b6 	strh	r3, [r7, r6]
 3002998:	0a00001e 	beq	3002a18 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1a8>
 300299c:	e5973ad4 	ldr	r3, [r7, #2772]	@ 0xad4
 30029a0:	e59f616c 	ldr	r6, [pc, #364]	@ 3002b14 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x2a4>
 30029a4:	e59fc16c 	ldr	ip, [pc, #364]	@ 3002b18 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x2a8>
 30029a8:	e3530000 	cmp	r3, #0
 30029ac:	01a0c006 	moveq	ip, r6
 30029b0:	e3a06000 	mov	r6, #0
 30029b4:	e08c3005 	add	r3, ip, r5
 30029b8:	e3130003 	tst	r3, #3
 30029bc:	0a00000e 	beq	30029fc <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x18c>
 30029c0:	e1540006 	cmp	r4, r6
 30029c4:	9a000013 	bls	3002a18 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1a8>
 30029c8:	e0443006 	sub	r3, r4, r6
 30029cc:	e08c6006 	add	r6, ip, r6
 30029d0:	e1a09000 	mov	r9, r0
 30029d4:	e1a08002 	mov	r8, r2
 30029d8:	e0866005 	add	r6, r6, r5
 30029dc:	e1a00006 	mov	r0, r6
 30029e0:	e1a06001 	mov	r6, r1
 30029e4:	e1a01003 	mov	r1, r3
 30029e8:	eb0000f5 	bl	3002dc4 <__ARMv5LongLdrPcThunk___aeabi_memclr>
 30029ec:	e1a00009 	mov	r0, r9
 30029f0:	e1a01006 	mov	r1, r6
 30029f4:	e1a02008 	mov	r2, r8
 30029f8:	ea000006 	b	3002a18 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1a8>
 30029fc:	e3540011 	cmp	r4, #17
 3002a00:	2a000014 	bcs	3002a58 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1e8>
 3002a04:	e35a0005 	cmp	sl, #5
 3002a08:	e5836000 	str	r6, [r3]
 3002a0c:	25836004 	strcs	r6, [r3, #4]
 3002a10:	235a0009 	cmpcs	sl, #9
 3002a14:	2a000037 	bcs	3002af8 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x288>
 3002a18:	e3a060f2 	mov	r6, #242	@ 0xf2
 3002a1c:	e58b5004 	str	r5, [fp, #4]
 3002a20:	e1cb10b2 	strh	r1, [fp, #2]
 3002a24:	e3a01001 	mov	r1, #1
 3002a28:	e0853004 	add	r3, r5, r4
 3002a2c:	e3e05019 	mvn	r5, #25
 3002a30:	e3866c5a 	orr	r6, r6, #23040	@ 0x5a00
 3002a34:	e1811102 	orr	r1, r1, r2, lsl #2
 3002a38:	e2455a05 	sub	r5, r5, #20480	@ 0x5000
 3002a3c:	e1b760b6 	ldrh	r6, [r7, r6]!	@ <UNPREDICTABLE>
 3002a40:	e5cb1000 	strb	r1, [fp]
 3002a44:	e2861001 	add	r1, r6, #1
 3002a48:	e7873005 	str	r3, [r7, r5]
 3002a4c:	e3a03000 	mov	r3, #0
 3002a50:	e1c710b0 	strh	r1, [r7]
 3002a54:	eaffffa2 	b	30028e4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x74>
 3002a58:	e35a000d 	cmp	sl, #13
 3002a5c:	e58d2008 	str	r2, [sp, #8]
 3002a60:	3a00000f 	bcc	3002aa4 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x234>
 3002a64:	e24a600d 	sub	r6, sl, #13
 3002a68:	e58d0004 	str	r0, [sp, #4]
 3002a6c:	e1a00003 	mov	r0, r3
 3002a70:	e58d1000 	str	r1, [sp]
 3002a74:	e356000f 	cmp	r6, #15
 3002a78:	23a0600f 	movcs	r6, #15
 3002a7c:	e04a6006 	sub	r6, sl, r6
 3002a80:	e2866002 	add	r6, r6, #2
 3002a84:	e0066009 	and	r6, r6, r9
 3002a88:	e1a0900c 	mov	r9, ip
 3002a8c:	e2866010 	add	r6, r6, #16
 3002a90:	e1a01006 	mov	r1, r6
 3002a94:	eb0000ca 	bl	3002dc4 <__ARMv5LongLdrPcThunk___aeabi_memclr>
 3002a98:	e59d1000 	ldr	r1, [sp]
 3002a9c:	e99d0005 	ldmib	sp, {r0, r2}
 3002aa0:	e1a0c009 	mov	ip, r9
 3002aa4:	e0483006 	sub	r3, r8, r6
 3002aa8:	e3530004 	cmp	r3, #4
 3002aac:	3affffc3 	bcc	30029c0 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x150>
 3002ab0:	e08c3006 	add	r3, ip, r6
 3002ab4:	e58dc004 	str	ip, [sp, #4]
 3002ab8:	e046e00a 	sub	lr, r6, sl
 3002abc:	e1a0a000 	mov	sl, r0
 3002ac0:	e1a09001 	mov	r9, r1
 3002ac4:	e083c005 	add	ip, r3, r5
 3002ac8:	e3e03003 	mvn	r3, #3
 3002acc:	e1c3300e 	bic	r3, r3, lr
 3002ad0:	e1a0000c 	mov	r0, ip
 3002ad4:	e2838004 	add	r8, r3, #4
 3002ad8:	e1a01008 	mov	r1, r8
 3002adc:	eb0000b8 	bl	3002dc4 <__ARMv5LongLdrPcThunk___aeabi_memclr>
 3002ae0:	e59dc004 	ldr	ip, [sp, #4]
 3002ae4:	e59d2008 	ldr	r2, [sp, #8]
 3002ae8:	e1a0000a 	mov	r0, sl
 3002aec:	e1a01009 	mov	r1, r9
 3002af0:	e0886006 	add	r6, r8, r6
 3002af4:	eaffffb1 	b	30029c0 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x150>
 3002af8:	e3540010 	cmp	r4, #16
 3002afc:	e5836008 	str	r6, [r3, #8]
 3002b00:	1affffc4 	bne	3002a18 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x1a8>
 3002b04:	e583600c 	str	r6, [r3, #12]
 3002b08:	e3a06010 	mov	r6, #16
 3002b0c:	eaffffab 	b	30029c0 <(*github.com/drpaneas/mtile/runtime.Heap).allocObject+0x150>
 3002b10:	02001ba0 	.word	0x02001ba0
 3002b14:	02009600 	.word	0x02009600
 3002b18:	02017600 	.word	0x02017600
