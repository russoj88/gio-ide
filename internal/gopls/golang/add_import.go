// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package golang

import (
	"context"

	"github.com/russoj88/gio-ide/internal/gopls/cache"
	"github.com/russoj88/gio-ide/internal/gopls/cache/parsego"
	"github.com/russoj88/gio-ide/internal/gopls/file"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/imports"
)

// AddImport adds a single import statement to the given file
func AddImport(ctx context.Context, snapshot *cache.Snapshot, fh file.Handle, importPath string) ([]protocol.TextEdit, error) {
	pgf, err := snapshot.ParseGo(ctx, fh, parsego.Full)
	if err != nil {
		return nil, err
	}
	return ComputeImportFixEdits(snapshot.Options().Local, pgf.Src, &imports.ImportFix{
		StmtInfo: imports.ImportInfo{
			ImportPath: importPath,
		},
		FixType: imports.AddImport,
	})
}
