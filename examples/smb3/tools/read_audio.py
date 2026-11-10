"""Run the ROM's isolated per-vblank sound-data driver at build time."""
from rom_layout import LayoutReader, prg_bank

DRIVER = 0xa000
QUEUE_PLAYER = 0x04f1
QUEUE_LEVEL = 0x04f2
QUEUE_NOISE = 0x04f3
QUEUE_MUSIC1 = 0x04f4
QUEUE_MUSIC2 = 0x04f5
QUEUE_MAP = 0x04f6


class SoundReader(LayoutReader):
    def __init__(self, rom):
        super().__init__(rom, 28)
        self.mem[0xc000:0xe000] = prg_bank(rom, 29)
        self.registers = []
        self.sp = 0xff

    def write(self, address, value):
        if 0x4000 <= address <= 0x4017:
            self.registers.append((address, value))
            self.mem[address] = value
        else:
            super().write(address, value)

    def tick(self):
        self.pc = DRIVER
        self.push(1)
        self.push(255)
        self.registers = []
        self.run()
        if self.sp != 0xff:
            raise ValueError('Unbalanced sound driver stack')
        return self.registers


def read_track(rom, address, value, frames):
    sound = SoundReader(rom)
    sound.mem[0x4015] = 15
    sound.mem[address] = value
    for _ in range(frames):
        writes = sound.tick()
        yield writes, sound.mem[0x04df], sound.mem[0x04d3:0x04df]
