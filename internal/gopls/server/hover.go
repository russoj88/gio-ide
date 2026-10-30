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
	"github.com/russoj88/gio-ide/internal/gopls/settings"
	"github.com/russoj88/gio-ide/internal/gopls/telemetry"
	"github.com/russoj88/gio-ide/internal/gopls/template"
	"github.com/russoj88/gio-ide/internal/gopls/work"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event"
)

func (s *server) Hover(ctx context.Context, params *protocol.HoverParams) (_ *protocol.Hover, rerr error) {
	recordLatency := telemetry.StartLatencyTimer("hover")
	defer func() {
		recordLatency(ctx, rerr)
	}()

	ctx, done := event.Start(ctx, "server.Hover", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()

	switch snapshot.FileKind(fh) {
	case file.Mod:
		return mod.Hover(ctx, snapshot, fh, params.Range)
	case file.Go:
		var pkgURL func(path golang.PackagePath, fragment string) protocol.URI
		if snapshot.Options().LinksInHover == settings.LinksInHover_Gopls {
			web, err := s.getWeb()
			if err != nil {
				event.Error(ctx, "failed to start web server", err)
			} else {
				pkgURL = func(path golang.PackagePath, fragment string) protocol.URI {
					return web.PkgURL(snapshot.View().ID(), path, fragment)
				}
			}
		}
		return golang.Hover(ctx, snapshot, fh, params.Range, pkgURL)
	case file.Tmpl:
		return template.Hover(ctx, snapshot, fh, params.Range)
	case file.Work:
		return work.Hover(ctx, snapshot, fh, params.Range)
	}
	return nil, nil // empty result
}
