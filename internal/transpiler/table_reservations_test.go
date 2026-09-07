package transpiler

import (
	"fmt"
	"strings"
	"testing"
)

func TestLoopTableReservationsRetainBuiltinPrecedence(t *testing.T) {
	source := `
hidden=0
values=nil
function add(t,value) hidden=99 end
function rnd(n) hidden=88 return n end
function fill()
  values={}
  for i=1,4 do add(values,{x=rnd(7)}) end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestReservationBuiltinResolution(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 c.Srand(rt.FromInt(17));random.Srand(rt.FromInt(17));g.fill(&c)
 if g.hidden!=0||c.TableLen(g.values)!=4 {t.Fatal("builtin resolution changed")}
 for i:=1;i<=4;i++ {
  v:=c.TableGet(c.TableGetInt(g.values,i),c.InternStatic("x"))
  if v!=rt.NumVal(random.Rnd(rt.FromInt(7))) {t.Fatal("builtin RNG changed")}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("reservation-builtins.lua", source, Options{DisableLoopTableReservations: disabled})
		if err != nil {
			t.Fatal(err)
		}
		runGeneratedHarness(t, got, harness, "TestReservationBuiltinResolution")
	}
}

func TestLoopTableReservationsPreserveRNGValuesAndFinalLayout(t *testing.T) {
	source := `
first=nil
second=nil
function fill()
  first={}
  unrelated=0
  second={}
  for i=0,16 do add(first,{x=rnd(10),index=i}) end
  for i=24,0,-1 do add(second,{x=rnd(10),index=i}) end
end
`
	harness := `package main
import("testing";rt "github.com/drpaneas/mtile/runtime")
func TestReservedLists(t *testing.T) {
 var g Game;var c,random rt.Context;g.Init(&c)
 for pass:=0;pass<4;pass++ {
  c.Srand(rt.FromInt(pass+10));random.Srand(rt.FromInt(pass+10));g.fill(&c)
  for which,list:=range []rt.Value{g.first,g.second} {
   count:=17;if which==1 {count=25}
   if c.TableLen(list)!=count||c.TableSlotCount(list)!=33 {t.Fatalf("list%d length/capacity differs",which)}
   for i:=0;i<count;i++ {
    record:=c.TableGetInt(list,i+1)
    if c.TableGet(record,c.InternStatic("x"))!=rt.NumVal(random.Rnd(rt.FromInt(10))) {t.Fatal("RNG/evaluation order changed")}
    index:=i;if which==1 {index=24-i}
    if c.TableGet(record,c.InternStatic("index"))!=rt.IntVal(index) {t.Fatal("append order changed")}
    if c.TableKeyAt(list,i)!=rt.IntVal(i+1) {t.Fatal("pairs array order changed")}
   }
  }
  if err:=c.Collect();err!=rt.HeapOK {t.Fatal(err)}
 }
}
`
	for _, disabled := range []bool{false, true} {
		got, err := Transpile("reserved-lists.lua", source, Options{DisableLoopTableReservations: disabled})
		if err != nil {
			t.Fatal(err)
		}
		body := generatedSection(got, "func (g *Game) fill(")
		want := 2
		if disabled {
			want = 0
		}
		if count := strings.Count(body, "c.NewTableWithCaps(32, 1)"); count != want {
			t.Fatalf("reserved %d arrays, want%d:\n%s", count, want, body)
		}
		runGeneratedHarness(t, got, harness, "TestReservedLists")
	}
}

func TestLoopTableReservationsUseExactDoublingCapacity(t *testing.T) {
	for _, tc := range []struct {
		bounds string
		cap    int
	}{
		{"1,2", 2}, {"1,3", 4}, {"0,16", 32}, {"24,0,-1", 32}, {"9,-23,-2", 32}, {"1,256", 256},
	} {
		t.Run(tc.bounds, func(t *testing.T) {
			got, err := Transpile("reservation-capacity.lua", "function fill() local list={} for i="+tc.bounds+" do add(list,true) end end", Options{})
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("c.NewTableWithCaps(%d, 1)", tc.cap)
			if !strings.Contains(generatedSection(got, "func (g *Game) fill("), want) {
				t.Fatalf("missing exact doubling capacity %s", want)
			}
		})
	}
}

func TestLoopTableReservationsRejectUnprovenShapes(t *testing.T) {
	for _, tc := range []struct{ name, setup, loop string }{
		{"dynamic-bound", "", "for i=1,n do add(list,{}) end"},
		{"overflow-positive", "", "for i=32760,32767 do add(list,{}) end"},
		{"overflow-negative", "", "for i=-32760,-32768,-1 do add(list,{}) end"},
		{"zero-step", "", "for i=1,17,0 do add(list,{}) end"},
		{"fractional-step", "", "for i=1,17,0.5 do add(list,{}) end"},
		{"empty-loop", "", "for i=17,1 do add(list,{}) end"},
		{"too-many", "", "for i=1,257 do add(list,{}) end"},
		{"alias", "other=list", "for i=1,17 do add(list,{}) end"},
		{"observed-before-fill", "other=#list", "for i=1,17 do add(list,{}) end"},
		{"rebound", "list=other", "for i=1,17 do add(list,{}) end"},
		{"shadowed", "local list=other", "for i=1,17 do add(list,{}) end"},
		{"loop-shadow", "", "for list=1,17 do add(list,{}) end"},
		{"unknown-call", "unknown()", "for i=1,17 do add(list,{}) end"},
		{"unknown-value-call", "", "for i=1,17 do add(list,{x=unknown()}) end"},
		{"aliased-value", "", "for i=1,17 do add(list,{x=list}) end"},
		{"conditional", "", "for i=1,17 do if i<9 then add(list,{}) end end"},
		{"break", "", "for i=1,17 do add(list,{}) break end"},
		{"indexed-add", "", "for i=1,17 do add(list,{},1) end"},
		{"nil-value", "", "for i=1,17 do add(list,nil) end"},
		{"unknown-value", "", "for i=1,17 do add(list,value) end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "other=nil\nfunction unknown() return 1 end\nfunction fill(n,value) local list={} " + tc.setup + " " + tc.loop + " end"
			got, err := Transpile("unproven-reservation.lua", source, Options{})
			if err != nil {
				t.Fatal(err)
			}
			body := generatedSection(got, "func (g *Game) fill(")
			if strings.Contains(body, "c.NewTableWithCaps(32, 1)") || strings.Contains(body, "c.NewTableWithCaps(8, 1)") || strings.Contains(body, "c.NewTableWithCaps(16, 1)") {
				t.Fatalf("reserved an unproven list:\n%s", body)
			}
		})
	}
}
