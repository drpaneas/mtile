package transpiler

import "testing"

func TestGeneratedRootHelpersRetainOnlyHeapValues(t *testing.T) {
	generated, err := Transpile("heap-root-filter.lua", "function _update() end", Options{})
	if err != nil {
		t.Fatal(err)
	}
	runGeneratedHarness(t, generated, `package main

import (
  rt "github.com/drpaneas/mtile/runtime"
  "testing"
)

func TestHeapRootFilter(t *testing.T) {
  var c rt.Context
  table, err := c.NewTableWithCaps(1, 1)
  if err != rt.HeapOK { t.Fatal(err) }
  if err := c.TableSet(table, rt.IntVal(1), rt.IntVal(42)); err != rt.HeapOK { t.Fatal(err) }
  str, err := c.NewString([]byte("heap value"))
  if err != rt.HeapOK { t.Fatal(err) }
  static := c.InternStatic("heap value")
  n := gluPushRoots(&c, rt.NilVal(), rt.IntVal(42), rt.BoolVal(true), static, table, str)
  if n != 2 { t.Fatalf("root count = %d, want two heap references", n) }
  if err := c.Collect(); err != rt.HeapOK { t.Fatal(err) }
  if got := c.TableGet(table, rt.IntVal(1)); got != rt.IntVal(42) { t.Fatalf("table value after GC = %+v", got) }
  if !c.ValuesEqual(str, static) { t.Fatal("heap string did not survive GC") }
  // A nested allocation must leave the caller's two references protected.
  combined := gluConcat(&c, str, static, table, rt.IntVal(99))
  if c.StrLen(combined) != 20 { t.Fatal("nested concat failed") }
  if err := c.Collect(); err != rt.HeapOK { t.Fatal(err) }
  if c.TableGet(table, rt.IntVal(1)) != rt.IntVal(42) || !c.ValuesEqual(str, static) {
    t.Fatal("nested helper discarded caller roots")
  }
  gluPopRoots(&c, n)

  // Fill all but one slot. On failure the helper must pop only the one
  // reference it pushed, leaving all preexisting roots in place.
  for i := 0; i < rt.MaxLocalRoots-1; i++ {
    if !c.PushLocal(table) { t.Fatalf("root leak before slot %d", i) }
  }
  if n := gluPushRoots(&c, rt.IntVal(1), table, rt.BoolVal(false), str); n != -1 {
    t.Fatalf("overflow result = %d, want -1", n)
  }
  if !c.PushLocal(table) { t.Fatal("failed helper leaked its partial root") }
  if c.PushLocal(table) { t.Fatal("failed helper popped preexisting roots") }
  c.PopLocals(rt.MaxLocalRoots)
  if n := gluPushRoots(&c, rt.NilVal(), static, rt.BoolVal(false), rt.IntVal(-1)); n != 0 {
    t.Fatalf("scalar-only root count = %d, want zero", n)
  }
}
`, "TestHeapRootFilter")
}
