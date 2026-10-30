// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/russoj88/gio-ide/internal/gopls/file"
	"github.com/russoj88/gio-ide/internal/gopls/golang"
	"github.com/russoj88/gio-ide/internal/gopls/label"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
	"github.com/russoj88/gio-ide/internal/gopls/template"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event"
)

func (s *server) DocumentSymbol(ctx context.Context, params *protocol.DocumentSymbolParams) ([]any, error) {
	ctx, done := event.Start(ctx, "server.DocumentSymbol", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()

	var docSymbols []protocol.DocumentSymbol
	switch snapshot.FileKind(fh) {
	case file.Tmpl:
		docSymbols, err = template.DocumentSymbols(snapshot, fh)
	case file.Go:
		docSymbols, err = golang.DocumentSymbols(ctx, snapshot, fh)
	default:
		return nil, nil // empty result
	}
	if err != nil {
		event.Error(ctx, "DocumentSymbols failed", err)
		return nil, nil // empty result
	}
	// Convert the symbols to an interface array.
	// TODO: Remove this once the lsp deprecates SymbolInformation.
	symbols := make([]any, len(docSymbols))
	for i, s := range docSymbols {
		if snapshot.Options().HierarchicalDocumentSymbolSupport {
			symbols[i] = s
			continue
		}
		// If the client does not support hierarchical document symbols, then
		// we need to be backwards compatible for now and return SymbolInformation.
		symbols[i] = protocol.SymbolInformation{
			Name:       s.Name,
			Kind:       s.Kind,
			Deprecated: s.Deprecated,
			Location:   params.TextDocument.URI.Location(s.Range),
		}
	}
	return symbols, nil
}
