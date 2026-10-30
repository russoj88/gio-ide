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
	"github.com/russoj88/gio-ide/internal/gopls/telemetry"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event"
)

func (s *server) Implementation(ctx context.Context, params *protocol.ImplementationParams) (_ []protocol.Location, rerr error) {
	recordLatency := telemetry.StartLatencyTimer("implementation")
	defer func() {
		recordLatency(ctx, rerr)
	}()

	ctx, done := event.Start(ctx, "server.Implementation", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()
	if snapshot.FileKind(fh) != file.Go {
		return nil, nil // empty result
	}
	return golang.Implementation(ctx, snapshot, fh, params.Range)
}
