package luaparser

import "testing"

func TestParsePauseEnableArgumentsAndPosition(t *testing.T) {
	chunk, err := ParseString("pause.lua", "pause(true)\npause(false)\npause(scene == 2)")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		call := chunk.Body[i].(*CallStmt).Call
		if callee, ok := call.Callee.(*VarExpr); !ok || callee.Name != "pause" {
			t.Fatal("lost public pause name")
		}
		if len(call.Args) != 1 || call.Pos().Source != "pause.lua" || call.Pos().Line != i+1 {
			t.Fatal("lost argument or position")
		}
	}
	if !chunk.Body[0].(*CallStmt).Call.Args[0].(*BoolExpr).Value || chunk.Body[1].(*CallStmt).Call.Args[0].(*BoolExpr).Value {
		t.Fatal("lost boolean")
	}
	if _, ok := chunk.Body[2].(*CallStmt).Call.Args[0].(*BinaryExpr); !ok {
		t.Fatal("lost dynamic expression")
	}
}
