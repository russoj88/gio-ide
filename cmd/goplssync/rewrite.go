package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdRewrite(module, dir string) error {
	fromGopls := "golang.org/x/tools/gopls/internal/"
	fromX := "golang.org/x/tools/internal/"
	toGopls := module + "/internal/gopls/"
	toX := module + "/internal/gopls/xtools/"
	var n int
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "patches" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(b)
		if !strings.Contains(s, fromGopls) && !strings.Contains(s, fromX) {
			return nil
		}
		// Longer prefix first so gopls/internal is not partially rewritten.
		s = strings.ReplaceAll(s, fromGopls, toGopls)
		s = strings.ReplaceAll(s, fromX, toX)
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			return err
		}
		n++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "rewrote imports in %d files\n", n)
	return nil
}
