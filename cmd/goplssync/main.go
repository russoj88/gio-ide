// Command goplssync copies the gopls v0.23.0 closure into internal/gopls,
// rewrites imports, applies the local patches, and lints the fork.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	switch cmd {
	case "closure":
		gopls := fs.String("gopls", "", "path to the upstream gopls module")
		fs.Parse(os.Args[2:])
		if *gopls == "" {
			fatalf("closure: --gopls is required")
		}
		if err := cmdClosure(*gopls, os.Stdout); err != nil {
			fatalf("closure: %v", err)
		}
	case "copy":
		gopls := fs.String("gopls", "", "path to the upstream gopls module")
		xtools := fs.String("xtools", "", "path to the pinned x/tools module")
		pkgs := fs.String("pkgs", "", "closure list from the closure command")
		dst := fs.String("dst", "", "fork root (internal/gopls)")
		fs.Parse(os.Args[2:])
		if *gopls == "" || *xtools == "" || *pkgs == "" || *dst == "" {
			fatalf("copy: --gopls --xtools --pkgs --dst are required")
		}
		if err := cmdCopy(*gopls, *xtools, *pkgs, *dst); err != nil {
			fatalf("copy: %v", err)
		}
	case "rewrite":
		module := fs.String("module", "github.com/russoj88/gio-ide", "gio-ide module path")
		dir := fs.String("dir", "", "fork root")
		fs.Parse(os.Args[2:])
		if *dir == "" {
			fatalf("rewrite: --dir is required")
		}
		if err := cmdRewrite(*module, *dir); err != nil {
			fatalf("rewrite: %v", err)
		}
	case "patch":
		dir := fs.String("dir", "", "fork root")
		module := fs.String("module", "github.com/russoj88/gio-ide", "gio-ide module path")
		fs.Parse(os.Args[2:])
		if *dir == "" {
			fatalf("patch: --dir is required")
		}
		if err := cmdPatch(*module, *dir); err != nil {
			fatalf("patch: %v", err)
		}
	case "gomod":
		from := fs.String("from", "", "upstream gopls/go.mod")
		fs.Parse(os.Args[2:])
		if *from == "" {
			fatalf("gomod: --from is required")
		}
		if err := cmdGoMod(*from); err != nil {
			fatalf("gomod: %v", err)
		}
	case "lint":
		dir := fs.String("dir", "", "fork root")
		fs.Parse(os.Args[2:])
		if *dir == "" {
			fatalf("lint: --dir is required")
		}
		if err := cmdLint(*dir); err != nil {
			fatalf("lint: %v", err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: goplssync closure|copy|rewrite|patch|gomod|lint [flags]\n")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
