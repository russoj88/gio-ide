package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	goplsPrefix  = "golang.org/x/tools/gopls/internal/"
	xtoolsPrefix = "golang.org/x/tools/internal/"
)

// cmdClosure prints "importPath<TAB>dir" for the fork closure.
func cmdClosure(gopls string, w io.Writer) error {
	pkgs, err := listClosure(gopls)
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		fmt.Fprintf(w, "%s\t%s\n", p.path, p.dir)
	}
	return nil
}

type pkgRef struct {
	path string
	dir  string
}

func listClosure(gopls string) ([]pkgRef, error) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}\t{{.Dir}}",
		"./internal/server", "./internal/cache", "./internal/settings", "./internal/protocol")
	cmd.Dir = gopls
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("go list: %w\n%s", err, ee.Stderr)
		}
		return nil, err
	}
	var pkgs []pkgRef
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		path, dir, ok := strings.Cut(line, "\t")
		if !ok || seen[path] {
			continue
		}
		if !strings.HasPrefix(path, goplsPrefix) && !strings.HasPrefix(path, xtoolsPrefix) {
			continue
		}
		seen[path] = true
		pkgs = append(pkgs, pkgRef{path: path, dir: dir})
	}
	return pkgs, sc.Err()
}
