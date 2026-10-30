// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/russoj88/gio-ide/internal/gopls/file"
	"github.com/russoj88/gio-ide/internal/gopls/golang"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
	"github.com/russoj88/gio-ide/internal/gopls/xtools/event"
)

func (s *server) PrepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	ctx, done := event.Start(ctx, "server.PrepareCallHierarchy")
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()
	switch snapshot.FileKind(fh) {
	case file.Go:
		return golang.PrepareCallHierarchy(ctx, snapshot, fh, params.Range)
	}
	return nil, nil // empty result
}

func (s *server) IncomingCalls(ctx context.Context, params *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	ctx, done := event.Start(ctx, "server.IncomingCalls")
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.Item.URI)
	if err != nil {
		return nil, err
	}
	defer release()
	switch snapshot.FileKind(fh) {
	case file.Go:
		return golang.IncomingCalls(ctx, snapshot, fh, params.Item.Range)
	}
	return nil, nil // empty result
}

func (s *server) OutgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	ctx, done := event.Start(ctx, "server.OutgoingCalls")
	defer done()

	fh, snapshot, release, err := s.session.FileOf(ctx, params.Item.URI)
	if err != nil {
		return nil, err
	}
	defer release()
	switch snapshot.FileKind(fh) {
	case file.Go:
		return golang.OutgoingCalls(ctx, snapshot, fh, params.Item.Range.Start)
	}
	return nil, nil // empty result
}
