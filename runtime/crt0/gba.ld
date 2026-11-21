OUTPUT_ARCH(arm)
ENTRY(_start)

/* Note: iwram is reduced by 96 bytes because the last part of that RAM
 * (starting at 0x03007FA0) is used for interrupt handling.
 */
MEMORY {
    ewram   : ORIGIN = 0x02000000, LENGTH = 256K-512 /* application EWRAM */
    debug   : ORIGIN = 0x0203FE00, LENGTH = 512      /* preview debug mailbox */
    iwram   : ORIGIN = 0x03000000, LENGTH = 32K-96 /* in-chip work RAM (faster) */
    rom     : ORIGIN = 0x08000000, LENGTH = 32M    /* flash ROM */
}

__stack_size_irq = 1K;
/* Generated Lua controllers can have a >3 KiB native frame before their
 * callers, table helpers and GC. Keep the CPU stack separate from the Lua
 * heap and leave room for these nested calls (Celeste target regression). */
__stack_size_usr = 8K;

SECTIONS
{
    .text :
    {
        KEEP (*(.init))
        *(.text)
        *(.text.*)
        . = ALIGN(4);
    } >rom

    .rodata :
    {
        . = ALIGN(4);
        *(.rodata)
        *(.rodata.*)
        . = ALIGN(4);
    } >rom

    /* Put the stack at the bottom of RAM, so that the application will
     * crash on stack overflow instead of silently corrupting memory.
     * See: http://blog.japaric.io/stack-overflow-protection/ */
    .stack (NOLOAD) :
    {
        . = ALIGN(4);
        /* ARM full-descending stacks grow toward lower addresses, so each
         * stack pointer must start at the end of its reserved range. */
        . += __stack_size_irq;
        _stack_top_irq = .;
        . += __stack_size_usr;
        _stack_top = .;
    } >iwram

    /* Start address (in flash) of .data, used by startup code. */
    _sidata = LOADADDR(.data);

    /* Globals with initial value */
    .data :
    {
        . = ALIGN(4);
        _sdata = .;        /* used by startup code */
        *(.data)
        *(.data.*)
        *(.iwram .iwram.*)
        . = ALIGN(4);
        _edata = .;        /* used by startup code */
    } >iwram AT>rom

    /* EWRAM BSS must be collected before IWRAM .bss. TinyGo may emit
     * .bss.ewram*; IWRAM *( .bss.* ) would steal those input sections.
     *
     * Semispaces use .ewram_heap (underscore, not .ewram.*) so they sit
     * after ctx and other hot objects. A leading 112 KB heap would slide
     * ctx to 0x0201C080, which ARM7TDMI cannot encode as one rotated
     * immediate. */
    .ewram (NOLOAD) :
    {
        . = ALIGN(4);
        _ewram_start = .;
        KEEP(*(.ewram_begin .bss.ewram_begin))
        *(.ewram .ewram.*)
        *(.bss.ewram .bss.ewram.*)
        *(.ewram_heap .ewram_heap.*)
        *(.bss.ewram_heap .bss.ewram_heap.*)
        KEEP(*(.ewram_end .bss.ewram_end))
        . = ALIGN(4);
        _ewram_end = .;
    } >ewram
    _heap_start = .;
    _heap_end = ORIGIN(ewram) + LENGTH(ewram);
    ASSERT(_heap_end - _ewram_end >= 4096, "EWRAM leftover below ewramReserve (4 KiB)")

    .debug_mailbox (NOLOAD) :
    {
        . = ALIGN(4);
        KEEP(*(.debug_mailbox .debug_mailbox.*))
        . = ALIGN(4);
    } >debug
    ASSERT(SIZEOF(.debug_mailbox) <= LENGTH(debug), "debug mailbox exceeds 512 bytes")

    /* Zero-initialized globals  */
    .bss :
    {
        . = ALIGN(4);
        _sbss = .;         /* used by startup code */
        *(.bss)
        /* ThinLTO can emit Maxmod globals in different orders. Sort their
         * named sections so the addresses embedded in the ROM are stable. */
        *(SORT_BY_NAME(.bss.mm*))
        *(SORT_BY_NAME(.bss.mpp_*))
        *(.bss.*)
        *(COMMON)
        . = ALIGN(4);
        _ebss = .;         /* used by startup code */
    } >iwram

    /DISCARD/ :
    {
        *(.ARM.exidx)      /* causes 'no memory region specified' error in lld */
        *(.ARM.exidx.*)    /* causes spurious 'undefined reference' errors */
    }
}

_globals_start = _sdata;
_globals_end = _ebss;
