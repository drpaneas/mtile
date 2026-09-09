// Execute the packed-span ARM helper with independent pixel and memory-access
// oracles.
#include <inttypes.h>
#include <limits.h>
#include <mgba/core/core.h>
#include <mgba/gba/core.h>
#include <mgba/internal/arm/arm.h>
#include <mgba/internal/arm/isa-inlines.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#define SRC 0x02010000u
#define DST 0x02000100u
#define SPANS 0x02020000u
#define FB 38400
struct Span {
  uint16_t x, y, len, off;
};
static struct mCore *core;
static struct ARMCore *cpu;
static struct ARMMemory orig;
static int checking;
static struct Span spans[32];
static int nspans, testcase;
static uint8_t source[32768], want[FB + 32];
static uint32_t state = 0x243fc68a;
static uint32_t rnd(void) { return state = 1664525 * state + 1013904223; }
static void checkread(uint32_t a, int n) {
  if (!checking)
    return;
  if (a >= 0x03006000 && a + n <= 0x03007014)
    return;
  if (a >= SPANS && a + n <= SPANS + nspans * 8)
    return;
  for (int i = 0; i < nspans; i++)
    if (!(spans[i].len & 0x8000) && a >= SRC + spans[i].off &&
        a + n <= SRC + spans[i].off + spans[i].len)
      return;
  fprintf(stderr, "invalid read case%d address%08x len%d pc%08x\n", testcase, a,
          n, cpu->gprs[ARM_PC]);
  exit(2);
}
static uint32_t load8(struct ARMCore *c, uint32_t a, int *t) {
  checkread(a, 1);
  return orig.load8(c, a, t);
}
static uint32_t load16(struct ARMCore *c, uint32_t a, int *t) {
  if (a & 1) {
    fprintf(stderr, "unaligned halfload\n");
    exit(2);
  }
  checkread(a, 2);
  return orig.load16(c, a, t);
}
static uint32_t load32(struct ARMCore *c, uint32_t a, int *t) {
  if (a & 3) {
    fprintf(stderr, "unaligned wordload\n");
    exit(2);
  }
  checkread(a, 4);
  return orig.load32(c, a, t);
}
static uint32_t loadmulti(struct ARMCore *c, uint32_t a, int m,
                          enum LSMDirection d, int *t) {
  if (d != LSM_IA || (a & 3)) {
    fprintf(stderr, "unexpected ldm\n");
    exit(2);
  }
  checkread(a, 4 * __builtin_popcount(m));
  return orig.loadMultiple(c, a, m, d, t);
}
static void store16(struct ARMCore *c, uint32_t a, int16_t v, int *t) {
  if (a & 1) {
    fprintf(stderr, "unaligned halfstore\n");
    exit(2);
  }
  orig.store16(c, a, v, t);
}
static void store32(struct ARMCore *c, uint32_t a, int32_t v, int *t) {
  if (a & 3) {
    fprintf(stderr, "unaligned wordstore\n");
    exit(2);
  }
  orig.store32(c, a, v, t);
}
static uint32_t storemulti(struct ARMCore *c, uint32_t a, int m,
                           enum LSMDirection d, int *t) {
  if (a & 3) {
    fprintf(stderr, "unaligned stm\n");
    exit(2);
  }
  return orig.storeMultiple(c, a, m, d, t);
}
static int run(int origin, int x0, int x1, int y0, int y1) {
  for (int i = 0; i < 13; i++)
    cpu->gprs[i] = 0x12345670 + i;
  cpu->gprs[0] = SRC;
  cpu->gprs[1] = DST + 16;
  cpu->gprs[2] = SPANS;
  cpu->gprs[3] = nspans;
  cpu->gprs[ARM_SP] = 0x03007000;
  int thumb = testcase & 1;
  cpu->gprs[ARM_LR] = 0x03007800 + thumb;
  int args[] = {origin, x0, x1, y0, y1};
  for (int i = 0; i < 5; i++)
    core->busWrite32(core, 0x03007000 + i * 4, args[i]);
  _ARMSetMode(cpu, MODE_ARM);
  cpu->gprs[ARM_PC] = 0x03001000;
  ARMWritePC(cpu);
  cpu->cycles = 0;
  cpu->nextEvent = INT_MAX;
  int steps = 0;
  while (cpu->gprs[ARM_PC] != 0x03007800 + (thumb ? 2 : 4) && ++steps < 100000)
    ARMRun(cpu);
  if (steps == 100000) {
    fprintf(stderr, "loop\n");
    exit(2);
  }
  for (int i = 4; i < 12; i++)
    if (cpu->gprs[i] != 0x12345670 + i) {
      fprintf(stderr, "clobber r%d\n", i);
      exit(2);
    }
  if (cpu->gprs[ARM_SP] != 0x03007000 ||
      cpu->executionMode != (thumb ? MODE_THUMB : MODE_ARM)) {
    fprintf(stderr, "ABI failure\n");
    exit(2);
  }
  return cpu->cycles;
}
static int one(int origin, int x0, int x1, int y0, int y1) {
  for (int i = 0; i < (int)sizeof(source); i++)
    core->busWrite8(core, SRC + i, source[i]);
  for (int i = 0; i < nspans * 8; i++)
    core->busWrite8(core, SPANS + i, ((uint8_t *)spans)[i]);
  for (int i = 0; i < (int)sizeof(want); i++) {
    want[i] = i * 73 + testcase;
    core->busWrite8(core, DST + i, want[i]);
  }
  for (int i = 0; i < nspans; i++) {
    struct Span s = spans[i];
    if (s.y < y0)
      continue;
    if (s.y >= y1)
      break;
    int len = s.len & 0x7fff;
    int left = s.x > x0 ? s.x : x0, right = s.x + len < x1 ? s.x + len : x1;
    for (int x = left; x < right; x++) {
      int d = s.y * 240 + x + origin;
      if (d < 0 || d >= FB) {
        fprintf(stderr, "bad oracle dst\n");
        exit(2);
      }
      want[16 + d] = (s.len & 0x8000) ? s.off : source[s.off + x - s.x];
    }
  }
  checking = 1;
  int cy = run(origin, x0, x1, y0, y1);
  checking = 0;
  for (int i = 0; i < (int)sizeof(want); i++)
    if (core->busRead8(core, DST + i) != want[i]) {
      fprintf(stderr, "mismatch case%d offset%d got%u want%u\n", testcase, i,
              core->busRead8(core, DST + i), want[i]);
      exit(1);
    }
  for (int i = 0; i < (int)sizeof(source); i++)
    if (core->busRead8(core, SRC + i) != source[i]) {
      fprintf(stderr, "source changed\n");
      exit(1);
    }
  for (int i = 0; i < nspans * 8; i++)
    if (core->busRead8(core, SPANS + i) != ((uint8_t *)spans)[i]) {
      fprintf(stderr, "span changed\n");
      exit(1);
    }
  testcase++;
  return cy;
}
int main(int argc, char **argv) {
  if (argc != 3) {
    fprintf(stderr, "usage: map-packed-check helper.bin dummy.gba\n");
    return 2;
  }
  core = GBACoreCreate();
  core->init(core);
  if (!mCoreLoadFile(core, argv[2]))
    return 2;
  cpu = core->cpu;
  cpu->cpsr.packed = 0x1f;
  ARMSetPrivilegeMode(cpu, MODE_SYSTEM);
  FILE *f = fopen(argv[1], "rb");
  unsigned char blob[1024];
  size_t sz = fread(blob, 1, sizeof(blob), f);
  fclose(f);
  for (size_t i = 0; i < sz; i++)
    core->busWrite8(core, 0x03001000 + i, blob[i]);
  orig = cpu->memory;
  cpu->memory.load8 = load8;
  cpu->memory.load16 = load16;
  cpu->memory.load32 = load32;
  cpu->memory.loadMultiple = loadmulti;
  cpu->memory.store16 = store16;
  cpu->memory.store32 = store32;
  cpu->memory.storeMultiple = storemulti;
  for (int i = 0; i < (int)sizeof(source); i++)
    source[i] = 1 + (rnd() >> 24) % 255;
  uint64_t cy = 0;
  for (int uniform = 0; uniform < 2; uniform++)
    for (int len = 0; len <= 260; len++)
      for (int a = 0; a < 4; a++)
        for (int clip = 0; clip < 8; clip++) {
          int x = 1024 + ((len + a) & 3), y = 60000;
          spans[0] = (struct Span){x, y, len | (uniform ? 0x8000 : 0),
                                   uniform ? len % 256 : 12};
          nspans = 1;
          int left = clip < 4 ? clip : 0,
              right = clip >= 4 ? len - (clip - 4) : len;
          cy += one(10 * 240 + 20 + a - y * 240 - x, x + left, x + right, y,
                    y + 1);
        }
  for (int k = 0; k < 2048; k++) {
    int baseX = 500 + (rnd() % 60000), baseY = 500 + (rnd() % 60000), pos = 0;
    nspans = rnd() % 32;
    for (int i = 0; i < nspans; i++) {
      int len = rnd() % 180, x = baseX + rnd() % 25;
      pos = (pos + 3) & ~3;
      int uniform = (i + k) % 3 != 0;
      spans[i] = (struct Span){x, baseY + i / 3, len | (uniform ? 0x8000 : 0),
                               uniform ? (rnd() >> 24) : pos};
      pos += len;
    }
    int x0 = baseX + (int)(rnd() % 30) - 4, x1 = baseX + 120 + rnd() % 80;
    int y0 = baseY + (int)(rnd() % 5) - 2, y1 = baseY + 5 + rnd() % 8;
    cy += one(20 * 240 + 10 + (k & 3) - baseY * 240 - baseX, x0, x1, y0, y1);
  }
  nspans = 1;
  for (int uniform = 0; uniform < 2; uniform++) {
    spans[0] =
        (struct Span){0, 0, 32767 | (uniform ? 0x8000 : 0), uniform ? 255 : 0};
    cy += one(0, 0, 32767, 0, 1);
  }
  printf("%d native cases PASS; copy/fill mixtures, all uniform colours, "
         "clipping, all alignments, lengths0..260/32767, random sorted "
         "batches, >255 coordinates, byte guards, source/span immutability, "
         "register/stack ABI, ARM/Thumb return; every load within source span "
         "or span record/stack. Mean cycles%.2f\n",
         testcase, (double)cy / testcase);
}
