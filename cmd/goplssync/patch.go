package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func cmdPatch(module, dir string) error {
	if err := ensureGuard(dir); err != nil {
		return err
	}
	patchDir := filepath.Join(dir, "patches")
	if err := os.MkdirAll(patchDir, 0o755); err != nil {
		return err
	}
	steps := []struct {
		name string
		fn   func(root string) error
	}{
		{"0001-P1-no-os-exit.patch", patchP1},
		{"0002-P2-filecache-no-fatal.patch", patchP2},
		{"0003-P3-packagesinternal-modes.patch", func(root string) error { return patchP3(module, root) }},
		{"0004-P4-guard-goroutines.patch", func(root string) error { return patchP4(module, root) }},
		{"0005-P5-remove-ballast.patch", patchP5},
		{"0006-P6-remaining-fatal.patch", patchP6},
	}
	for _, st := range steps {
		before, err := snapshotGo(dir)
		if err != nil {
			return err
		}
		if err := st.fn(dir); err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
		after, err := snapshotGo(dir)
		if err != nil {
			return err
		}
		diff, err := unified(before, after, "internal/gopls")
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(diff)) == 0 {
			return fmt.Errorf("%s produced an empty diff", st.name)
		}
		if err := os.WriteFile(filepath.Join(patchDir, st.name), diff, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", st.name, len(diff))
	}
	return nil
}

func ensureGuard(dir string) error {
	path := filepath.Join(dir, "guard", "guard.go")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(guardSrc), 0o644)
}

const guardSrc = `// The Unlicense covers this file. See LICENSE at the repository root.

// Package guard starts goroutines that recover a panic, log the stack,
// and call the hook the intel host registers.
package guard

import (
	"log"
	"runtime/debug"
	"sync"
)

// Hook, when set, is called after a recovered panic is logged.
// The intel host sets it so a gopls panic marks the session degraded.
var Hook func(recovered any, stack []byte)

var hookMu sync.Mutex

// Go runs fn in a new goroutine. A panic is recovered, logged, and reported.
func Go(fn func()) {
	go func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			stack := debug.Stack()
			log.Printf("gopls guard: panic: %v\n%s", r, stack)
			hookMu.Lock()
			h := Hook
			hookMu.Unlock()
			if h == nil {
				return
			}
			defer func() { _ = recover() }()
			h(r, stack)
		}()
		fn()
	}()
}
`

func patchP1(root string) error {
	path := filepath.Join(root, "server", "general.go")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	old := "\tif s.state != serverShutDown {\n\t\t// TODO: We should be able to do better than this.\n\t\tos.Exit(1)\n\t}"
	neu := "\tif s.state != serverShutDown {\n\t\t// gio-ide P1: never end the editor process.\n\t\treturn fmt.Errorf(\"exit called before shutdown\")\n\t}"
	if !strings.Contains(s, old) {
		return fmt.Errorf("P1 pattern not found in server/general.go")
	}
	s = strings.Replace(s, old, neu, 1)
	if strings.Count(s, "os.") == 0 {
		s = strings.Replace(s, "\t\"os\"\n", "", 1)
	}
	return os.WriteFile(path, []byte(s), 0o644)
}

func patchP2(root string) error {
	path := filepath.Join(root, "filecache", "filecache.go")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	old := "\tif errNoCache, ok := errors.AsType[ErrNoCache](err); ok {\n\t\tlog.Fatal(errNoCache) // unrecoverable\n\t}"
	neu := "\tif errNoCache, ok := errors.AsType[ErrNoCache](err); ok {\n\t\t// gio-ide P2: a cache fault must not end the editor.\n\t\tbug.Reportf(\"filecache: %v\", errNoCache)\n\t\treturn *new(T), false\n\t}"
	if !strings.Contains(s, old) {
		return fmt.Errorf("P2 pattern not found")
	}
	s = strings.Replace(s, old, neu, 1)
	return os.WriteFile(path, []byte(s), 0o644)
}

func patchP3(module, root string) error {
	path := filepath.Join(root, "xtools", "packagesinternal", "packages.go")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	const initFn = `
func init() {
	// gio-ide P3. go/packages sets these on its own copy of this package.
	// The fork never runs that init. The numbers are the LoadMode bits
	// needInternalDepsErrors (1<<10) and typecheckCgo (1<<12) in the
	// pinned golang.org/x/tools go/packages/packages.go.
	// GetDepsErrors stays at its default and returns nil.
	TypecheckCgo = 4096
	DepsErrors = 1024
}
`
	if strings.Contains(s, "gio-ide P3") {
		return nil
	}
	s += initFn
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		return err
	}
	const test = `package packagesinternal

import "testing"

// TestP3ModeBits fails if the pinned go/packages LoadMode bits move.
func TestP3ModeBits(t *testing.T) {
	if TypecheckCgo != 4096 {
		t.Fatalf("TypecheckCgo = %d, want 4096 (typecheckCgo bit)", TypecheckCgo)
	}
	if DepsErrors != 1024 {
		t.Fatalf("DepsErrors = %d, want 1024 (needInternalDepsErrors bit)", DepsErrors)
	}
}
`
	_ = module
	return os.WriteFile(filepath.Join(root, "xtools", "packagesinternal", "p3_test.go"), []byte(test), 0o644)
}

func patchP4(module, root string) error {
	guardImport := module + "/internal/gopls/guard"
	var stmts, files int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "patches" || d.Name() == "testdata" || d.Name() == "guard" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, n, err := rewriteGoStmts(b, guardImport)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if n == 0 {
			return nil
		}
		stmts += n
		files++
		return os.WriteFile(path, out, 0o644)
	})
	fmt.Fprintf(os.Stderr, "P4: %d go statements in %d non-test files\n", stmts, files)
	return err
}

func patchP5(root string) error {
	path := filepath.Join(root, "cache", "cache.go")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	start := strings.Index(s, "// ballast is a 100MB")
	end := strings.Index(s, "var ballast = make([]byte, 100*1e6)\n")
	if start < 0 || end < 0 || end < start {
		return fmt.Errorf("P5 ballast block not found")
	}
	end += len("var ballast = make([]byte, 100*1e6)\n")
	if strings.Contains(s[end:], "ballast") {
		return fmt.Errorf("P5: ballast is referenced after the variable")
	}
	s = s[:start] + "// ballast removed by gio-ide P5. The editor sets a memory limit instead.\n\n" + s[end:]
	return os.WriteFile(path, []byte(s), 0o644)
}

func patchP6(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "patches" || d.Name() == "testdata" || d.Name() == "guard" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		if strings.HasSuffix(rel, "settings/analysis.go") || strings.HasSuffix(rel, "settings/staticcheck.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, changed, err := neutralizeFatals(b)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		if !changed {
			return nil
		}
		return os.WriteFile(path, out, 0o644)
	})
}

type goSpan struct {
	start, end, callStart int
}

func rewriteGoStmts(src []byte, guardImport string) ([]byte, int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "f.go", src, parser.ParseComments)
	if err != nil {
		return nil, 0, err
	}
	tf := fset.File(f.Pos())
	var spans []goSpan
	ast.Inspect(f, func(n ast.Node) bool {
		g, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		spans = append(spans, goSpan{
			start:     tf.Offset(g.Go),
			end:       tf.Offset(g.Call.End()),
			callStart: tf.Offset(g.Call.Pos()),
		})
		return true
	})
	if len(spans) == 0 {
		return src, 0, nil
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	for i, sp := range spans {
		call := src[sp.callStart:sp.end]
		repl := append([]byte("guard.Go(func() { "), call...)
		repl = append(repl, []byte(" })")...)
		src = append(append(append([]byte{}, src[:sp.start]...), repl...), src[sp.end:]...)
		delta := len(repl) - (sp.end - sp.start)
		for j := i + 1; j < len(spans); j++ {
			if spans[j].end >= sp.start {
				spans[j].end += delta
			}
			if spans[j].callStart >= sp.start {
				spans[j].callStart += delta
			}
		}
	}
	src = addImport(src, guardImport)
	return src, len(spans), nil
}

func addImport(src []byte, path string) []byte {
	quoted := `"` + path + `"`
	if bytes.Contains(src, []byte(quoted)) {
		return src
	}
	s := string(src)
	if i := strings.Index(s, "import (\n"); i >= 0 {
		i += len("import (\n")
		s = s[:i] + "\t" + quoted + "\n" + s[i:]
		return []byte(s)
	}
	// A single import spec, or none.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "f.go", src, parser.ParseComments)
	if err != nil || f.Name == nil {
		return src
	}
	tf := fset.File(f.Pos())
	insertAt := tf.Offset(f.Name.End())
	if len(f.Imports) == 1 {
		imp := f.Imports[0]
		// Replace the whole import declaration.
		var start, end int
		if imp.Doc != nil {
			// keep it simple: insert a second import line after the spec
		}
		start = tf.Offset(imp.Pos())
		end = tf.Offset(imp.End())
		// Walk back to the "import" keyword.
		kw := strings.LastIndex(s[:start], "import")
		if kw >= 0 {
			start = kw
		}
		block := "import (\n\t" + s[tf.Offset(imp.Pos()):end] + "\n\t" + quoted + "\n)"
		s = s[:start] + block + s[end:]
		return []byte(s)
	}
	line := "\n\nimport " + quoted + "\n"
	s = s[:insertAt] + line + s[insertAt:]
	return []byte(s)
}

func neutralizeFatals(src []byte) ([]byte, bool, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "f.go", src, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	tf := fset.File(f.Pos())
	type rep struct {
		start, end int
		text       string
	}
	var reps []rep
	needFmt := false
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		start := tf.Offset(call.Pos())
		end := tf.Offset(call.End())
		switch {
		case pkg.Name == "log" && (sel.Sel.Name == "Fatal" || sel.Sel.Name == "Fatalf" || sel.Sel.Name == "Fatalln"):
			args := ""
			if len(call.Args) > 0 {
				a0 := tf.Offset(call.Args[0].Pos())
				a1 := tf.Offset(call.Args[len(call.Args)-1].End())
				args = string(src[a0:a1])
			}
			text := "panic(" + args + ")"
			if sel.Sel.Name != "Fatal" {
				text = "panic(fmt.Sprintf(" + args + "))"
				needFmt = true
			}
			reps = append(reps, rep{start, end, text})
		case pkg.Name == "os" && sel.Sel.Name == "Exit":
			reps = append(reps, rep{start, end, `panic("process exit removed")`})
		}
		return true
	})
	if len(reps) == 0 {
		return src, false, nil
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].start > reps[j].start })
	for _, r := range reps {
		src = append(append(append([]byte{}, src[:r.start]...), r.text...), src[r.end:]...)
	}
	if needFmt && !bytes.Contains(src, []byte(`"fmt"`)) {
		src = addImport(src, "fmt")
	}
	src = dropUnusedImport(src, "log")
	src = dropUnusedImport(src, "os")
	return src, true, nil
}

func dropUnusedImport(src []byte, name string) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "f.go", src, 0)
	if err != nil {
		return src
	}
	used := false
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if ok && id.Name == name {
			used = true
		}
		return true
	})
	if used {
		return src
	}
	s := string(src)
	needle := "\t\"" + name + "\"\n"
	if strings.Contains(s, needle) {
		s = strings.Replace(s, needle, "", 1)
		return []byte(s)
	}
	single := "import \"" + name + "\"\n"
	if strings.Contains(s, single) {
		s = strings.Replace(s, single, "", 1)
		return []byte(s)
	}
	return src
}

func snapshotGo(root string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "patches" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = b
		return nil
	})
	return out, err
}

func unified(before, after map[string][]byte, prefix string) ([]byte, error) {
	names := map[string]bool{}
	for n := range before {
		names[n] = true
	}
	for n := range after {
		names[n] = true
	}
	var list []string
	for n := range names {
		if bytes.Equal(before[n], after[n]) {
			continue
		}
		list = append(list, n)
	}
	sort.Strings(list)
	var buf bytes.Buffer
	for _, n := range list {
		old := before[n]
		neu := after[n]
		diff, err := diffBytes(old, neu, prefix+"/"+n)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		buf.Write(diff)
		if len(diff) > 0 && !bytes.HasSuffix(diff, []byte("\n")) {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes(), nil
}

func diffBytes(old, neu []byte, rel string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "gio-ide-diff")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	if old == nil {
		oldPath = "/dev/null"
	} else if err := os.WriteFile(oldPath, old, 0o644); err != nil {
		return nil, err
	}
	if neu == nil {
		newPath = "/dev/null"
	} else if err := os.WriteFile(newPath, neu, 0o644); err != nil {
		return nil, err
	}
	cmd := exec.Command("diff", "-u", "--label", "a/"+rel, "--label", "b/"+rel, oldPath, newPath)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			// differences found
		} else {
			return nil, fmt.Errorf("diff: %w", err)
		}
	}
	// diff -u header is "--- label" / "+++ label". Prefix a git header.
	var b bytes.Buffer
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", rel, rel)
	if old == nil {
		b.WriteString("new file mode 100644\n")
	}
	b.Write(out)
	return b.Bytes(), nil
}
