package emulator

import (
	"bytes"
	"strings"
	"sync"
)

const diagnosticLimit = 32 * 1024

// diagnosticLog is written by os/exec's stderr pump while the preview reads
// frames. Keep both error context and unread messages bounded and race-free.
type diagnosticLog struct {
	mu        sync.Mutex
	tail      []byte
	pending   []byte
	truncated bool
}

func appendDiagnosticBytes(dst, src []byte) ([]byte, bool) {
	if len(src) >= diagnosticLimit {
		return append(dst[:0], src[len(src)-diagnosticLimit:]...), len(dst)+len(src) > diagnosticLimit
	}
	dropped := len(dst) + len(src) - diagnosticLimit
	if dropped > 0 {
		copy(dst, dst[dropped:])
		dst = dst[:len(dst)-dropped]
	}
	return append(dst, src...), dropped > 0
}

func (l *diagnosticLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tail, _ = appendDiagnosticBytes(l.tail, p)
	var dropped bool
	l.pending, dropped = appendDiagnosticBytes(l.pending, p)
	l.truncated = l.truncated || dropped
	return len(p), nil
}

func (l *diagnosticLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.TrimSpace(string(l.tail))
}

func (l *diagnosticLog) drain() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	end := bytes.LastIndexByte(l.pending, '\n')
	if end < 0 {
		return nil
	}
	var lines []string
	if l.truncated {
		lines = append(lines, "Earlier runtime messages were truncated.")
	}
	for _, line := range strings.Split(string(l.pending[:end]), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	copy(l.pending, l.pending[end+1:])
	l.pending = l.pending[:len(l.pending)-end-1]
	l.truncated = false
	return lines
}

// DrainLogs returns complete helper/ROM diagnostic lines received since the
// last call. Diagnostics use stderr, separate from the binary frame protocol.
func (s *Session) DrainLogs() []string { return s.stderr.drain() }
