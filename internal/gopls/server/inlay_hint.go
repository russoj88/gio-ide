// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/russoj88/gio-ide/internal/gopls/file"
	"github.com/russoj88/gio-ide/internal/gopls/golang"
	"github.com/russoj88/gio-ide/internal/gopls/label"
	"github.com/russoj88/gio-ide/internal/gopls/mod"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event"
)

func (s *server) InlayHint(ctx context.Context, params *protocol.InlayHintParams) ([]protocol.InlayHint, error) {
	ctx, done := event.Start(ctx, "server.InlayHint", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()

	switch snapshot.FileKind(fh) {
	case file.Mod:
		return mod.InlayHint(ctx, snapshot, fh, params.Range)
	case file.Go:
		return golang.InlayHint(ctx, snapshot, fh, params.Range)
	}
	return nil, nil // empty result
}
