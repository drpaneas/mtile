"""Decode SMB3 level macro data offline through its bounded cartridge loader.

Only WRAM, internal RAM, four read-only PRG windows, and the 6502's registers
are provided. No frame loop, hardware I/O, rendering, sound, or game runtime.
"""
import hashlib

ROM_SHA256 = "4377a7f5e6eb50bdd2ac6f249bf1a7085500aca8eb41f38545c3a2731c51a579"
BANK_SIZE = 8192

# Documented 6502 address forms used by the original layout decompressor.
FORMS = {
    'ADC': 'IMM:69 ZP:65 ZPX:75 ABS:6D ABSX:7D ABSY:79 INDX:61 INDY:71',
    'AND': 'IMM:29 ZP:25 ZPX:35 ABS:2D ABSX:3D ABSY:39 INDX:21 INDY:31',
    'ASL': 'ACC:0A ZP:06 ZPX:16 ABS:0E ABSX:1E',
    'BCC': 'REL:90',
    'BCS': 'REL:B0',
    'BEQ': 'REL:F0',
    'BIT': 'ZP:24 ABS:2C',
    'BMI': 'REL:30',
    'BNE': 'REL:D0',
    'BPL': 'REL:10',
    'BRK': 'IMP:00',
    'BVC': 'REL:50',
    'BVS': 'REL:70',
    'CLC': 'IMP:18',
    'CLD': 'IMP:D8',
    'CLI': 'IMP:58',
    'CLV': 'IMP:B8',
    'CMP': 'IMM:C9 ZP:C5 ZPX:D5 ABS:CD ABSX:DD ABSY:D9 INDX:C1 INDY:D1',
    'CPX': 'IMM:E0 ZP:E4 ABS:EC',
    'CPY': 'IMM:C0 ZP:C4 ABS:CC',
    'DEC': 'ZP:C6 ZPX:D6 ABS:CE ABSX:DE',
    'DEX': 'IMP:CA',
    'DEY': 'IMP:88',
    'EOR': 'IMM:49 ZP:45 ZPX:55 ABS:4D ABSX:5D ABSY:59 INDX:41 INDY:51',
    'INC': 'ZP:E6 ZPX:F6 ABS:EE ABSX:FE',
    'INX': 'IMP:E8',
    'INY': 'IMP:C8',
    'JMP': 'ABS:4C IND:6C',
    'JSR': 'ABS:20',
    'LDA': 'IMM:A9 ZP:A5 ZPX:B5 ABS:AD ABSX:BD ABSY:B9 INDX:A1 INDY:B1',
    'LDX': 'IMM:A2 ZP:A6 ZPY:B6 ABS:AE ABSY:BE',
    'LDY': 'IMM:A0 ZP:A4 ZPX:B4 ABS:AC ABSX:BC',
    'LSR': 'ACC:4A ZP:46 ZPX:56 ABS:4E ABSX:5E',
    'NOP': 'IMP:EA',
    'ORA': 'IMM:09 ZP:05 ZPX:15 ABS:0D ABSX:1D ABSY:19 INDX:01 INDY:11',
    'PHA': 'IMP:48',
    'PHP': 'IMP:08',
    'PLA': 'IMP:68',
    'PLP': 'IMP:28',
    'ROL': 'ACC:2A ZP:26 ZPX:36 ABS:2E ABSX:3E',
    'ROR': 'ACC:6A ZP:66 ZPX:76 ABS:6E ABSX:7E',
    'RTI': 'IMP:40',
    'RTS': 'IMP:60',
    'SBC': 'IMM:E9 ZP:E5 ZPX:F5 ABS:ED ABSX:FD ABSY:F9 INDX:E1 INDY:F1',
    'SEC': 'IMP:38',
    'SED': 'IMP:F8',
    'SEI': 'IMP:78',
    'STA': 'ZP:85 ZPX:95 ABS:8D ABSX:9D ABSY:99 INDX:81 INDY:91',
    'STX': 'ZP:86 ZPY:96 ABS:8E',
    'STY': 'ZP:84 ZPX:94 ABS:8C',
    'TAX': 'IMP:AA',
    'TAY': 'IMP:A8',
    'TSX': 'IMP:BA',
    'TXA': 'IMP:8A',
    'TXS': 'IMP:9A',
    'TYA': 'IMP:98',
}
OPCODES = {int(code, 16): (op, mode) for op, forms in FORMS.items()
           for mode, code in (form.split(':') for form in forms.split())}
CARRY, ZERO, OVERFLOW, NEGATIVE = 1, 2, 64, 128
STOP_PC = 0x0200
# Random_Pool at first entry to the Plains layout reader in the reference replay.
REFERENCE_CLOUD_SEED = bytes.fromhex("8a2337711ffdc239bd")

# (terrain bank, layout pointer, object pointer, tileset, spawn x)
COURSES = {
    'world1-1': (15, 0xbb82, 0xc527, 1, 24),
    'world1-2': (16, 0xaf2a, 0xc6aa, 3, 24),
    'world1-1-bonus': (15, 0xbc93, 0xc006, 1, 0),
    'world1-2-bonus': (15, 0xba49, 0xc006, 1, 0),
}


def check_rom(rom):
    if hashlib.sha256(rom).hexdigest() != ROM_SHA256:
        raise ValueError('mario3.nes does not match the analyzed US ROM revision')


def prg_bank(rom, index):
    start = 16 + index * BANK_SIZE
    return rom[start:start + BANK_SIZE]


class LayoutReader:
    """Instruction subset for the ROM's pure terrain-construction routine."""

    def __init__(self, rom, terrain_bank):
        self.mem = bytearray(0x10000)
        for window, bank in enumerate((30, terrain_bank, 14, 31)):
            start = 0x8000 + window * BANK_SIZE
            self.mem[start:start + BANK_SIZE] = prg_bank(rom, bank)
        self.pc = 0xa408  # Entry to Plains/Hills layout initializers in these banks.
        self.a = self.x = self.y = 0
        self.status = 0x24
        self.sp = 0xff
        self.push((STOP_PC - 1) >> 8)
        self.push((STOP_PC - 1) & 255)
        self.seen = set()

    def fetch(self):
        value = self.mem[self.pc]
        self.pc = (self.pc + 1) & 65535
        return value

    def word(self, address, high_address=None):
        if high_address is None:
            high_address = (address + 1) & 65535
        return self.mem[address] | self.mem[high_address] << 8

    def push(self, value):
        self.mem[0x100 + self.sp] = value
        self.sp = (self.sp - 1) & 255

    def pop(self):
        self.sp = (self.sp + 1) & 255
        return self.mem[0x100 + self.sp]

    def flag(self, mask, on):
        self.status = (self.status | mask) if on else (self.status & ~mask)

    def nz(self, value):
        value &= 255
        self.flag(ZERO, value == 0)
        self.flag(NEGATIVE, value & 128)
        return value

    def adc(self, value):
        result = self.a + value + (self.status & CARRY)
        self.flag(CARRY, result > 255)
        self.flag(OVERFLOW, (~(self.a ^ value) & (self.a ^ result)) & 128)
        self.a = self.nz(result)

    def write(self, address, value):
        if not (0 <= address < 0x0800 or 0x6000 <= address < 0x8000):
            raise ValueError(f'Layout routine accessed a write outside RAM at ${address:04x}')
        self.mem[address] = value

    def operand(self, mode):
        if mode in ('IMP', 'ACC'):
            return 0
        low = self.fetch()
        if mode in ('IMM', 'REL', 'ZP'):
            return low
        if mode == 'ZPX':
            return (low + self.x) & 255
        if mode == 'ZPY':
            return (low + self.y) & 255
        if mode == 'INDX':
            address = (low + self.x) & 255
            return self.word(address, (address + 1) & 255)
        if mode == 'INDY':
            return (self.word(low, (low + 1) & 255) + self.y) & 65535
        address = low | self.fetch() << 8
        if mode == 'ABSX':
            return (address + self.x) & 65535
        if mode == 'ABSY':
            return (address + self.y) & 65535
        if mode == 'IND':
            return self.word(address, (address & 0xff00) | ((address + 1) & 255))
        if mode != 'ABS':
            raise ValueError(f'Unknown addressing mode {mode}')
        return address

    def step(self):
        pc = self.pc
        opcode = self.fetch()
        if opcode not in OPCODES:
            raise ValueError(f'Unsupported opcode ${opcode:02x} at ${pc:04x}')
        op, mode = OPCODES[opcode]
        self.seen.add(opcode)
        address = self.operand(mode)
        value = address if mode == 'IMM' else self.mem[address]
        if op in ('LDA', 'LDX', 'LDY'):
            setattr(self, op[-1].lower(), self.nz(value))
        elif op in ('STA', 'STX', 'STY'):
            self.write(address, getattr(self, op[-1].lower()))
        elif op in ('ADC', 'SBC'):
            self.adc(value if op == 'ADC' else value ^ 255)
        elif op in ('AND', 'ORA', 'EOR'):
            if op == 'AND':
                self.a = self.nz(self.a & value)
            elif op == 'ORA':
                self.a = self.nz(self.a | value)
            else:
                self.a = self.nz(self.a ^ value)
        elif op in ('CMP', 'CPX', 'CPY'):
            register = self.a if op == 'CMP' else getattr(self, op[-1].lower())
            self.flag(CARRY, register >= value)
            self.nz(register - value)
        elif op == 'BIT':
            self.flag(ZERO, (self.a & value) == 0)
            self.flag(OVERFLOW, value & OVERFLOW)
            self.flag(NEGATIVE, value & NEGATIVE)
        elif op in ('INC', 'DEC', 'ASL', 'LSR', 'ROL', 'ROR'):
            if mode == 'ACC':
                value = self.a
            carry = self.status & CARRY
            if op == 'INC':
                value += 1
            elif op == 'DEC':
                value -= 1
            elif op in ('ASL', 'ROL'):
                self.flag(CARRY, value & 128)
                value = (value << 1) | (carry if op == 'ROL' else 0)
            else:
                self.flag(CARRY, value & 1)
                value = (value >> 1) | ((carry << 7) if op == 'ROR' else 0)
            if mode == 'ACC':
                self.a = self.nz(value)
            else:
                self.write(address, self.nz(value))
        elif op in ('INX', 'INY', 'DEX', 'DEY'):
            register = op[-1].lower()
            setattr(self, register, self.nz(getattr(self, register) + (1 if op[0] == 'I' else -1)))
        elif op in ('TAX', 'TAY', 'TXA', 'TYA', 'TSX', 'TXS'):
            src, dst = op[1:].lower()
            value = getattr(self, 'sp' if src == 's' else src)
            setattr(self, 'sp' if dst == 's' else dst, value if dst == 's' else self.nz(value))
        elif op == 'PHA':
            self.push(self.a)
        elif op == 'PLA':
            self.a = self.nz(self.pop())
        elif op == 'PHP':
            self.push(self.status | 0x30)
        elif op == 'PLP':
            self.status = self.pop()
        elif op in ('CLC', 'SEC', 'CLV', 'CLD', 'CLI', 'SED', 'SEI'):
            self.flag({'C': CARRY, 'V': OVERFLOW, 'D': 8, 'I': 4}[op[2]], op[0] == 'S')
        elif op == 'JSR':
            return_address = (self.pc - 1) & 65535
            self.push(return_address >> 8)
            self.push(return_address & 255)
            self.pc = address
        elif op == 'RTS':
            low = self.pop()
            self.pc = ((low | self.pop() << 8) + 1) & 65535
        elif op == 'JMP':
            self.pc = address
        elif mode == 'REL':
            mask, expected = {'BCC': (CARRY, 0), 'BCS': (CARRY, CARRY),
                              'BEQ': (ZERO, ZERO), 'BNE': (ZERO, 0),
                              'BMI': (NEGATIVE, NEGATIVE), 'BPL': (NEGATIVE, 0),
                              'BVC': (OVERFLOW, 0), 'BVS': (OVERFLOW, OVERFLOW)}[op]
            if self.status & mask == expected:
                self.pc = (self.pc + address - (256 if address & 128 else 0)) & 65535
        elif op != 'NOP':
            raise ValueError(f'Unsupported layout instruction {op} at ${pc:04x}')

    def run(self):
        for _ in range(500_000):
            if self.pc == STOP_PC:
                return
            if self.pc < 0x8000:
                raise ValueError(f'Layout routine jumped out of PRG to ${self.pc:04x}')
            self.step()
        raise ValueError('Layout instruction budget exhausted')


def decode_layout(rom, name):
    terrain_bank, layout, object_pointer, tileset, spawn_x = COURSES[name]
    cpu = LayoutReader(rom, terrain_bank)
    cpu.mem[0x61:0x63] = layout.to_bytes(2, 'little')  # Level_LayPtr_AddrL/H.
    cpu.mem[0x070a] = tileset
    cpu.mem[0x0781:0x078a] = REFERENCE_CLOUD_SEED
    cpu.run()
    work = cpu.mem[0x6000:0x8000]
    work[0x1e94:0x1e9c] = prg_bank(rom, terrain_bank)[0x400:0x408]
    obj_bank = prg_bank(rom, 6)
    start = object_pointer - 0xc000
    work[0x1b40:0x1bd0] = obj_bank[start:start + 0x90]
    work[0x1eb9:0x1ebd] = layout.to_bytes(2, 'little') + object_pointer.to_bytes(2, 'little')
    cpu.mem[0x0090] = spawn_x
    cpu.mem[0x0728] = 3
    return cpu.mem[:0x800], work
