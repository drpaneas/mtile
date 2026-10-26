package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLSPCommandUsesStdioAndKeepsStdoutFramed(t *testing.T) {
	var input, output, stderr bytes.Buffer
	for _, message := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"done","method":"shutdown"}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	} {
		fmt.Fprintf(&input, "Content-Length: %d\r\n\r\n%s", len(message), message)
	}
	if err := runWithInput([]string{"lsp"}, &input, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	frames := 0
	for text != "" {
		header, body, ok := strings.Cut(text, "\r\n\r\n")
		var size int
		if _, err := fmt.Sscanf(header, "Content-Length: %d", &size); !ok || err != nil || size > len(body) {
			t.Fatalf("stdout contains non-protocol output: %q", text)
		}
		if !json.Valid([]byte(body[:size])) {
			t.Fatalf("invalid JSON response: %q", body[:size])
		}
		text = body[size:]
		frames++
	}
	if frames != 2 || stderr.Len() != 0 {
		t.Fatalf("frames=%d stderr=%q", frames, stderr.String())
	}
}

func TestLSPHelpUsesStderrAndInvalidArgumentsFail(t *testing.T) {
	var output, stderr bytes.Buffer
	if err := runWithInput([]string{"lsp", "-h"}, strings.NewReader(""), &output, &stderr); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 || !strings.Contains(stderr.String(), "Usage of lsp") {
		t.Fatalf("stdout=%q stderr=%q", output.String(), stderr.String())
	}
	if err := runWithInput([]string{"lsp", "one", "two"}, strings.NewReader(""), &output, &stderr); err == nil {
		t.Fatal("accepted two project roots")
	}
}
