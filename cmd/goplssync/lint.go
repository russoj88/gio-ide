package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func cmdLint(dir string) error {
	var problems []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "patches" || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, b, 0)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: parse: %v", rel, err))
			return nil
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, "golang.org/x/tools/gopls/") || strings.HasPrefix(p, "golang.org/x/tools/internal/") {
				problems = append(problems, fmt.Sprintf("%s: forbidden import %s", rel, p))
			}
		}
		allowFatal := strings.HasSuffix(rel, "settings/analysis.go") || strings.HasSuffix(rel, "settings/staticcheck.go")
		inGuard := rel == "guard/guard.go" || strings.HasPrefix(rel, "guard/")
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.GoStmt:
				if !inGuard {
					problems = append(problems, fmt.Sprintf("%s:%d: unguarded go statement", rel, fset.Position(x.Pos()).Line))
				}
			case *ast.CallExpr:
				sel, ok := x.Fun.(*ast.SelectorExpr)
				if !ok || allowFatal {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if pkg.Name == "os" && sel.Sel.Name == "Exit" {
					problems = append(problems, fmt.Sprintf("%s:%d: os.Exit", rel, fset.Position(x.Pos()).Line))
				}
				if pkg.Name == "log" && strings.HasPrefix(sel.Sel.Name, "Fatal") {
					problems = append(problems, fmt.Sprintf("%s:%d: log.%s", rel, fset.Position(x.Pos()).Line, sel.Sel.Name))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d fork lint problem(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
	fmt.Println("lint-fork: ok")
	return nil
}
