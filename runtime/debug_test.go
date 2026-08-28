package runtime

import "testing"

func TestDebugMailboxStableLayout(t *testing.T) {
	var mailbox DebugMailbox
	stats := PerformanceStats{Frames: 7, MissedFrames: 2, LastFrame: 99, LastUpdate: 11, LastDraw: 22, DiagnosticSkips: 3}
	mailbox.Begin(stats)
	mailbox.Add("score", NumVal(Number(3*One+One/2)))
	mailbox.Add("alive", BoolVal(true))
	mailbox.End()
	if string(mailbox[:4]) != "GLUD" || debugGet32(mailbox[:], 12) != 7 || debugGet32(mailbox[:], 16) != 2 {
		t.Fatalf("mailbox header/stats were not encoded: %x", mailbox[:24])
	}
	if debugGet32(mailbox[:], debugSequenceOffset)&1 != 0 {
		t.Fatal("published mailbox sequence is odd")
	}
	if mailbox[debugWatchCount] != 2 || string(mailbox[debugWatchBase:debugWatchBase+5]) != "score" {
		t.Fatalf("watch metadata = %x", mailbox[debugWatchBase:debugWatchBase+48])
	}
	if got := int32(debugGet32(mailbox[:], debugWatchBase+20)); got != int32(3*One+One/2) {
		t.Fatalf("fixed point watch = %d", got)
	}
}
