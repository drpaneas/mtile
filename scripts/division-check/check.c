// Execute the actual ARM7TDMI helpers in mGBA and compare with host arithmetic.
// The frozen baseline is timed in the same IWRAM configuration as the candidate.
#include <inttypes.h>
#include <limits.h>
#include <mgba/core/core.h>
#include <mgba/gba/core.h>
#include <mgba/internal/arm/arm.h>
#include <mgba/internal/arm/isa-inlines.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

static struct mCore *core;
static struct ARMCore *cpu;
static uint32_t entries[2][4];
// Helpers enter in ARM mode and return to either ARM or Thumb callers.
static int run(int variant, int fn, uint64_t n, uint64_t d, uint64_t *q, uint64_t *r) {
    for (int i = 0; i < 13; i++)
        cpu->gprs[i] = 0x12345670 + i;
    cpu->gprs[0] = n;
    if (fn < 2)
        cpu->gprs[1] = d;
    else {
        cpu->gprs[1] = n >> 32;
        cpu->gprs[2] = d;
        cpu->gprs[3] = d >> 32;
    }
    int thumbReturn = n & 1;
    cpu->gprs[ARM_SP] = 0x03006000;
    cpu->gprs[ARM_LR] = 0x03007000 + thumbReturn;
    _ARMSetMode(cpu, MODE_ARM);
    cpu->gprs[ARM_PC] = entries[variant][fn];
    ARMWritePC(cpu);
    cpu->cycles = 0;
    cpu->nextEvent = INT_MAX;
    cpu->cpsr.n = (n >> 2) & 1;
    cpu->cpsr.z = (n >> 3) & 1;
    cpu->cpsr.c = (n >> 4) & 1;
    cpu->cpsr.v = (n >> 5) & 1;
    int steps = 0;
    while (cpu->gprs[ARM_PC] != 0x03007000 + (thumbReturn ? 2 : 4) && ++steps < 4000) {
        uint32_t instruction = cpu->prefetch[0];
        if (variant && (instruction & 0x0f000000) == 0x0b000000 &&
            ARMTestCondition(cpu, instruction >> 28) && (cpu->gprs[ARM_SP] & 7)) {
            fprintf(stderr, "unaligned call frame\n");
            exit(2);
        }
        ARMRun(cpu);
    }
    if (cpu->executionMode != (thumbReturn ? MODE_THUMB : MODE_ARM)) {
        fprintf(stderr, "bad return mode\n");
        exit(2);
    }
    if (steps >= 4000) {
        fprintf(stderr, "loop pc=%x\n", cpu->gprs[ARM_PC]);
        exit(2);
    }
    for (int i = 4; i <= 11; i++)
        if (cpu->gprs[i] != 0x12345670 + i) {
            fprintf(stderr, "clobber r%d\n", i);
            exit(2);
        }
    if (cpu->gprs[ARM_SP] != 0x03006000) {
        fprintf(stderr, "sp imbalance\n");
        exit(2);
    }
    *q = (uint32_t)cpu->gprs[0];
    *r = (uint32_t)cpu->gprs[1];
    if (fn >= 2) {
        *q |= (uint64_t)(uint32_t)cpu->gprs[1] << 32;
        *r = (uint32_t)cpu->gprs[2] | ((uint64_t)(uint32_t)cpu->gprs[3] << 32);
    }
    return cpu->cycles;
}

struct Stats {
    unsigned long count, regress;
    uint64_t old, now;
    int maxold, maxnow, maxreg;
};
static struct Stats stats[4][5];
static uint64_t calls;
static void check(int fn, int group, uint64_t n, uint64_t d) {
    if (fn < 2) {
        n = (uint32_t)n;
        d = (uint32_t)d;
    }
    uint64_t wq = 0, wr = 0;
    if (d) {
        if (fn == 0) {
            wq = n / d;
            wr = n % d;
        }
        if (fn == 1) {
            int64_t sn = (int32_t)n, sd = (int32_t)d;
            wq = (uint32_t)(sn / sd);
            wr = (uint32_t)(sn % sd);
        }
        if (fn == 2) {
            wq = n / d;
            wr = n % d;
        }
        if (fn == 3) {
            __int128 sn = (int64_t)n, sd = (int64_t)d;
            wq = sn / sd;
            wr = sn % sd;
        }
    }
    int cycles[2];
    for (int v = 0; v < 2; v++) {
        uint64_t q, r;
        cycles[v] = run(v, fn, n, d, &q, &r);
        if (q != wq || r != wr) {
            fprintf(stderr,
                    "variant%d fn%d n=%" PRIx64 " d=%" PRIx64 " got=%" PRIx64 ",%" PRIx64
                    " want=%" PRIx64 ",%" PRIx64 "\n",
                    v, fn, n, d, q, r, wq, wr);
            exit(1);
        }
    }
    struct Stats *s = &stats[fn][group];
    s->count++;
    s->old += cycles[0];
    s->now += cycles[1];
    if (cycles[0] > s->maxold)
        s->maxold = cycles[0];
    if (cycles[1] > s->maxnow)
        s->maxnow = cycles[1];
    if (cycles[1] > cycles[0]) {
        s->regress++;
        if (cycles[1] - cycles[0] > s->maxreg)
            s->maxreg = cycles[1] - cycles[0];
    }
    calls++;
}
static uint64_t seed = 0x298398923423ULL;
static uint64_t random64(void) {
    seed = seed * 6364136223846793005ULL + 1442695040888963407ULL;
    uint64_t x = seed;
    x ^= x >> 33;
    x *= 0xff51afd7ed558ccdULL;
    x ^= x >> 33;
    return x;
}
int main(int argc, char **argv) {
    if (argc != 12) {
        fprintf(stderr, "usage: checker old.bin oldAddr0 oldAddr1 oldAddr2 oldAddr3 new.bin "
                        "newAddr0 newAddr1 newAddr2 newAddr3 dummy.gba\n");
        return 3;
    }
    core = GBACoreCreate();
    if (!core || !core->init(core) || !mCoreLoadFile(core, argv[11]))
        return 3;
    cpu = core->cpu;
    cpu->cpsr.packed = 0x1f;
    ARMSetPrivilegeMode(cpu, MODE_SYSTEM);
    for (int v = 0; v < 2; v++) {
        FILE *f = fopen(argv[1 + v * 5], "rb");
        if (!f)
            return 2;
        unsigned char buf[8192];
        size_t sz = fread(buf, 1, sizeof(buf), f);
        fclose(f);
        for (size_t i = 0; i < sz; i++)
            core->busWrite8(core, 0x03001000 + v * 0x2000 + i, buf[i]);
        for (int fn = 0; fn < 4; fn++)
            entries[v][fn] = strtoul(argv[2 + v * 5 + fn], 0, 16);
    }
    // Cross every positive/negative power-of-two boundary with every other boundary.
    uint64_t ns[800];
    int nn = 0;
    ns[nn++] = 0;
    ns[nn++] = 1;
    ns[nn++] = 2;
    ns[nn++] = 3;
    ns[nn++] = UINT64_MAX;
    for (int bit = 1; bit < 64; bit++)
        for (int delta = -2; delta <= 2; delta++) {
            ns[nn++] = (1ULL << bit) + delta;
            ns[nn++] = 0 - ((1ULL << bit) + delta);
        }
    for (int fn = 0; fn < 4; fn++)
        for (int i = 0; i < nn; i++)
            for (int j = 0; j < nn; j++)
                check(fn, 0, ns[i], ns[j]);
    // Deterministic full-width, small-divisor, power-of-two, and fixed-point samples.
    for (int i = 0; i < 100000; i++) {
        uint64_t n = random64(), d = random64();
        for (int fn = 0; fn < 4; fn++)
            check(fn, 1, n, d);
        d = (random64() % 65535) + 1;
        for (int fn = 0; fn < 4; fn++)
            check(fn, 2, n, (fn & 1) && (i & 1) ? 0 - d : d);
        unsigned bit = random64() % 64;
        for (int fn = 0; fn < 4; fn++) {
            d = 1ULL << (bit % (fn < 2 ? 32 : 64));
            check(fn, 3, n, (fn & 1) && (i & 1) ? 0 - d : d);
        }
        int32_t raw = random64(), div = random64();
        uint64_t fixed = (uint64_t)(int64_t)raw << 16;
        check(2, 4, raw < 0 ? 0 - fixed : fixed, div < 0 ? -(uint64_t)div : div);
        check(3, 4, fixed, (int64_t)div);
    }
    const char *fns[] = {"u32", "s32", "u64", "s64"},
               *groups[] = {"edges", "random", "small-divisor", "powers-of-two", "fixed-point"};
    printf("Passed %" PRIu64 " cases (%" PRIu64 " ARM executions)\n", calls, calls * 2);
    for (int fn = 0; fn < 4; fn++)
        for (int group = 0; group < 5; group++) {
            struct Stats *s = &stats[fn][group];
            if (!s->count)
                continue;
            printf("%-3s %-14s n=%lu mean=%.2f->%.2f max=%d->%d regressions=%lu maxRegression=%d\n",
                   fns[fn], groups[group], s->count, (double)s->old / s->count,
                   (double)s->now / s->count, s->maxold, s->maxnow, s->regress, s->maxreg);
        }
    uint64_t ds[] = {60000, 65536, 32768, 3, 240};
    for (int fn = 0; fn < 4; fn++)
        for (int i = 0; i < 5; i++) {
            uint64_t q, r;
            int old = run(0, fn, 65536, ds[i], &q, &r), now = run(1, fn, 65536, ds[i], &q, &r);
            printf("%s(65536,%" PRIu64 ") cycles=%d->%d\n", fns[fn], ds[i], old, now);
        }
    unsigned long regressions = 0;
    for (int fn = 0; fn < 4; fn++)
        for (int group = 0; group < 5; group++)
            regressions += stats[fn][group].regress;
    return regressions ? 1 : 0;
}
