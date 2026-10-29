package transpiler

import "testing"

func TestGeneratedArrayStorePreservesRootsAndFallback(t *testing.T) {
	generated, err := Transpile("array-store-roots.lua", "function _update() end", Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, `package main
import ("testing"; rt "github.com/drpaneas/mtile/runtime")
func TestArrayStoreRoots(t *testing.T) {
 var c rt.Context
 owner,err:=c.NewTableWithCaps(4,1);if err!=rt.HeapOK{t.Fatal(err)}
 c.RegisterGlobal(&owner)
 value,err:=c.NewString([]byte("retained"));if err!=rt.HeapOK{t.Fatal(err)}
 // Existing slots cannot collect, even with no room for temporary roots.
 for i:=0;i<rt.MaxLocalRoots;i++{if !c.PushLocal(owner){t.Fatal("setup roots")}}
 if err:=gluTableSet(&c,owner,rt.IntVal(1),value);err!=rt.HeapOK{t.Fatal(err)}
 // A hash/growth store must still use the rooted path, and leave scopes balanced.
 if err:=gluTableSet(&c,owner,rt.IntVal(200),value);err!=rt.HeapOOM{t.Fatal("unrooted slow path",err)}
 c.PopLocals(rt.MaxLocalRoots)
 if err:=c.Collect();err!=rt.HeapOK{t.Fatal(err)}
 if !c.ValuesEqual(c.TableGetInt(owner,1),c.InternStatic("retained")){t.Fatal("fast store lost reference")}
 value=c.TableGetInt(owner,1)
 if err:=gluTableSet(&c,owner,rt.IntVal(200),value);err!=rt.HeapOK{t.Fatal(err)}
 if err:=c.Collect();err!=rt.HeapOK{t.Fatal(err)}
 if !c.ValuesEqual(c.TableGetInt(owner,200),c.InternStatic("retained")){t.Fatal("slow store lost reference")}
 for i:=0;i<rt.MaxLocalRoots;i++{if !c.PushLocal(owner){t.Fatal("helper leaked roots")}}
 c.PopLocals(rt.MaxLocalRoots)
}
`, "TestArrayStoreRoots")
}
