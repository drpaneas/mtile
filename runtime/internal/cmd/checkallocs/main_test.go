package main

import (
	"strings"
	"testing"
)

func TestCheckAllocs_zeroCountIsPass(t *testing.T) {
	t.Parallel()
	src := "mode: set\n/opt/homebrew/Cellar/tinygo/0.41.1/src/runtime/baremetal.go:44.1,44.24 1 0\n"
	if err := checkAllocs(strings.NewReader(src)); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAllocs_nonzeroCountIsFail(t *testing.T) {
	t.Parallel()
	src := "mode: set\n/tmp/foo.go:1.1,1.10 1 3\n"
	if err := checkAllocs(strings.NewReader(src)); err == nil {
		t.Fatal("expected fail on nonzero count")
	}
}

func TestCheckAllocs_ignoresLinkerNoise(t *testing.T) {
	t.Parallel()
	src := "ld.lld --gc-sections -o /tmp/main\nmode: set\n"
	if err := checkAllocs(strings.NewReader(src)); err != nil {
		t.Fatal(err)
	}
}

func TestAllocCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		line   string
		wantN  int
		wantOK bool
	}{
		{name: "header", line: "mode: set", wantOK: false},
		{name: "zero", line: "/x.go:44.1,44.24 1 0", wantN: 0, wantOK: true},
		{name: "nonzero", line: "/x.go:44.1,44.24 1 2", wantN: 2, wantOK: true},
		{name: "lld", line: "ld.lld --gc-sections", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n, ok := allocCount(tt.line)
			if ok != tt.wantOK || n != tt.wantN {
				t.Fatalf("allocCount(%q) = %d, %v", tt.line, n, ok)
			}
		})
	}
}
