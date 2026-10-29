package luaparser

import "testing"

func TestParseSaveCallsPreservesKeyAndPosition(t *testing.T) {
	source := "save_read(\"checkpoint-v1\")\nsave_set(63, -12.5)\nsave_write((\"checkpoint-v1\"))\nscore=save_get(63)"
	chunk, err := ParseString("saving.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Body) != 4 {
		t.Fatal("lost save statements")
	}
	for _, i := range []int{0, 2} {
		call := chunk.Body[i].(*CallStmt).Call
		arg := call.Args[0]
		if p, ok := arg.(*ParenExpr); ok {
			arg = p.Expr
		}
		key, ok := arg.(*StringExpr)
		if !ok || key.Value != "checkpoint-v1" || key.Pos().Source != "saving.lua" || key.Pos().Line != i+1 {
			t.Fatalf("key/position: %#v", arg)
		}
	}
}

func TestParseSaveSlotAndPending(t *testing.T) {
	chunk, err := ParseString("slots.lua", `save_write("profiles-v1", selected)
save_read("profiles-v1", 2)
ready = not save_pending()`)
	if err != nil {
		t.Fatal(err)
	}
	call := chunk.Body[0].(*CallStmt).Call
	if len(call.Args) != 2 || call.Args[1].(*VarExpr).Name != "selected" || call.Args[1].Pos().Line != 1 {
		t.Fatal("lost dynamic slot")
	}
	if chunk.Body[1].(*CallStmt).Call.Args[1].(*NumberExpr).Value != 2 {
		t.Fatal("lost literal slot")
	}
}
