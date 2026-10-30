package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// testSubset are the packages that keep _test.go and testdata.
var testSubset = map[string]bool{
	"golang.org/x/tools/gopls/internal/protocol":          true,
	"golang.org/x/tools/gopls/internal/cache":             true,
	"golang.org/x/tools/gopls/internal/golang/completion": true,
}

func cmdCopy(gopls, xtools, pkgsFile, dst string) error {
	pkgs, err := readPkgs(pkgsFile)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("empty closure list %s", pkgsFile)
	}
	if err := resetDst(dst); err != nil {
		return err
	}
	toolsRoot := filepath.Dir(gopls) // .../tools
	if err := copyFile(filepath.Join(toolsRoot, "LICENSE"), filepath.Join(dst, "LICENSE")); err != nil {
		return fmt.Errorf("LICENSE: %w", err)
	}
	if err := copyFile(filepath.Join(toolsRoot, "PATENTS"), filepath.Join(dst, "PATENTS")); err != nil {
		return fmt.Errorf("PATENTS: %w", err)
	}
	pkgs = append(pkgs, testSupportPkgs(gopls, xtools)...)
	var copied int
	for _, p := range pkgs {
		src, err := pkgSource(p, gopls, xtools)
		if err != nil {
			return err
		}
		rel := dstRel(p.path)
		target := filepath.Join(dst, filepath.FromSlash(rel))
		n, err := copyPackage(src, target, testSubset[p.path])
		if err != nil {
			return fmt.Errorf("%s: %w", p.path, err)
		}
		copied += n
		fmt.Fprintf(os.Stderr, "copy %s (%d files)\n", rel, n)
	}
	fmt.Fprintf(os.Stderr, "copied %d files from %d packages\n", copied, len(pkgs))
	return nil
}

// testSupportPkgs are not in the server closure. The cache session tests import them.
func testSupportPkgs(gopls, xtools string) []pkgRef {
	return []pkgRef{
		{path: "golang.org/x/tools/internal/testenv", dir: filepath.Join(xtools, "internal", "testenv")},
		{path: "golang.org/x/tools/internal/proxydir", dir: filepath.Join(xtools, "internal", "proxydir")},
		{path: "golang.org/x/tools/internal/jsonrpc2/servertest", dir: filepath.Join(xtools, "internal", "jsonrpc2", "servertest")},
		{path: "golang.org/x/tools/gopls/internal/test/integration/fake", dir: filepath.Join(gopls, "internal", "test", "integration", "fake")},
	}
}

func readPkgs(path string) ([]pkgRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []pkgRef
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, dir, ok := strings.Cut(line, "\t")
		if !ok {
			path = line
		}
		out = append(out, pkgRef{path: path, dir: dir})
	}
	return out, sc.Err()
}

func resetDst(dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	keep := map[string]bool{
		"LICENSE": true, "PATENTS": true, "NOTICE.md": true, "PATCHES.md": true,
		"UPSTREAM": true, "patches": true, "guard": true,
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if keep[e.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func pkgSource(p pkgRef, gopls, xtools string) (string, error) {
	if p.dir != "" {
		if st, err := os.Stat(p.dir); err == nil && st.IsDir() {
			return p.dir, nil
		}
	}
	switch {
	case strings.HasPrefix(p.path, goplsPrefix):
		return filepath.Join(gopls, "internal", filepath.FromSlash(strings.TrimPrefix(p.path, goplsPrefix))), nil
	case strings.HasPrefix(p.path, xtoolsPrefix):
		return filepath.Join(xtools, "internal", filepath.FromSlash(strings.TrimPrefix(p.path, xtoolsPrefix))), nil
	default:
		return "", fmt.Errorf("not a fork package: %s", p.path)
	}
}

func dstRel(importPath string) string {
	switch {
	case strings.HasPrefix(importPath, goplsPrefix):
		return strings.TrimPrefix(importPath, goplsPrefix)
	case strings.HasPrefix(importPath, xtoolsPrefix):
		return "xtools/" + strings.TrimPrefix(importPath, xtoolsPrefix)
	default:
		return importPath
	}
}

func copyPackage(src, dst string, tests bool) (int, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	var n int
	var goSrc []byte
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			if !tests {
				continue
			}
		} else if strings.HasSuffix(name, ".go") {
			b, err := os.ReadFile(filepath.Join(src, name))
			if err != nil {
				return n, err
			}
			if isIgnoreFile(b) {
				fmt.Fprintf(os.Stderr, "skip ignore %s\n", filepath.Join(src, name))
				continue
			}
			goSrc = append(goSrc, b...)
			goSrc = append(goSrc, '\n')
		} else if !tests {
			// non-Go files are copied only when a go:embed names them, except tests
			continue
		}
		if strings.HasSuffix(name, ".go") || tests {
			if strings.HasSuffix(name, ".go") {
				b, err := os.ReadFile(filepath.Join(src, name))
				if err != nil {
					return n, err
				}
				if isIgnoreFile(b) {
					continue
				}
				if err := os.WriteFile(filepath.Join(dst, name), b, 0o644); err != nil {
					return n, err
				}
				n++
			}
		}
	}
	// Second pass writes Go files already handled. Copy embeds from the Go sources we kept.
	emb, err := copyEmbeds(src, dst, goSrc)
	if err != nil {
		return n, err
	}
	n += emb
	if tests {
		td := filepath.Join(src, "testdata")
		if st, err := os.Stat(td); err == nil && st.IsDir() {
			c, err := copyTree(td, filepath.Join(dst, "testdata"))
			if err != nil {
				return n, err
			}
			n += c
		}
	}
	return n, nil
}

func copyEmbeds(src, dst string, goSrc []byte) (int, error) {
	var n int
	for _, pat := range embedPatterns(goSrc) {
		c, err := copyPattern(src, dst, pat)
		if err != nil {
			return n, fmt.Errorf("embed %s: %w", pat, err)
		}
		n += c
	}
	return n, nil
}

func embedPatterns(src []byte) []string {
	var out []string
	for line := range strings.SplitSeq(string(src), "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "//go:embed") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trim, "//go:embed"))
		out = append(out, splitEmbed(rest)...)
	}
	return out
}

func splitEmbed(s string) []string {
	var out []string
	for s != "" {
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}
		if s[0] == '"' || s[0] == '`' {
			q := s[0]
			i := 1
			for i < len(s) && s[i] != q {
				if s[i] == '\\' && q == '"' && i+1 < len(s) {
					i += 2
					continue
				}
				i++
			}
			if i >= len(s) {
				out = append(out, strings.Trim(s, "\"`"))
				break
			}
			out = append(out, s[1:i])
			s = s[i+1:]
			continue
		}
		i := 0
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	return out
}

func copyPattern(src, dst, pat string) (int, error) {
	if strings.ContainsAny(pat, "*?[") {
		matches, err := fs.Glob(os.DirFS(src), pat)
		if err != nil {
			return 0, err
		}
		var n int
		for _, m := range matches {
			c, err := copyAny(filepath.Join(src, m), filepath.Join(dst, m))
			if err != nil {
				return n, err
			}
			n += c
		}
		return n, nil
	}
	return copyAny(filepath.Join(src, filepath.FromSlash(pat)), filepath.Join(dst, filepath.FromSlash(pat)))
}

func copyAny(src, dst string) (int, error) {
	st, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if st.IsDir() {
		return copyTree(src, dst)
	}
	if err := copyFile(src, dst); err != nil {
		return 0, err
	}
	return 1, nil
}

func copyTree(src, dst string) (int, error) {
	var n int
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

// isIgnoreFile reports files tagged //go:build ignore (generators, not the package).
func isIgnoreFile(src []byte) bool {
	s := string(src)
	if len(s) > 4096 {
		s = s[:4096]
	}
	for line := range strings.SplitSeq(s, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if !strings.HasPrefix(trim, "//") {
			return false
		}
		var rest string
		switch {
		case strings.HasPrefix(trim, "//go:build "):
			rest = strings.TrimPrefix(trim, "//go:build ")
		case strings.HasPrefix(trim, "// +build "):
			rest = strings.TrimPrefix(trim, "// +build ")
		default:
			continue
		}
		for f := range strings.FieldsSeq(rest) {
			f = strings.Trim(f, "()")
			for part := range strings.SplitSeq(f, ",") {
				part = strings.TrimPrefix(part, "!")
				if part == "ignore" {
					return true
				}
			}
			if f == "ignore" {
				return true
			}
		}
	}
	return false
}
