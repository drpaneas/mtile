package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/drpaneas/mtile/internal/assetsync"
	"github.com/drpaneas/mtile/internal/build"
)

type testMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Params json.RawMessage `json:"params"`
	Error  *rpcError       `json:"error"`
}

type testClient struct {
	t        *testing.T
	in       *io.PipeWriter
	messages chan testMessage
	errors   chan error
	done     chan error
}

func newTestClient(t *testing.T, root string) *testClient {
	return newTestClientWithCheck(t, root, analyze)
}

func newTestClientWithCheck(t *testing.T, root string, check projectCheck) *testClient {
	t.Helper()
	reader, input := io.Pipe()
	output, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	c := &testClient{t, input, make(chan testMessage, 64), make(chan error, 1), make(chan error, 1)}
	go func() { c.done <- serve(ctx, reader, writer, root, check); writer.Close() }()
	go func() {
		buffer := bufio.NewReader(output)
		for {
			data, err := readFrame(buffer)
			if err != nil {
				c.errors <- err
				return
			}
			var msg testMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				c.errors <- err
				return
			}
			select {
			case c.messages <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	t.Cleanup(func() { cancel(); input.Close(); reader.Close(); writer.Close(); output.Close() })
	return c
}

func (c *testClient) send(method string, params any, id any) {
	c.t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id != nil {
		msg["id"] = id
	}
	if err := writeFrame(c.in, msg); err != nil {
		c.t.Fatal(err)
	}
}

func (c *testClient) until(match func(testMessage) bool) testMessage {
	c.t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case msg := <-c.messages:
			if msg.Error != nil {
				c.t.Fatalf("RPC error: %+v", msg.Error)
			}
			if match(msg) {
				return msg
			}
		case err := <-c.errors:
			c.t.Fatalf("protocol reader: %v", err)
		case <-timer.C:
			c.t.Fatal("timed out waiting for language server")
		}
	}
}

func (c *testClient) response(id string) testMessage {
	return c.until(func(msg testMessage) bool { return string(msg.ID) == id })
}

type diagnosticsParams struct {
	URI         string             `json:"uri"`
	Version     *int               `json:"version"`
	Diagnostics []editorDiagnostic `json:"diagnostics"`
}

func (c *testClient) diagnostic(uri string, version int, code string) diagnosticsParams {
	c.t.Helper()
	var result diagnosticsParams
	c.until(func(msg testMessage) bool {
		if msg.Method != "textDocument/publishDiagnostics" {
			return false
		}
		if json.Unmarshal(msg.Params, &result) != nil || result.URI != uri || result.Version == nil || *result.Version != version {
			return false
		}
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Code == code {
				return true
			}
		}
		return false
	})
	return result
}

func (c *testClient) initialize(root string) {
	c.send("initialize", map[string]any{"rootUri": fileURI(root)}, 1)
	msg := c.response("1")
	if !strings.Contains(string(msg.Result), `"positionEncoding":"utf-16"`) || !strings.Contains(string(msg.Result), `"hoverProvider":true`) {
		c.t.Fatalf("capabilities = %s", msg.Result)
	}
	c.send("initialized", map[string]any{}, nil)
}

func (c *testClient) finish() {
	c.send("shutdown", nil, "shutdown")
	msg := c.response(`"shutdown"`)
	if string(msg.Result) != "null" {
		c.t.Fatalf("shutdown = %s", msg.Result)
	}
	c.send("exit", nil, nil)
	select {
	case err := <-c.done:
		if err != nil {
			c.t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		c.t.Fatal("server did not exit")
	}
}

func lspProject(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project with spaces")
	if err := build.CreateProject(root, build.NewProjectOptions{SheetWidth: 8, SheetHeight: 8, MapWidth: 1, MapHeight: 1}); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestStdioCompletionHoverAndRendererDiagnostics(t *testing.T) {
	root := lspProject(t)
	c := newTestClient(t, "")
	c.initialize(root)
	uri := fileURI(filepath.Join(root, "game.lua"))
	source := "function _draw()\n cls(0)\n circfill(4,4,1,7)\nend\n"
	c.send("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "lua", "version": 1, "text": source}}, nil)
	result := c.diagnostic(uri, 1, "M0_MODE4_BUILTIN")
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Severity != build.DiagnosticInformation || result.Diagnostics[0].Range.Start != (position{2, 1}) {
		t.Fatalf("AUTO diagnostic = %+v", result)
	}
	c.send("textDocument/hover", map[string]any{"textDocument": documentID{uri}, "position": position{2, 3}}, 2)
	if msg := c.response("2"); !strings.Contains(string(msg.Result), "Mode 4 only") || !strings.Contains(string(msg.Result), "circfill(") {
		t.Fatalf("hover = %s", msg.Result)
	}
	// The partial source is invalid Lua; completion must remain usable.
	c.send("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": uri, "version": 2}, "contentChanges": []any{map[string]string{"text": "function _draw()\n cls(0)\n ss"}}}, nil)
	c.send("textDocument/completion", map[string]any{"textDocument": documentID{uri}, "position": position{2, 3}}, 3)
	msg := c.response("3")
	if !strings.Contains(string(msg.Result), `"label":"sspr"`) || !strings.Contains(string(msg.Result), `"label":"sset"`) || strings.Contains(string(msg.Result), `"label":"spr"`) {
		t.Fatalf("completion = %s", msg.Result)
	}
	result = c.diagnostic(uri, 2, "LUA_SYNTAX")
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity != build.DiagnosticError {
			t.Fatalf("stale renderer fallback = %+v", result)
		}
	}
	// Apply an incremental edit and verify the same compiler fallback returns.
	c.send("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": uri, "version": 3}, "contentChanges": []any{map[string]any{"range": textRange{position{2, 1}, position{2, 3}}, "text": "circfill(4,4,1,7)\nend\n"}}}, nil)
	c.diagnostic(uri, 3, "M0_MODE4_BUILTIN")
	project, err := assetsync.LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	project.Renderer = "mode0"
	if err := assetsync.SaveProject(root, project); err != nil {
		t.Fatal(err)
	}
	c.send("workspace/didChangeWatchedFiles", map[string]any{"changes": []any{map[string]any{"uri": fileURI(filepath.Join(root, "project.json")), "type": 2}}}, nil)
	result = c.diagnostic(uri, 3, "M0_MODE4_BUILTIN")
	if result.Diagnostics[0].Severity != build.DiagnosticError {
		t.Fatalf("forced Mode 0 = %+v", result)
	}
	c.send("textDocument/didClose", map[string]any{"textDocument": documentID{uri}}, nil)
	c.until(func(msg testMessage) bool {
		var params diagnosticsParams
		return msg.Method == "textDocument/publishDiagnostics" && json.Unmarshal(msg.Params, &params) == nil && params.URI == uri && len(params.Diagnostics) == 0
	})
	c.finish()
}

func TestDiagnosticsUseUTF16AndOriginalModuleURI(t *testing.T) {
	root := lspProject(t)
	game := "require(\"view\")\nfunction _draw() cls(0) draw_scene() end\n"
	prefix := " local label = \"😀é\"; "
	module := "function draw_scene()\n" + prefix + "circfill(4,4,1,7)\nend\n"
	gamePath, modulePath := filepath.Join(root, "game.lua"), filepath.Join(root, "src/view.lua")
	gameURI, moduleURI := fileURI(gamePath), fileURI(modulePath)
	result := analyze(root, 1, map[string]document{
		gameURI:   {gameURI, gamePath, game, 2},
		moduleURI: {moduleURI, modulePath, module, 5},
	})
	diagnostics := result.diagnostics[moduleURI]
	if len(diagnostics) != 1 || len(result.diagnostics[gameURI]) != 0 {
		t.Fatalf("module diagnostics = %+v", result.diagnostics)
	}
	start := len(utf16.Encode([]rune(prefix)))
	if diagnostics[0].Range != (textRange{position{1, start}, position{1, start + 8}}) {
		t.Fatalf("Unicode range = %+v", diagnostics[0].Range)
	}
	if _, err := os.Stat(modulePath); !os.IsNotExist(err) {
		t.Fatal("analysis saved the module")
	}
}

func TestInFlightAnalysisCannotPublishOlderSourceVersion(t *testing.T) {
	root := lspProject(t)
	started, release := make(chan struct{}), make(chan struct{})
	var first, unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	c := newTestClientWithCheck(t, root, func(root string, revision uint64, docs map[string]document) analysisResult {
		first.Do(func() { close(started); <-release })
		return analyze(root, revision, docs)
	})
	c.initialize(root)
	uri := fileURI(filepath.Join(root, "game.lua"))
	c.send("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "text": "function _draw() cls(0) circfill(4,4,1,7) end", "version": 1}}, nil)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("analysis did not start")
	}
	c.send("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": uri, "version": 2}, "contentChanges": []any{map[string]string{"text": "function _draw()\n ss"}}}, nil)
	// This response proves the protocol loop processed the change while the
	// older compiler pass was still blocked in its worker.
	c.send("textDocument/completion", map[string]any{"textDocument": documentID{uri}, "position": position{1, 3}}, 9)
	c.response("9")
	unblock.Do(func() { close(release) })
	c.until(func(msg testMessage) bool {
		if msg.Method != "textDocument/publishDiagnostics" {
			return false
		}
		var params diagnosticsParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.Version != nil && *params.Version == 1 && len(params.Diagnostics) > 0 {
			t.Fatalf("published stale renderer diagnostic: %+v", params)
		}
		return params.Version != nil && *params.Version == 2 && len(params.Diagnostics) == 1 && params.Diagnostics[0].Code == "LUA_SYNTAX"
	})
	c.finish()
}

func TestWatchRegistrationAndClientResponse(t *testing.T) {
	c := newTestClient(t, lspProject(t))
	c.send("initialize", map[string]any{"capabilities": map[string]any{"workspace": map[string]any{"didChangeWatchedFiles": map[string]bool{"dynamicRegistration": true}}}}, 1)
	c.response("1")
	c.send("initialized", map[string]any{}, nil)
	msg := c.until(func(msg testMessage) bool { return msg.Method == "client/registerCapability" })
	if !strings.Contains(string(msg.Params), "**/assets/**") || !strings.Contains(string(msg.Params), "**/*.lua") {
		t.Fatalf("watchers = %s", msg.Params)
	}
	if err := writeFrame(c.in, map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": nil}); err != nil {
		t.Fatal(err)
	}
	c.finish()
}

func TestCompletionReplacesWholeIdentifierAtMiddleCursor(t *testing.T) {
	uri := fileURI(filepath.Join(t.TempDir(), "game.lua"))
	s := &server{documents: map[string]document{uri: {URI: uri, Text: "sspr("}}}
	params, _ := json.Marshal(map[string]any{"textDocument": documentID{uri}, "position": position{0, 2}})
	result, rpcErr, _ := s.help(message{Method: "textDocument/completion", Params: params})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	data, _ := json.Marshal(result)
	var completion struct {
		Items []struct {
			Label    string
			TextEdit struct {
				Range   textRange
				NewText string
			}
		}
	}
	if err := json.Unmarshal(data, &completion); err != nil {
		t.Fatal(err)
	}
	for _, item := range completion.Items {
		if item.Label == "sspr" && item.TextEdit.Range == (textRange{position{0, 0}, position{0, 4}}) && item.TextEdit.NewText == "sspr" {
			return
		}
	}
	t.Fatalf("completion edit = %s", data)
}

func TestHelpIgnoresStringsCommentsAndMembers(t *testing.T) {
	for _, text := range []string{`"spr"`, "-- spr", "object.spr", "object:spr"} {
		index := strings.Index(text, "spr")
		if identifierInCode(text, index, index+3) {
			t.Errorf("builtin in %q", text)
		}
	}
	text := "spr(1,2,3)\n\"unfinished"
	if !identifierInCode(text, 0, 3) {
		t.Fatal("later incomplete edit disabled help")
	}
}

func TestUTF16PositionsAndCRLFEdits(t *testing.T) {
	text := "a😀é\r\nspr()\r\n"
	for _, tc := range []struct {
		at     position
		offset int
	}{{position{0, 1}, 1}, {position{0, 3}, 5}, {position{0, 4}, 7}, {position{1, 0}, 9}, {position{1, 99}, 14}} {
		offset, err := offsetAt(text, tc.at)
		if err != nil || offset != tc.offset {
			t.Fatalf("offsetAt(%+v) = %d / %v", tc.at, offset, err)
		}
	}
	if _, err := offsetAt(text, position{0, 2}); err == nil {
		t.Fatal("split surrogate accepted")
	}
	if got := compilerPosition(text, 1, 8); got != (position{0, 4}) {
		t.Fatalf("byte position = %+v", got)
	}
}

func TestFramingCountsUTF8BytesAndRejectsBadLengths(t *testing.T) {
	var output strings.Builder
	if err := writeFrame(&output, map[string]string{"text": "😀"}); err != nil {
		t.Fatal(err)
	}
	data, err := readFrame(bufio.NewReader(strings.NewReader(output.String())))
	if err != nil || !strings.Contains(string(data), "😀") {
		t.Fatalf("frame = %s / %v", data, err)
	}
	for _, header := range []string{"Content-Length: -1\r\n\r\n", "Content-Length: 999999999\r\n\r\n", "Other: x\r\n\r\n", "Content-Length: 1\r\nContent-Length: 1\r\n\r\nx"} {
		if _, err := readFrame(bufio.NewReader(strings.NewReader(header))); err == nil {
			t.Errorf("accepted %q", header)
		}
	}
}
