// Package format runs go fix, goimports, and gofumpt on a Go source buffer.
// gofumpt does not start another process. go fix and goimports run the go command.
package format

import (
	"context"
	"errors"
	"fmt"
	goversion "go/version"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/mod/modfile"

	gofumpt "github.com/russoj88/gio-ide/internal/thirdparty/gofumpt/format"
	"github.com/russoj88/gio-ide/internal/thirdparty/goimports/gocommand"
	goimp "github.com/russoj88/gio-ide/internal/thirdparty/goimports/imports"
)

// ModuleInfo is the language version and module path gofumpt should assume
// for one file.
type ModuleInfo struct {
	// LangVersion is a go/version language version, such as go1.27.
	// It is never empty.
	LangVersion string
	// ModulePath is the module path from the nearest go.mod.
	// It is empty when the file is not in a module.
	ModulePath string
}

// LookupModule reads the go.mod that contains filePath.
// A missing or unreadable go.mod uses the language version of this binary
// and an empty module path.
func LookupModule(filePath string) ModuleInfo {
	info := ModuleInfo{LangVersion: runtimeLang()}
	if filePath == "" {
		return info
	}
	dir := filepath.Dir(filePath)
	for {
		modPath := filepath.Join(dir, "go.mod")
		b, err := os.ReadFile(modPath)
		if err == nil {
			parsed, err := modfile.Parse(modPath, b, nil)
			if err != nil || parsed == nil {
				return info
			}
			if parsed.Go != nil && parsed.Go.Version != "" {
				if lang := goversion.Lang("go" + parsed.Go.Version); lang != "" {
					info.LangVersion = lang
				}
			}
			if parsed.Module != nil {
				info.ModulePath = parsed.Module.Mod.Path
			}
			return info
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return info
		}
		dir = parent
	}
}

// Gofumpt formats src with the embedded gofumpt v0.12.0.
// Extra rules stay off. A parse error is returned.
func Gofumpt(src []byte, info ModuleInfo) ([]byte, error) {
	lang := info.LangVersion
	if !goversion.IsValid(lang) {
		lang = runtimeLang()
	}
	out, err := gofumpt.Source(src, gofumpt.Options{
		LangVersion: lang,
		ModulePath:  info.ModulePath,
	})
	if err != nil {
		return nil, fmt.Errorf("gofumpt: %w", err)
	}
	return out, nil
}

// Goimports formats src and fixes its imports with the embedded goimports
// from golang.org/x/tools v0.50.0. filename is the file's path; its directory
// selects the module. The call stops after 30 seconds. It runs the go command.
func Goimports(ctx context.Context, filename string, src []byte) ([]byte, error) {
	if filename == "" {
		return nil, errors.New("goimports: file path is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := goimp.Process(filename, src, &goimp.Options{
		Env: &goimp.ProcessEnv{
			Context:     ctx,
			GocmdRunner: &gocommand.Runner{},
		},
		Comments:  true,
		TabIndent: true,
		TabWidth:  8,
	})
	if err != nil {
		return nil, fmt.Errorf("goimports: %w", err)
	}
	return out, nil
}

// Hunk is one replacement in an old buffer. Start and End are byte offsets
// of the half-open range [Start, End). Text is the replacement.
type Hunk struct {
	Start, End int
	Text       string
}

// Hunks returns the edits that turn old into neu.
// The hunks are sorted, non-overlapping, and use old-buffer offsets.
// The result is nil when the buffers are equal.
func Hunks(old, neu []byte) []Hunk {
	if string(old) == string(neu) {
		return nil
	}
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string(old), string(neu), false)
	var hunks []Hunk
	off := 0
	for i := 0; i < len(diffs); i++ {
		d := diffs[i]
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			off += len(d.Text)
		case diffmatchpatch.DiffDelete:
			h := Hunk{Start: off, End: off + len(d.Text)}
			off += len(d.Text)
			if i+1 < len(diffs) && diffs[i+1].Type == diffmatchpatch.DiffInsert {
				h.Text = diffs[i+1].Text
				i++
			}
			if h.Start != h.End || h.Text != "" {
				hunks = append(hunks, h)
			}
		case diffmatchpatch.DiffInsert:
			if d.Text != "" {
				hunks = append(hunks, Hunk{Start: off, End: off, Text: d.Text})
			}
		}
	}
	return hunks
}

// MapOffset maps a byte offset in the old buffer through hunks.
// A point inside a replaced span, or on its end, lands at the end of the new text.
// A point on a pure insert lands after the inserted text.
func MapOffset(off int, hunks []Hunk) int {
	shift := 0
	for _, h := range hunks {
		if off < h.Start {
			return off + shift
		}
		if off <= h.End {
			return h.Start + len(h.Text) + shift
		}
		shift += len(h.Text) - (h.End - h.Start)
	}
	return off + shift
}

func runtimeLang() string {
	if lang := goversion.Lang(runtime.Version()); goversion.IsValid(lang) {
		return lang
	}
	return "go1.27"
}
