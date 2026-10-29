package runtime

import "github.com/drpaneas/mtile/runtime/hw"

const (
	SaveFieldCount = 64
	SaveSlotCount  = 32
	SaveOK         = 1
	SaveEmpty      = 0
	SaveIOError    = -1
	SaveCorrupt    = -2
	SaveWrongKey   = -3
	SaveBadKey     = -4
	SaveBadSlot    = -5

	saveRecordBytes  = 304
	saveRecordStride = 512
	saveSlotStride   = 2 * saveRecordStride
	saveCommit       = 0xa5
)

// SaveMemory is a game's staging buffer, linked only by games using saving.
// Fields retain the exact signed 16:16 representation; there is no heap or
// automatic serialization of Lua tables, globals, graphics, or audio state.
type SaveMemory struct {
	fields [SaveFieldCount]Number
	// Fixed scratch storage keeps record arrays off TinyGo's allocation path
	// and out of the small native stack, including in -gc=none cartridges.
	records [2]saveRecord
}

func (c *Context) AttachSaveMemory(memory *SaveMemory) { c.saveMemory = memory }

func saveFieldIndex(v Value) (int, bool) {
	return v.Int(), v.Kind == KindNumber && v.Num&0xffff == 0 && v.Num >= 0 && v.Num < FromInt(SaveFieldCount)
}

func (c *Context) SaveGetV(index Value) Value {
	i, ok := saveFieldIndex(index)
	if !ok || c.saveMemory == nil {
		return IntVal(0)
	}
	return NumVal(c.saveMemory.fields[i])
}

func (c *Context) SaveSetV(index, value Value) Value {
	i, ok := saveFieldIndex(index)
	if !ok || c.saveMemory == nil || value.Kind != KindNumber {
		return BoolVal(false)
	}
	c.saveMemory.fields[i] = value.Num
	return BoolVal(true)
}

type saveRecord [saveRecordBytes]byte

func saveReadRecord(slot int, record *saveRecord) {
	for i := range record {
		record[i] = hw.ReadSRAMByte(slot*saveRecordStride + i)
	}
}

func saveWord(record *saveRecord, offset int) uint32 {
	return uint32(record[offset]) | uint32(record[offset+1])<<8 | uint32(record[offset+2])<<16 | uint32(record[offset+3])<<24
}

func savePutWord(record *saveRecord, offset int, value uint32) {
	for i := 0; i < 4; i++ {
		record[offset+i] = byte(value >> uint(i*8))
	}
}

// CRC covers the format, full key/padding, generation, and payload. The
// one-byte commit marker and the CRC field itself are deliberately excluded.
// Four-bit IEEE steps keep synchronous save work below the frame budget with
// the shared menu loop, using just 64 bytes of ROM rather than a 1 KiB table.
//
//go:section .rodata
var saveCRCNibbles = [16]uint32{
	0x00000000, 0x1db71064, 0x3b6e20c8, 0x26d930ac,
	0x76dc4190, 0x6b6b51f4, 0x4db26158, 0x5005713c,
	0xedb88320, 0xf00f9344, 0xd6d6a3e8, 0xcb61b38c,
	0x9b64c2b0, 0x86d3d2d4, 0xa00ae278, 0xbdbdf21c,
}

func saveCRC(record *saveRecord) uint32 {
	crc := ^uint32(0)
	for i := 1; i < len(record); i++ {
		if i >= 44 && i < 48 {
			continue
		}
		crc ^= uint32(record[i])
		crc = crc>>4 ^ saveCRCNibbles[crc&15]
		crc = crc>>4 ^ saveCRCNibbles[crc&15]
	}
	return ^crc
}

func saveValid(record *saveRecord) bool {
	return record[0] == saveCommit && record[1] == 'M' && record[2] == 'T' && record[3] == 'S' && record[4] == 'V' &&
		record[5] == 1 && record[6] >= 1 && record[6] <= 32 && record[7] == 0 && saveWord(record, 44) == saveCRC(record)
}

func saveBlank(record *saveRecord) bool {
	fill := record[0]
	if fill != 0 && fill != 0xff {
		return false
	}
	for _, b := range record {
		if b != fill {
			return false
		}
	}
	return true
}

func saveNewest(a, b *saveRecord) int {
	av, bv := saveValid(a), saveValid(b)
	if bv && (!av || int32(saveWord(b, 8)-saveWord(a, 8)) > 0) {
		return 1
	}
	if av {
		return 0
	}
	return -1
}

// SaveRead and SaveWrite retain the original slot-zero storage layout.
func (c *Context) SaveRead(key string) int  { return c.SaveReadSlot(key, 0) }
func (c *Context) SaveWrite(key string) int { return c.SaveWriteSlot(key, 0) }

func saveSlotIndex(slot Value) (int, bool) {
	return slot.Int(), slot.Kind == KindNumber && slot.Num&0xffff == 0 && slot.Num >= 0 && slot.Num < FromInt(SaveSlotCount)
}

func (c *Context) SaveReadV(key string, slot Value) Value {
	i, ok := saveSlotIndex(slot)
	if !ok {
		return IntVal(SaveBadSlot)
	}
	return IntVal(c.SaveReadSlot(key, i))
}

func (c *Context) SaveWriteV(key string, slot Value) Value {
	i, ok := saveSlotIndex(slot)
	if !ok {
		return IntVal(SaveBadSlot)
	}
	return IntVal(c.SaveWriteSlot(key, i))
}

// SavePending reports a five-second power-off reminder, not the cartridge's
// SD flush status. The generated loop advances it even while paused.
func (c *Context) SavePending() bool { return c.saveWait > 0 }

// SaveReadSlot selects the newest complete recovery copy in one independent
// slot before checking its key. Every failure leaves staging and SRAM intact.
func (c *Context) SaveReadSlot(key string, slot int) int {
	if len(key) < 1 || len(key) > 32 {
		return SaveBadKey
	}
	if slot < 0 || slot >= SaveSlotCount {
		return SaveBadSlot
	}
	if c.saveMemory == nil {
		return SaveIOError
	}
	hw.PrepareSRAM()
	a, b := &c.saveMemory.records[0], &c.saveMemory.records[1]
	saveReadRecord(slot*2, a)
	saveReadRecord(slot*2+1, b)
	n := saveNewest(a, b)
	if n < 0 {
		if saveBlank(a) && saveBlank(b) {
			return SaveEmpty
		}
		return SaveCorrupt
	}
	record := a
	if n == 1 {
		record = b
	}
	if int(record[6]) != len(key) {
		return SaveWrongKey
	}
	for i := 0; i < len(key); i++ {
		if record[12+i] != key[i] {
			return SaveWrongKey
		}
	}
	for i := range c.saveMemory.fields {
		c.saveMemory.fields[i] = Number(saveWord(record, 48+i*4))
	}
	return SaveOK
}

// SaveWrite alternates records, invalidates and verifies the target marker,
// writes and verifies the body, then commits with a single byte. No write
// touches the newest valid record. Success verifies cartridge-visible bytes;
// flashcart firmware may still have its own later SD-card flush procedure.
func (c *Context) SaveWriteSlot(key string, slot int) int {
	if len(key) < 1 || len(key) > 32 {
		return SaveBadKey
	}
	if slot < 0 || slot >= SaveSlotCount {
		return SaveBadSlot
	}
	if c.saveMemory == nil {
		return SaveIOError
	}
	hw.PrepareSRAM()
	a, b := &c.saveMemory.records[0], &c.saveMemory.records[1]
	saveReadRecord(slot*2, a)
	saveReadRecord(slot*2+1, b)
	n := saveNewest(a, b)
	target, generation := 0, uint32(1)
	if n >= 0 {
		target = 1 - n
		previous := a
		if n == 1 {
			previous = b
		}
		generation = saveWord(previous, 8) + 1
	}
	record := a
	if target == 1 {
		record = b
	}
	for i := range record {
		record[i] = 0
	}
	record[1], record[2], record[3], record[4] = 'M', 'T', 'S', 'V'
	record[5], record[6] = 1, byte(len(key))
	savePutWord(record, 8, generation)
	for i := 0; i < len(key); i++ {
		record[12+i] = key[i]
	}
	for i, value := range c.saveMemory.fields {
		savePutWord(record, 48+i*4, uint32(value))
	}
	savePutWord(record, 44, saveCRC(record))
	base := slot*saveSlotStride + target*saveRecordStride
	hw.WriteSRAMByte(base, 0)
	if hw.ReadSRAMByte(base) != 0 {
		return SaveIOError
	}
	for i := 1; i < len(record); i++ {
		hw.WriteSRAMByte(base+i, record[i])
	}
	for i := 1; i < len(record); i++ {
		if hw.ReadSRAMByte(base+i) != record[i] {
			return SaveIOError
		}
	}
	hw.WriteSRAMByte(base, saveCommit)
	if hw.ReadSRAMByte(base) != saveCommit {
		return SaveIOError
	}
	c.saveWait = 150
	return SaveOK
}
