package runtime

import (
	"hash/crc32"
	"testing"

	"github.com/drpaneas/mtile/runtime/hw"
)

func saveContext() *Context {
	c := new(Context)
	c.AttachSaveMemory(new(SaveMemory))
	return c
}

func TestSaveRoundTripAndColdBoot(t *testing.T) {
	for _, fill := range []byte{0, 0xff} {
		hw.ResetSRAMForTest(fill)
		c := saveContext()
		if got := c.SaveRead("test-v1"); got != SaveEmpty {
			t.Fatal(got)
		}
		for i := 0; i < SaveFieldCount; i++ {
			c.SaveSetV(IntVal(i), NumVal(Number(uint32(i)*0x891173ab)))
		}
		if got := c.SaveWrite("test-v1"); got != SaveOK {
			t.Fatal(got)
		}
		boot := saveContext() // No RAM from the writer survives.
		if got := boot.SaveRead("test-v1"); got != SaveOK {
			t.Fatal(got)
		}
		for i := 0; i < SaveFieldCount; i++ {
			if got := boot.SaveGetV(IntVal(i)); got != c.SaveGetV(IntVal(i)) {
				t.Fatalf("field %d: %+v", i, got)
			}
		}
		for i := 0; i < 1024; i++ {
			// Unused gaps and the rest of SRAM must remain untouched.
			if i >= saveRecordBytes && i < saveRecordStride || i >= saveRecordStride+saveRecordBytes {
				if hw.ReadSRAMByte(i) != fill {
					t.Fatalf("out-of-record write at %d", i)
				}
			}
		}
		if hw.ReadSRAMByte(32767) != fill {
			t.Fatal("last SRAM byte overwritten")
		}
	}
}

func TestSaveInterruptEveryByteStore(t *testing.T) {
	// Exercise first save, second save, and reuse of a previously committed
	// destination. A failed invalidation must not leave a mixed valid record.
	for _, previousWrites := range []int{0, 1, 2, 3} {
		for cut := 0; cut <= saveRecordBytes+1; cut++ {
			hw.ResetSRAMForTest(0xff)
			c := saveContext()
			for i := 1; i <= previousWrites; i++ {
				c.SaveSetV(IntVal(0), IntVal(i))
				if c.SaveWrite("power-v1") != SaveOK {
					t.Fatal("setup")
				}
			}
			c.SaveSetV(IntVal(0), IntVal(99))
			hw.SetSRAMWriteLimitForTest(cut)
			result := c.SaveWrite("power-v1")
			hw.SetSRAMWriteLimitForTest(-1)
			boot := saveContext()
			loaded := boot.SaveRead("power-v1")
			if loaded == SaveOK {
				value := boot.SaveGetV(IntVal(0))
				if value != IntVal(previousWrites) && value != IntVal(99) {
					t.Fatalf("cut %d: torn save: %+v", cut, value)
				}
				if previousWrites == 0 && value != IntVal(99) {
					t.Fatal("invented first save")
				}
				if result == SaveOK && value != IntVal(99) {
					t.Fatal("false success")
				}
			} else if previousWrites > 0 || result == SaveOK {
				t.Fatalf("prior=%d cut=%d write=%d load=%d", previousWrites, cut, result, loaded)
			}
			if c.SaveGetV(IntVal(0)) != IntVal(99) {
				t.Fatal("failed write changed RAM")
			}
		}
	}
}

func TestSaveValidationAndRecovery(t *testing.T) {
	hw.ResetSRAMForTest(0xff)
	c := saveContext()
	for _, v := range []Value{NilVal(), BoolVal(false), NumVal(One / 2), IntVal(-1), IntVal(64)} {
		if c.SaveSetV(v, IntVal(4)).Truthy() || c.SaveGetV(v) != IntVal(0) {
			t.Fatalf("accepted index %+v", v)
		}
	}
	if c.SaveSetV(IntVal(0), BoolVal(true)).Truthy() {
		t.Fatal("accepted non-number")
	}
	for _, key := range []string{"", "012345678901234567890123456789012"} {
		if c.SaveRead(key) != SaveBadKey || c.SaveWrite(key) != SaveBadKey {
			t.Fatal("accepted invalid key")
		}
	}
	const key = "save-\x00-é-v1"
	c.SaveSetV(IntVal(0), IntVal(7))
	if c.SaveWrite(key) != SaveOK {
		t.Fatal("first save")
	}
	c.SaveSetV(IntVal(0), IntVal(8))
	if c.SaveWrite(key) != SaveOK {
		t.Fatal("second save")
	}
	if c.SaveRead("different-v1") != SaveWrongKey || c.SaveGetV(IntVal(0)) != IntVal(8) {
		t.Fatal("wrong key altered staging buffer")
	}
	// Flip each byte in the newest record independently, including the key,
	// generation, CRC and final payload byte. The older record must load.
	var original saveRecord
	saveReadRecord(1, &original)
	for i := range original {
		hw.WriteSRAMByte(saveRecordStride+i, original[i]^1)
		if c.SaveRead(key) != SaveOK || c.SaveGetV(IntVal(0)) != IntVal(7) {
			t.Fatalf("corrupt byte %d was not recovered", i)
		}
		hw.WriteSRAMByte(saveRecordStride+i, original[i])
	}
	hw.WriteSRAMByte(0, 0)
	hw.WriteSRAMByte(saveRecordStride, 0)
	c.SaveSetV(IntVal(0), IntVal(42))
	if c.SaveRead(key) != SaveCorrupt || c.SaveGetV(IntVal(0)) != IntVal(42) {
		t.Fatal("corrupt load altered RAM")
	}
	// Intentional user saving can replace corrupt data or another key.
	if c.SaveWrite("replacement-v2") != SaveOK || c.SaveRead(key) != SaveWrongKey {
		t.Fatal("replacement key")
	}
}

func TestSaveGenerationWrapAndMissingMemory(t *testing.T) {
	hw.ResetSRAMForTest(0xff)
	c := saveContext()
	c.SaveSetV(IntVal(0), IntVal(1))
	c.SaveWrite("wrap")
	var a saveRecord
	saveReadRecord(0, &a)
	savePutWord(&a, 8, ^uint32(0))
	savePutWord(&a, 44, saveCRC(&a))
	for i, b := range a {
		hw.WriteSRAMByte(i, b)
	}
	c.SaveSetV(IntVal(0), IntVal(2))
	if c.SaveWrite("wrap") != SaveOK {
		t.Fatal("wrap write")
	}
	boot := saveContext()
	if boot.SaveRead("wrap") != SaveOK || boot.SaveGetV(IntVal(0)) != IntVal(2) {
		t.Fatal("generation zero not newest")
	}
	var absent Context
	if absent.SaveRead("test") != SaveIOError || absent.SaveWrite("test") != SaveIOError || absent.SaveSetV(IntVal(0), IntVal(0)).Truthy() || absent.SaveGetV(IntVal(0)) != IntVal(0) {
		t.Fatal("missing memory")
	}
}

func TestSaveWriteFailureCanRetry(t *testing.T) {
	hw.ResetSRAMForTest(0xff)
	c := saveContext()
	c.SaveSetV(IntVal(0), IntVal(12))
	hw.SetSRAMWriteLimitForTest(0)
	if c.SaveWrite("retry") != SaveIOError {
		t.Fatal("read-only save reported success")
	}
	if c.SaveGetV(IntVal(0)) != IntVal(12) {
		t.Fatal("progress lost")
	}
	hw.SetSRAMWriteLimitForTest(-1)
	if c.SaveWrite("retry") != SaveOK {
		t.Fatal("retry failed")
	}
}

func TestSaveCRCAndMaximumKey(t *testing.T) {
	for seed := uint32(0); seed < 64; seed++ {
		var sample saveRecord
		state := seed + 1
		for i := range sample {
			state = state*1664525 + 1013904223
			sample[i] = byte(state >> 24)
		}
		covered := append([]byte{}, sample[1:44]...)
		covered = append(covered, sample[48:]...)
		if got, want := saveCRC(&sample), crc32.ChecksumIEEE(covered); got != want {
			t.Fatalf("seed %d CRC=%x want %x", seed, got, want)
		}
	}
	hw.ResetSRAMForTest(0xff)
	c := saveContext()
	key := "01234567890123456789012345678901"
	if c.SaveWrite(key) != SaveOK || c.SaveRead(key) != SaveOK {
		t.Fatal("32-byte key")
	}
	var record saveRecord
	saveReadRecord(0, &record)
	covered := append([]byte{}, record[1:44]...)
	covered = append(covered, record[48:]...)
	if got, want := saveWord(&record, 44), crc32.ChecksumIEEE(covered); got != want {
		t.Fatalf("CRC=%x want %x", got, want)
	}
	// A future unsupported record version must not load as current data.
	record[5] = 2
	savePutWord(&record, 44, saveCRC(&record))
	for i, b := range record {
		hw.WriteSRAMByte(i, b)
	}
	if c.SaveRead(key) != SaveCorrupt {
		t.Fatal("unsupported format accepted")
	}
}
