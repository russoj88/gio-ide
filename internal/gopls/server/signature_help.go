// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/russoj88/gio-ide/internal/gopls/file"
	"github.com/russoj88/gio-ide/internal/gopls/golang"
	"github.com/russoj88/gio-ide/internal/gopls/label"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event"
)

func (s *server) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	ctx, done := event.Start(ctx, "server.SignatureHelp", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()

	if snapshot.FileKind(fh) != file.Go {
		return nil, nil // empty result
	}

	info, err := golang.SignatureHelp(ctx, snapshot, fh, params.Range, params.Context)
	if err != nil {
		// TODO(rfindley): is this correct? Apparently, returning an error from
		// signatureHelp is distracting in some editors, though I haven't confirmed
		// that recently.
		//
		// It's unclear whether we still need to avoid returning this error result.
		event.Error(ctx, "signature help failed", err, label.Position.Of(params.Range.Start), label.Position.Of(params.Range.End))
		return nil, nil
	}
	if info == nil {
		return nil, nil
	}
	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{*info},
		ActiveSignature: 0,
		ActiveParameter: info.ActiveParameter,
	}, nil
}
