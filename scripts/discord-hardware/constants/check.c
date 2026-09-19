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
static int run(uint32_t address, unsigned flags) {
  for (int i = 0; i < 13; i++)
    cpu->gprs[i] = 0x12345670 + i;
  cpu->gprs[ARM_SP] = 0x03006000;
  cpu->gprs[ARM_LR] = 0x03007000;
  _ARMSetMode(cpu, MODE_ARM);
  cpu->gprs[ARM_PC] = address;
  ARMWritePC(cpu);
  cpu->cycles = 0;
  cpu->nextEvent = INT_MAX;
  cpu->cpsr.n = (flags >> 3) & 1;
  cpu->cpsr.z = (flags >> 2) & 1;
  cpu->cpsr.c = (flags >> 1) & 1;
  cpu->cpsr.v = flags & 1;
  int steps = 0;
  while (cpu->gprs[ARM_PC] != 0x03007004 && ++steps < 10)
    ARMRun(cpu);
  if (steps >= 10 || cpu->gprs[0] != 0x040000de ||
      cpu->gprs[ARM_SP] != 0x03006000 || cpu->executionMode != MODE_ARM) {
    fprintf(stderr, "result/sp/state failure\n");
    exit(1);
  }
  for (int i = 1; i < 13; i++)
    if (cpu->gprs[i] != 0x12345670 + i) {
      fprintf(stderr, "register %d failure\n", i);
      exit(1);
    }
  if (cpu->cpsr.n != ((flags >> 3) & 1) || cpu->cpsr.z != ((flags >> 2) & 1) ||
      cpu->cpsr.c != ((flags >> 1) & 1) || cpu->cpsr.v != (flags & 1)) {
    fprintf(stderr, "flags failure\n");
    exit(1);
  }
  return cpu->cycles;
}
int main(int argc, char **argv) {
  if (argc != 2)
    return 2;
  core = GBACoreCreate();
  if (!core || !core->init(core) || !mCoreLoadFile(core, argv[1]))
    return 2;
  cpu = core->cpu;
  cpu->cpsr.packed = 0x1f;
  ARMSetPrivilegeMode(cpu, MODE_SYSTEM);
  for (int i = 0; i < 24; i++)
    core->busWrite8(core, 0x03001000 + i, core->busRead8(core, 0x08001000 + i));
  core->busWrite16(core, 0x04000204, 0x4317);
  const char *names[] = {"ROM", "IWRAM"};
  for (int location = 0; location < 2; location++) {
    uint32_t base = location ? 0x03001000 : 0x08001000;
    for (int variant = 0; variant < 2; variant++) {
      int min = INT_MAX, max = 0;
      uint64_t sum = 0;
      for (unsigned i = 0; i < 16000; i++) {
        int cycles = run(base + variant * 12, i & 15);
        if (cycles < min)
          min = cycles;
        if (cycles > max)
          max = cycles;
        sum += cycles;
      }
      printf("%s %s n=16000 min=%d max=%d mean=%.3f bytes=12\n",
             names[location], variant ? "two_immediates" : "literal_load", min,
             max, (double)sum / 16000);
    }
  }
  printf(
      "PASS: value, r1-r12, SP, CPSR flags, ARM return state; WAITCNT=%04x\n",
      core->busRead16(core, 0x04000204));
  return 0;
}
