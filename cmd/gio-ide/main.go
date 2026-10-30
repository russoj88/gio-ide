// Command gio-ide is a Linux desktop Go code editor.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/russoj88/gio-ide/internal/app"
)

// version is set with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fs := flag.NewFlagSet("gio-ide", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	show := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *show {
		fmt.Println("gio-ide " + version)
		return
	}
	dir := ""
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
		if _, err := os.Stat(dir); err != nil {
			fmt.Fprintf(os.Stderr, "gio-ide: %s\n", err)
			os.Exit(2)
		}
	}
	if err := app.Run(dir, version); err != nil {
		fmt.Fprintln(os.Stderr, "gio-ide:", err)
		os.Exit(1)
	}
}
