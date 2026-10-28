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

func TestParsePauseMenuCallbackNames(t *testing.T) {
	chunk, err := ParseString("pause.lua", `pause_menu("store_progress", "restore_progress")`)
	if err != nil {
		t.Fatal(err)
	}
	call := chunk.Body[0].(*CallStmt).Call
	for i, want := range []string{"store_progress", "restore_progress"} {
		literal, ok := call.Args[i].(*StringExpr)
		if !ok || literal.Value != want || literal.Pos().Source != "pause.lua" {
			t.Fatalf("callback %d: %#v", i, call.Args[i])
		}
	}
}
