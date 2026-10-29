package runtime

import (
	"github.com/drpaneas/mtile/runtime/hw"
	"testing"
)

func TestIndependentSaveSlotsAndLegacyLayout(t *testing.T) {
	hw.ResetSRAMForTest(255)
	c := saveContext()
	for slot := 0; slot < SaveSlotCount; slot++ {
		if c.SaveReadSlot("slots-v1", slot) != SaveEmpty {
			t.Fatalf("slot %d not empty", slot)
		}
		for field := 0; field < SaveFieldCount; field++ {
			c.SaveSetV(IntVal(field), NumVal(Number(uint32(slot*64+field)*0x891173ab)))
		}
		if c.SaveWriteSlot("slots-v1", slot) != SaveOK {
			t.Fatalf("slot %d write", slot)
		}
	}
	boot := saveContext()
	for slot := 0; slot < SaveSlotCount; slot++ {
		if boot.SaveReadSlot("slots-v1", slot) != SaveOK {
			t.Fatal("cold read", slot)
		}
		for field := 0; field < SaveFieldCount; field++ {
			if boot.SaveGetV(IntVal(field)) != NumVal(Number(uint32(slot*64+field)*0x891173ab)) {
				t.Fatal("slot alias", slot, field)
			}
		}
	}
	first := uint32(0x891173ab)
	if boot.SaveRead("slots-v1") != SaveOK || boot.SaveGetV(IntVal(1)) != NumVal(Number(first)) {
		t.Fatal("legacy slot zero")
	}
	if hw.ReadSRAMByte(31*saveSlotStride+saveRecordBytes) != 255 || hw.ReadSRAMByte(32767) != 255 {
		t.Fatal("wrote outside record")
	}
}

func TestSaveSlotValidationHasNoEffects(t *testing.T) {
	hw.ResetSRAMForTest(255)
	c := saveContext()
	c.SaveSetV(IntVal(0), IntVal(99))
	for _, slot := range []Value{NilVal(), BoolVal(false), IntVal(-1), IntVal(32), NumVal(One / 2)} {
		if c.SaveReadV("v1", slot) != IntVal(SaveBadSlot) || c.SaveWriteV("v1", slot) != IntVal(SaveBadSlot) {
			t.Fatal("invalid slot accepted", slot)
		}
	}
	for _, slot := range []int{-1, 32, 65536} {
		if c.SaveReadSlot("v1", slot) != SaveBadSlot || c.SaveWriteSlot("v1", slot) != SaveBadSlot {
			t.Fatal("Go slot accepted")
		}
	}
	if c.SavePending() || c.SaveGetV(IntVal(0)) != IntVal(99) {
		t.Fatal("invalid slot changed RAM")
	}
	for i := 0; i < 32768; i++ {
		if hw.ReadSRAMByte(i) != 255 {
			t.Fatal("invalid slot touched SRAM", i)
		}
	}
}

func TestSlotInterruptionNeverTouchesNeighbours(t *testing.T) {
	for _, slot := range []int{1, 31} {
		for cut := 0; cut <= saveRecordBytes+1; cut++ {
			hw.ResetSRAMForTest(255)
			c := saveContext()
			c.SaveSetV(IntVal(0), IntVal(7))
			c.SaveWriteSlot("neighbour", slot-1)
			for n := 1; n <= 3; n++ {
				c.SaveSetV(IntVal(0), IntVal(n))
				if c.SaveWriteSlot("target", slot) != SaveOK {
					t.Fatal("setup")
				}
			}
			c.SaveSetV(IntVal(0), IntVal(99))
			hw.SetSRAMWriteLimitForTest(cut)
			result := c.SaveWriteSlot("target", slot)
			hw.SetSRAMWriteLimitForTest(-1)
			boot := saveContext()
			if boot.SaveReadSlot("target", slot) != SaveOK {
				t.Fatal("lost prior save", slot, cut)
			}
			v := boot.SaveGetV(IntVal(0))
			if v != IntVal(3) && v != IntVal(99) || result == SaveOK && v != IntVal(99) {
				t.Fatal("torn record", slot, cut, v)
			}
			if boot.SaveReadSlot("neighbour", slot-1) != SaveOK || boot.SaveGetV(IntVal(0)) != IntVal(7) {
				t.Fatal("damaged neighbour", slot, cut)
			}
			if boot.SaveReadSlot("other-key", slot) != SaveWrongKey || boot.SaveGetV(IntVal(0)) != IntVal(7) {
				t.Fatal("wrong-key read changed fields")
			}
		}
	}
}
