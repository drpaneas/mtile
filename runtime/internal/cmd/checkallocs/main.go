package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	in := io.Reader(os.Stdin)
	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "checkallocs: %v\n", err)
			os.Exit(2)
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	if err := checkAllocs(in); err != nil {
		fmt.Fprintf(os.Stderr, "checkallocs: %v\n", err)
		os.Exit(1)
	}
}

// checkAllocs accepts TinyGo -print-allocs=. output (stderr). That flag uses
// Go's "mode: set" coverage-profile format. A trailing count of 0 is a regexp
// match that did not allocate. Only a nonzero last field is a fail.
func checkAllocs(r io.Reader) error {
	sc := bufio.NewScanner(r)
	var allocs []string
	for sc.Scan() {
		n, ok := allocCount(sc.Text())
		if !ok {
			continue
		}
		if n != 0 {
			allocs = append(allocs, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(allocs) > 0 {
		return fmt.Errorf("heap allocs:\n%s", strings.Join(allocs, "\n"))
	}
	return nil
}

func allocCount(line string) (int, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "mode:") {
		return 0, false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 || !strings.Contains(fields[0], ".go:") {
		return 0, false
	}
	count, err1 := strconv.Atoi(fields[len(fields)-1])
	if _, err2 := strconv.Atoi(fields[len(fields)-2]); err1 != nil || err2 != nil {
		return 0, false
	}
	return count, true
}
