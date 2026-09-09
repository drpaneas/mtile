package emulator

import (
	"bytes"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestDiagnosticLogDrainsCompleteLinesOnce(t *testing.T) {
	var session Session
	_, _ = session.stderr.Write([]byte("mGBA: first\npart"))
	if got := session.DrainLogs(); !reflect.DeepEqual(got, []string{"mGBA: first"}) {
		t.Fatalf("first drain = %q", got)
	}
	if got := session.DrainLogs(); len(got) != 0 {
		t.Fatalf("repeated drain = %q", got)
	}
	_, _ = session.stderr.Write([]byte("ial\r\n\nlast\n"))
	if got := session.DrainLogs(); !reflect.DeepEqual(got, []string{"partial", "last"}) {
		t.Fatalf("second drain = %q", got)
	}
	if got := session.stderr.String(); !strings.Contains(got, "first\npartial") {
		t.Fatalf("draining lost error context: %q", got)
	}
}

func TestDiagnosticLogBoundsUnterminatedAndUnreadOutput(t *testing.T) {
	var log diagnosticLog
	message := bytes.Repeat([]byte{'x'}, diagnosticLimit*4)
	if n, err := log.Write(message); n != len(message) || err != nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	if len(log.tail) != diagnosticLimit || len(log.pending) != diagnosticLimit || len(log.drain()) != 0 {
		t.Fatal("unterminated output was unbounded or emitted early")
	}
	_, _ = log.Write([]byte("\nnewest\n"))
	lines := log.drain()
	if len(log.tail) > diagnosticLimit || len(log.pending) != 0 || len(lines) < 2 || !strings.Contains(lines[0], "truncated") || lines[len(lines)-1] != "newest" {
		t.Fatalf("bounded drain lost newest output: %d lines", len(lines))
	}
}

func TestDiagnosticLogConcurrentReadsAndWrites(t *testing.T) {
	var log diagnosticLog
	var writers sync.WaitGroup
	writers.Go(func() {
		for i := 0; i < 1000; i++ {
			_, _ = log.Write([]byte("message\n"))
		}
	})
	for i := 0; i < 1000; i++ {
		_ = log.String()
		log.drain()
	}
	writers.Wait()
	_, _ = log.Write([]byte("last\n"))
	if !strings.HasSuffix(log.String(), "last") {
		t.Fatal("lost final message")
	}
}
