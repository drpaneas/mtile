package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/drpaneas/mtile/internal/build"
	"github.com/drpaneas/mtile/internal/language"
)

type document struct {
	URI, Path, Text string
	Version         int
}

type server struct {
	out                              io.Writer
	root                             string
	documents                        map[string]document
	published                        map[string]bool
	initialized, ready, shutdown     bool
	watchRegistration, registerWatch bool
	revision                         uint64
	timer                            *time.Timer
	due, busy                        bool
	results                          chan analysisResult
	check                            projectCheck
}

type projectCheck func(string, uint64, map[string]document) analysisResult

type analysisResult struct {
	revision    uint64
	documents   map[string]document
	diagnostics map[string][]editorDiagnostic
}

type editorDiagnostic struct {
	Range    textRange                `json:"range"`
	Severity build.DiagnosticSeverity `json:"severity"`
	Code     string                   `json:"code"`
	Source   string                   `json:"source"`
	Message  string                   `json:"message"`
}

// Serve owns a single project and writes only framed JSON-RPC to out. Source
// analysis runs off the protocol loop, so incomplete edits never block help.
// The caller owns in and must close it to release a blocked reader on cancel.
func Serve(ctx context.Context, in io.Reader, out io.Writer, projectDir string) error {
	return serve(ctx, in, out, projectDir, analyze)
}

func serve(ctx context.Context, in io.Reader, out io.Writer, projectDir string, check projectCheck) error {
	if projectDir != "" {
		var err error
		projectDir, err = filepath.Abs(projectDir)
		if err != nil {
			return err
		}
		if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
			return fmt.Errorf("LSP project directory is unavailable: %s", projectDir)
		}
	}
	s := &server{out: out, root: projectDir, documents: make(map[string]document), published: make(map[string]bool), results: make(chan analysisResult, 1), check: check}
	s.timer = time.NewTimer(time.Hour)
	s.timer.Stop()
	defer s.timer.Stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type incoming struct {
		data []byte
		err  error
	}
	messages := make(chan incoming)
	go func() {
		reader := bufio.NewReader(in)
		for {
			data, err := readFrame(reader)
			select {
			case messages <- incoming{data, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case incoming := <-messages:
			if incoming.err != nil {
				if errors.Is(incoming.err, io.EOF) {
					return nil
				}
				return incoming.err
			}
			var msg message
			if err := json.Unmarshal(incoming.data, &msg); err != nil {
				if err := s.reply(json.RawMessage("null"), nil, &rpcError{-32700, "invalid JSON"}); err != nil {
					return err
				}
				continue
			}
			if msg.Method == "" && len(msg.ID) > 0 && msg.JSONRPC == "2.0" {
				continue // Responses to optional client capability registration.
			}
			if msg.JSONRPC != "2.0" || msg.Method == "" {
				if err := s.reply(msg.ID, nil, &rpcError{-32600, "invalid JSON-RPC request"}); err != nil {
					return err
				}
				continue
			}
			previousRevision := s.revision
			result, rpcErr, exit := s.handle(msg)
			if exit {
				if !s.shutdown {
					return errors.New("LSP exit received before shutdown")
				}
				return nil
			}
			if previousRevision != s.revision {
				if err := s.clearDiagnostics(); err != nil {
					return err
				}
			}
			if s.registerWatch {
				s.registerWatch = false
				registration := map[string]any{"jsonrpc": "2.0", "id": "mtile/watch", "method": "client/registerCapability", "params": map[string]any{
					"registrations": []any{map[string]any{"id": "mtile/project-files", "method": "workspace/didChangeWatchedFiles", "registerOptions": map[string]any{
						"watchers": []any{map[string]string{"globPattern": "**/*.lua"}, map[string]string{"globPattern": "**/project.json"}, map[string]string{"globPattern": "**/assets/**"}},
					}}},
				}}
				if err := writeFrame(s.out, registration); err != nil {
					return err
				}
			}
			if len(msg.ID) != 0 {
				if err := s.reply(msg.ID, result, rpcErr); err != nil {
					return err
				}
			}
			if s.due && !s.busy {
				s.startAnalysis(ctx)
			}
		case <-s.timer.C:
			s.due = true
			if !s.busy {
				s.startAnalysis(ctx)
			}
		case result := <-s.results:
			s.busy = false
			if result.revision == s.revision && !s.shutdown {
				if err := s.publish(result); err != nil {
					return err
				}
			}
			if s.due {
				s.startAnalysis(ctx)
			}
		}
	}
}

func (s *server) reply(id json.RawMessage, result any, err *rpcError) error {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	response := map[string]any{"jsonrpc": "2.0", "id": id}
	if err != nil {
		response["error"] = err
	} else {
		response["result"] = result
	}
	return writeFrame(s.out, response)
}

func (s *server) notify(method string, params any) error {
	return writeFrame(s.out, map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func invalidParams() *rpcError { return &rpcError{-32602, "invalid parameters"} }

func (s *server) handle(msg message) (any, *rpcError, bool) {
	if msg.Method == "exit" {
		return nil, nil, true
	}
	if s.shutdown {
		return nil, &rpcError{-32600, "server has shut down"}, false
	}
	if msg.Method == "initialize" {
		if s.initialized {
			return nil, &rpcError{-32600, "already initialized"}, false
		}
		var params struct {
			RootURI          string `json:"rootUri"`
			RootPath         string `json:"rootPath"`
			WorkspaceFolders []struct {
				URI string `json:"uri"`
			} `json:"workspaceFolders"`
			Capabilities struct {
				Workspace struct {
					DidChangeWatchedFiles struct {
						DynamicRegistration bool `json:"dynamicRegistration"`
					} `json:"didChangeWatchedFiles"`
				} `json:"workspace"`
			} `json:"capabilities"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, invalidParams(), false
		}
		if s.root == "" {
			uri := params.RootURI
			if len(params.WorkspaceFolders) > 0 {
				uri = params.WorkspaceFolders[0].URI
			}
			if uri != "" {
				var err error
				s.root, err = filePath(uri)
				if err != nil {
					return nil, invalidParams(), false
				}
			} else if params.RootPath != "" {
				var err error
				s.root, err = filepath.Abs(params.RootPath)
				if err != nil {
					return nil, invalidParams(), false
				}
			}
		}
		s.initialized = true
		s.watchRegistration = params.Capabilities.Workspace.DidChangeWatchedFiles.DynamicRegistration
		return map[string]any{
			"capabilities": map[string]any{
				"positionEncoding":   "utf-16",
				"textDocumentSync":   map[string]any{"openClose": true, "change": 2, "save": map[string]any{"includeText": true}},
				"completionProvider": map[string]any{"resolveProvider": false},
				"hoverProvider":      true,
			},
			"serverInfo": map[string]string{"name": "mtile"},
		}, nil, false
	}
	if !s.initialized {
		return nil, &rpcError{-32002, "server is not initialized"}, false
	}
	switch msg.Method {
	case "initialized":
		s.ready = true
		s.registerWatch = s.watchRegistration
		s.schedule()
	case "shutdown":
		s.shutdown = true
		s.revision++
		s.due = false
		s.timer.Stop()
	case "textDocument/didOpen":
		var params struct {
			TextDocument struct {
				URI, Text string
				Version   int
			} `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, invalidParams(), false
		}
		path, err := filePath(params.TextDocument.URI)
		if err != nil || filepath.Ext(path) != ".lua" {
			return nil, invalidParams(), false
		}
		if s.root == "" {
			s.root = findRoot(filepath.Dir(path))
		}
		if _, ok := relativePath(s.root, path); !ok {
			return nil, invalidParams(), false
		}
		s.documents[params.TextDocument.URI] = document{params.TextDocument.URI, path, params.TextDocument.Text, params.TextDocument.Version}
		s.schedule()
	case "textDocument/didChange":
		var params struct {
			TextDocument struct {
				URI     string
				Version int
			} `json:"textDocument"`
			ContentChanges []struct {
				Range *textRange `json:"range"`
				Text  string     `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, invalidParams(), false
		}
		doc, ok := s.documents[params.TextDocument.URI]
		if !ok || params.TextDocument.Version <= doc.Version {
			return nil, invalidParams(), false
		}
		for _, change := range params.ContentChanges {
			if change.Range == nil {
				doc.Text = change.Text
				continue
			}
			start, err1 := offsetAt(doc.Text, change.Range.Start)
			end, err2 := offsetAt(doc.Text, change.Range.End)
			if err1 != nil || err2 != nil || start > end {
				return nil, invalidParams(), false
			}
			doc.Text = doc.Text[:start] + change.Text + doc.Text[end:]
		}
		doc.Version = params.TextDocument.Version
		s.documents[doc.URI] = doc
		s.schedule()
	case "textDocument/didClose":
		var params struct {
			TextDocument documentID `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, invalidParams(), false
		}
		delete(s.documents, params.TextDocument.URI)
		s.schedule()
	case "textDocument/didSave":
		var params struct {
			TextDocument documentID `json:"textDocument"`
			Text         *string    `json:"text"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, invalidParams(), false
		}
		if doc, ok := s.documents[params.TextDocument.URI]; ok && params.Text != nil {
			doc.Text = *params.Text
			s.documents[doc.URI] = doc
		}
		s.schedule()
	case "workspace/didChangeWatchedFiles", "workspace/didChangeConfiguration":
		s.schedule()
	case "textDocument/completion", "textDocument/hover":
		return s.help(msg)
	case "$/cancelRequest", "$/setTrace":
	default:
		return nil, &rpcError{-32601, "method not found: " + msg.Method}, false
	}
	return nil, nil, false
}

func findRoot(dir string) string {
	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "project.json")); err == nil {
			return current
		}
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	return dir
}

func (s *server) help(msg message) (any, *rpcError, bool) {
	var params struct {
		TextDocument documentID `json:"textDocument"`
		Position     position   `json:"position"`
	}
	if json.Unmarshal(msg.Params, &params) != nil {
		return nil, invalidParams(), false
	}
	doc, ok := s.documents[params.TextDocument.URI]
	if !ok {
		return nil, invalidParams(), false
	}
	offset, err := offsetAt(doc.Text, params.Position)
	if err != nil {
		return nil, invalidParams(), false
	}
	if msg.Method == "textDocument/hover" {
		word, span := wordAt(doc.Text, offset)
		start, _ := offsetAt(doc.Text, span.Start)
		end, _ := offsetAt(doc.Text, span.End)
		if !identifierInCode(doc.Text, start, end) {
			return nil, nil, false
		}
		entry, ok := language.Lookup(word)
		if !ok {
			return nil, nil, false
		}
		return map[string]any{"contents": map[string]string{"kind": "markdown", "value": entry.Markdown()}, "range": span}, nil, false
	}
	start := offset
	for start > 0 && identifierByte(doc.Text[start-1]) {
		start--
	}
	items := []any{}
	_, span := wordAt(doc.Text, offset)
	if !identifierInCode(doc.Text, start, offset) {
		return map[string]any{"isIncomplete": false, "items": items}, nil, false
	}
	for _, entry := range language.Complete(doc.Text[start:offset]) {
		items = append(items, map[string]any{
			"label": entry.Name, "kind": 3, "detail": entry.Signature,
			"documentation": map[string]string{"kind": "markdown", "value": entry.Markdown()},
			"textEdit":      map[string]any{"range": span, "newText": entry.Name},
		})
	}
	return map[string]any{"isIncomplete": false, "items": items}, nil, false
}

func (s *server) schedule() {
	s.revision++
	s.due = false
	if s.ready && s.root != "" {
		s.timer.Reset(200 * time.Millisecond)
	}
}

func (s *server) startAnalysis(ctx context.Context) {
	if s.shutdown || s.root == "" {
		s.due = false
		return
	}
	s.busy, s.due = true, false
	root, revision := s.root, s.revision
	documents := make(map[string]document, len(s.documents))
	for uri, doc := range s.documents {
		documents[uri] = doc
	}
	go func() {
		result := s.check(root, revision, documents)
		select {
		case s.results <- result:
		case <-ctx.Done():
		}
	}()
}

func analyze(root string, revision uint64, documents map[string]document) analysisResult {
	overlay := make(map[string]string, len(documents))
	for _, doc := range documents {
		if relative, ok := relativePath(root, doc.Path); ok {
			overlay[relative] = doc.Text
		}
	}
	analysis := (build.Service{UseProjectRenderer: true}).CheckProject(root, overlay)
	result := analysisResult{revision, documents, make(map[string][]editorDiagnostic)}
	for _, doc := range documents {
		result.diagnostics[doc.URI] = []editorDiagnostic{}
	}
	// Syntax feedback also covers open Lua files not required by the entry.
	for source, text := range overlay {
		for _, diagnostic := range build.CheckLuaSource(source, text) {
			duplicate := false
			for _, existing := range analysis.Diagnostics {
				duplicate = duplicate || existing.Source == diagnostic.Source && existing.Line == diagnostic.Line && existing.Column == diagnostic.Column && existing.Code == diagnostic.Code
			}
			if !duplicate {
				analysis.Diagnostics = append(analysis.Diagnostics, diagnostic)
			}
		}
	}
	for _, diagnostic := range analysis.Diagnostics {
		path := filepath.Join(root, filepath.FromSlash(diagnostic.Source))
		uri := fileURI(path)
		for _, doc := range documents {
			if doc.Path == path {
				uri = doc.URI
				break
			}
		}
		text, ok := overlay[diagnostic.Source]
		if !ok {
			data, _ := os.ReadFile(path)
			text = string(data)
		}
		result.diagnostics[uri] = append(result.diagnostics[uri], editorDiagnostic{
			Range:    textRange{compilerPosition(text, diagnostic.Line, diagnostic.Column), compilerPosition(text, diagnostic.EndLine, diagnostic.EndColumn)},
			Severity: diagnostic.Severity, Code: diagnostic.Code, Source: "mtile", Message: diagnostic.Message,
		})
	}
	return result
}

func (s *server) publish(result analysisResult) error {
	for uri := range s.published {
		if _, ok := result.diagnostics[uri]; !ok {
			result.diagnostics[uri] = []editorDiagnostic{}
		}
	}
	s.published = make(map[string]bool, len(result.diagnostics))
	for uri, diagnostics := range result.diagnostics {
		params := map[string]any{"uri": uri, "diagnostics": diagnostics}
		if doc, ok := result.documents[uri]; ok {
			params["version"] = doc.Version
		}
		if err := s.notify("textDocument/publishDiagnostics", params); err != nil {
			return err
		}
		if len(diagnostics) > 0 {
			s.published[uri] = true
		}
	}
	return nil
}

func (s *server) clearDiagnostics() error {
	for uri := range s.published {
		params := map[string]any{"uri": uri, "diagnostics": []editorDiagnostic{}}
		if doc, ok := s.documents[uri]; ok {
			params["version"] = doc.Version
		}
		if err := s.notify("textDocument/publishDiagnostics", params); err != nil {
			return err
		}
	}
	s.published = make(map[string]bool)
	return nil
}
