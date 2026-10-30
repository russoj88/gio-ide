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

func (s *server) DocumentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	ctx, done := event.Start(ctx, "server.DocumentHighlight", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()

	switch snapshot.FileKind(fh) {
	case file.Tmpl:
		return template.Highlight(ctx, snapshot, fh, params.Range)
	case file.Go:
		rngs, err := golang.Highlight(ctx, snapshot, fh, params.Range)
		if err != nil {
			event.Error(ctx, "no highlight", err)
		}
		return rngs, nil
	}
	return nil, nil // empty result
}
