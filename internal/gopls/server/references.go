// Copyright 2019 The Go Authors. All rights reserved.
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
	"github.com/russoj88/gio-ide/internal/gopls/telemetry"
	"github.com/russoj88/gio-ide/internal/gopls/template"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event"
)

func (s *server) References(ctx context.Context, params *protocol.ReferenceParams) (_ []protocol.Location, rerr error) {
	recordLatency := telemetry.StartLatencyTimer("references")
	defer func() {
		recordLatency(ctx, rerr)
	}()

	ctx, done := event.Start(ctx, "server.References", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()
	switch snapshot.FileKind(fh) {
	case file.Tmpl:
		return template.References(ctx, snapshot, fh, params)
	case file.Go:
		return golang.References(ctx, snapshot, fh, params.Range, params.Context.IncludeDeclaration)
	case file.Mod:
		return mod.References(ctx, snapshot, fh, params)
	}
	return nil, nil // empty result
}
