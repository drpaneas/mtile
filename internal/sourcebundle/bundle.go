// Package sourcebundle resolves Mtile's compile-time Lua source graph.
//
// Mtile cartridges do not carry a Lua interpreter. A top-level
// require("module") therefore means "compile this project source unit before
// this one", not Lua's runtime package loader. All source units share the
// cartridge global namespace and each module is included at most once.
package sourcebundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/drpaneas/mtile/internal/luaparser"
)

const DefaultEntry = "game.lua"

// Span maps a contiguous range in Source back to one project file. Lines are
// one-based and include the generated source marker on BundleStart.
type Span struct {
	Path         string
	Relative     string
	BundleStart  int
	BundleEnd    int
	ContentStart int
}

// Bundle is the deterministic, dependency-first source passed to the
// transpiler together with enough metadata for editor/build diagnostics.
type Bundle struct {
	Entry  string
	Source string
	Files  []string
	Spans  []Span
}

type resolver struct {
	root     string
	rootReal string
	overlay  map[string]string
	state    map[string]uint8
	stack    []string
	units    []unit
}

type unit struct {
	path     string
	relative string
	source   string
}

type requirement struct {
	module string
	line   int
}

var staticRequireLine = regexp.MustCompile(`^\s*require\s*(?:\(\s*["']([^"']+)["']\s*\)|["']([^"']+)["'])\s*;?\s*(?:--.*)?$`)

// Resolve loads entry and all of its static dependencies from projectDir.
func Resolve(projectDir, entry string) (Bundle, error) {
	return resolve(projectDir, entry, nil)
}

// ResolveWithOverlay resolves the same graph as Resolve while replacing the
// contents of selected existing project-relative files. The editor uses this
// to analyze all open buffers without writing unrelated dirty tabs to disk.
func ResolveWithOverlay(projectDir, entry string, overlay map[string]string) (Bundle, error) {
	return resolve(projectDir, entry, overlay)
}

func resolve(projectDir, entry string, overlay map[string]string) (Bundle, error) {
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return Bundle{}, fmt.Errorf("resolve project directory: %w", err)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Bundle{}, fmt.Errorf("resolve project directory: %w", err)
	}
	if entry == "" {
		entry = DefaultEntry
	}
	entryPath, err := secureProjectPath(root, real, entry)
	if err != nil {
		return Bundle{}, fmt.Errorf("entry %q: %w", entry, err)
	}
	resolvedOverlay := make(map[string]string, len(overlay))
	for relative, source := range overlay {
		path, err := secureProjectPath(root, real, relative)
		if err != nil {
			return Bundle{}, fmt.Errorf("source overlay %q: %w", relative, err)
		}
		resolvedOverlay[path] = source
	}
	r := &resolver{root: root, rootReal: real, overlay: resolvedOverlay, state: make(map[string]uint8)}
	if err := r.visit(entryPath); err != nil {
		return Bundle{}, err
	}

	var source strings.Builder
	bundle := Bundle{Entry: relativeSlash(root, entryPath)}
	if len(r.units) == 1 {
		item := r.units[0]
		bundle.Files = []string{item.relative}
		bundle.Spans = []Span{{Path: item.path, Relative: item.relative, BundleStart: 1, BundleEnd: 1 + strings.Count(item.source, "\n"), ContentStart: 1}}
		bundle.Source = item.source
		return bundle, nil
	}
	line := 1
	for index, item := range r.units {
		if index != 0 {
			source.WriteByte('\n')
			line++
		}
		start := line
		marker := "-- mtile source: " + item.relative + "\n"
		source.WriteString(marker)
		line++
		source.WriteString(item.source)
		line += strings.Count(item.source, "\n")
		bundle.Files = append(bundle.Files, item.relative)
		bundle.Spans = append(bundle.Spans, Span{Path: item.path, Relative: item.relative, BundleStart: start, BundleEnd: line, ContentStart: start + 1})
	}
	bundle.Source = source.String()
	return bundle, nil
}

// Position translates a bundled one-based line back to its source unit.
func (b Bundle) Position(line, column int) (relative string, sourceLine, sourceColumn int, ok bool) {
	for _, span := range b.Spans {
		if line < span.ContentStart || line > span.BundleEnd {
			continue
		}
		return span.Relative, line - span.ContentStart + 1, column, true
	}
	return "", 0, 0, false
}

// RemapError returns an equivalent diagnostic whose location names the
// original module. It handles parser errors structurally and the transpiler's
// established `source:line:column` semantic diagnostics textually.
func (b Bundle) RemapError(err error) error {
	if err == nil {
		return nil
	}
	var parseError *luaparser.Error
	if errors.As(err, &parseError) {
		if relative, line, column, ok := b.Position(parseError.Pos.Line, parseError.Pos.Column); ok {
			copyError := *parseError
			copyError.Pos.Source = relative
			copyError.Pos.Line = line
			copyError.Pos.Column = column
			return &copyError
		}
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(b.Entry) + `:(\d+):(\d+)`)
	message := pattern.ReplaceAllStringFunc(err.Error(), func(location string) string {
		match := pattern.FindStringSubmatch(location)
		if match == nil {
			return location
		}
		line, lineErr := strconv.Atoi(match[1])
		column, columnErr := strconv.Atoi(match[2])
		if lineErr != nil || columnErr != nil {
			return location
		}
		relative, sourceLine, sourceColumn, ok := b.Position(line, column)
		if !ok {
			return location
		}
		return fmt.Sprintf("%s:%d:%d", relative, sourceLine, sourceColumn)
	})
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}

func (r *resolver) visit(path string) error {
	switch r.state[path] {
	case 2:
		return nil
	case 1:
		at := slices.Index(r.stack, path)
		cycle := append([]string(nil), r.stack[max(0, at):]...)
		cycle = append(cycle, path)
		for i := range cycle {
			cycle[i] = relativeSlash(r.root, cycle[i])
		}
		return fmt.Errorf("require cycle in Lua modules: %s", strings.Join(cycle, " -> "))
	}

	source, ok := r.overlay[path]
	if !ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read Lua source %q: %w", relativeSlash(r.root, path), err)
		}
		source = string(data)
	}
	relative := relativeSlash(r.root, path)
	requirements, stripped, err := parseRequirements(path, source)
	if err != nil {
		return err
	}
	r.state[path] = 1
	r.stack = append(r.stack, path)
	for _, req := range requirements {
		dependency, err := r.resolveModule(path, req.module)
		if err != nil {
			return fmt.Errorf("%s:%d: require %q: %w", relative, req.line, req.module, err)
		}
		if err := r.visit(dependency); err != nil {
			return err
		}
	}
	r.stack = r.stack[:len(r.stack)-1]
	r.state[path] = 2
	r.units = append(r.units, unit{path: path, relative: relative, source: stripped})
	return nil
}

func parseRequirements(path, source string) ([]requirement, string, error) {
	chunk, err := luaparser.ParseString(path, source)
	if err != nil {
		return nil, "", err
	}
	topLevel := make(map[int]requirement)
	for _, statement := range chunk.Body {
		callStatement, ok := statement.(*luaparser.CallStmt)
		if !ok || callStatement.Call == nil {
			continue
		}
		callee, ok := callStatement.Call.Callee.(*luaparser.VarExpr)
		if !ok || callee.Name != "require" {
			continue
		}
		if len(callStatement.Call.Args) != 1 {
			return nil, "", fmt.Errorf("%s: require expects one static module name", callStatement.Pos())
		}
		name, ok := callStatement.Call.Args[0].(*luaparser.StringExpr)
		if !ok || name.Value == "" {
			return nil, "", fmt.Errorf("%s: require expects one non-empty string literal", callStatement.Pos())
		}
		if callStatement.Call.LastPos().Line != callStatement.Pos().Line {
			return nil, "", fmt.Errorf("%s: compile-time require must be written on one line", callStatement.Pos())
		}
		topLevel[callStatement.Pos().Line] = requirement{module: name.Value, line: callStatement.Pos().Line}
	}

	lines := strings.SplitAfter(source, "\n")
	var requirements []requirement
	for index, line := range lines {
		plain := strings.TrimSuffix(line, "\n")
		plain = strings.TrimSuffix(plain, "\r")
		match := staticRequireLine.FindStringSubmatch(plain)
		if match == nil {
			continue
		}
		lineNumber := index + 1
		req, ok := topLevel[lineNumber]
		if !ok {
			return nil, "", fmt.Errorf("%s:%d: require is compile-time only and must be a top-level statement", path, lineNumber)
		}
		spelling := match[1]
		if spelling == "" {
			spelling = match[2]
		}
		if spelling != req.module {
			return nil, "", fmt.Errorf("%s:%d: could not resolve require spelling", path, lineNumber)
		}
		requirements = append(requirements, req)
		if strings.HasSuffix(line, "\n") {
			lines[index] = "\n"
		} else {
			lines[index] = ""
		}
		delete(topLevel, lineNumber)
	}
	if len(topLevel) != 0 {
		for line := range topLevel {
			return nil, "", fmt.Errorf("%s:%d: compile-time require must occupy its own line", path, line)
		}
	}
	return requirements, strings.Join(lines, ""), nil
}

func (r *resolver) resolveModule(requiringPath, module string) (string, error) {
	modulePath, err := cleanModulePath(module)
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(filepath.Dir(requiringPath), modulePath),
		filepath.Join(r.root, "src", modulePath),
	}
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		secure, err := secureExistingPath(r.root, r.rootReal, candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		info, err := os.Stat(secure)
		if err == nil && !info.IsDir() {
			return secure, nil
		}
	}
	return "", fmt.Errorf("module was not found relative to %s or src/", relativeSlash(r.root, requiringPath))
}

func cleanModulePath(module string) (string, error) {
	if strings.TrimSpace(module) != module || module == "" || strings.Contains(module, "\\") {
		return "", fmt.Errorf("invalid module name")
	}
	stem := strings.TrimSuffix(module, ".lua")
	path := strings.ReplaceAll(stem, ".", "/")
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", fmt.Errorf("absolute module paths are forbidden")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("module path escapes the project")
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid module name")
		}
	}
	if filepath.Ext(clean) != "" {
		return "", fmt.Errorf("module components may not contain file extensions")
	}
	clean += ".lua"
	return clean, nil
}

func secureProjectPath(root, rootReal, relative string) (string, error) {
	if filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", fmt.Errorf("absolute paths are forbidden")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the project")
	}
	return secureExistingPath(root, rootReal, filepath.Join(root, clean))
}

func secureExistingPath(root, rootReal, candidate string) (string, error) {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the project")
	}
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	realRelative, err := filepath.Rel(rootReal, real)
	if err != nil || realRelative == ".." || strings.HasPrefix(realRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("symlink escapes the project")
	}
	return filepath.Clean(candidate), nil
}

func relativeSlash(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
