package transpiler

import (
	"strings"
	"testing"

	"github.com/drpaneas/mtile/internal/luaparser"
	runtimeapi "github.com/drpaneas/mtile/runtime"
)

func TestMode0BudgetsExpressionPoseDrawsAndAssignsDistinctWindows(t *testing.T) {
	cases := map[string]string{
		"local assignment":  `function _draw() cls() local discarded=pspr(0,0,0) pspr(0,8,0) end`,
		"global assignment": `function _draw() cls() discarded=pspr(0,0,0) pspr(0,8,0) end`,
		"helper result":     `function mark() pspr(0,0,0) return 2 end function _draw() cls() local x=mark() pspr(0,x,0) end`,
		"helper return":     `function mark() return pspr(0,0,0) end function _draw() cls() local x=mark() pspr(0,8,0) end`,
		"argument first":    `function mark() pspr(0,0,0) return 2 end function _draw() cls() pspr(0,mark(),0) end`,
		"if condition":      `function _draw() cls() if pspr(0,0,0) then end pspr(0,8,0) end`,
		"elseif condition":  `function _draw() cls() if flag then elseif pspr(0,0,0) then end pspr(0,8,0) end`,
		"short circuit":     `function _draw() cls() local x=flag and pspr(0,0,0) pspr(0,8,0) end`,
		"table field":       `function _draw() cls() local x={a=pspr(0,0,0)} pspr(0,8,0) end`,
		"table key":         `function mark() pspr(0,0,0) return 2 end function _draw() cls() local x={[mark()]=true} pspr(0,8,0) end`,
		"index target":      `function mark() pspr(0,0,0) return 2 end function _draw() cls() local x={} x[mark()]=true pspr(0,8,0) end`,
		"numeric for bound": `function mark() pspr(0,0,0) return 2 end function _draw() cls() for i=1,mark() do end pspr(0,8,0) end`,
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			code, report, err := TranspileWithReport("expression.lua", source, psprMode0Opts(runtimeapi.PosePart{Tile: 0}))
			if err != nil {
				t.Fatal(err)
			}
			if report.Mode0OAMEntries != 2 {
				t.Fatalf("OAM=%d, want 2", report.Mode0OAMEntries)
			}
			for _, call := range []string{"PSprSiteV(rt.IntVal(0), rt.IntVal(0)", "PSprSiteV(rt.IntVal(1), rt.IntVal(1)"} {
				if !strings.Contains(code, call) {
					t.Fatalf("missing distinct allocation %q", call)
				}
			}
		})
	}
}

func TestMode0RepeatedExpressionDrawUsesPerInvocationAllocation(t *testing.T) {
	code, report, err := TranspileWithReport("expression.lua", `function _draw() cls() for i=1,3 do local x=pspr(0,i*8,0) end end`, psprMode0Opts(runtimeapi.PosePart{Tile: 0}))
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode0OAMEntries != 3 || !strings.Contains(code, "PSprSiteFromBaseV(rt.IntVal(0), rt.IntVal(0), rt.IntVal(1)") {
		t.Fatalf("loop allocation was not budgeted for 3 draws: %+v", report)
	}
}

func TestMode0DrawsInUnboundedLoopConditionsFailClosed(t *testing.T) {
	for _, loop := range []string{`while probe() do end`, `repeat until probe()`} {
		source := `function probe() pspr(0,0,0) return flag end function _draw() cls() ` + loop + ` pspr(0,8,0) end`
		opts := psprMode0Opts(runtimeapi.PosePart{Tile: 0})
		if _, _, err := TranspileWithReport("condition.lua", source, opts); err == nil || !strings.Contains(err.Error(), "condition.lua:") {
			t.Fatalf("unbounded condition accepted: %v", err)
		}
		opts.Renderer = RendererAuto
		_, report, err := TranspileWithReport("condition.lua", source, opts)
		if err != nil || report.Selected != RendererMode4 {
			t.Fatalf("AUTO failed to fall back: %+v error=%v", report, err)
		}
	}
}

func TestMode0ExpressionTraversalKeepsPureRecursiveHelpers(t *testing.T) {
	source := `function sum(n) if n<=0 then return 0 end return n+sum(n-1) end function _draw() cls() pspr(0,sum(3),0) end`
	chunk, err := luaparser.ParseString("pure-helper.lua", source)
	if err != nil {
		t.Fatal(err)
	}
	report, err := SelectRenderer(chunk, psprMode0Opts(runtimeapi.PosePart{Tile: 0}))
	if err != nil || report.Mode0OAMEntries != 1 {
		t.Fatalf("pure helper affected draw proof: %+v error=%v", report, err)
	}
}

func TestMode0HiddenExpressionDrawsCannotBypassObjectBudget(t *testing.T) {
	source := `function _draw() cls() for i=1,128 do local x=pspr(0,i,0) end pspr(0,0,0) end`
	_, _, err := TranspileWithReport("expression-budget.lua", source, psprMode0Opts(runtimeapi.PosePart{Tile: 0}))
	if err == nil || !strings.Contains(err.Error(), "128") {
		t.Fatalf("129 hidden/visible draws escaped physical limit: %v", err)
	}
}
