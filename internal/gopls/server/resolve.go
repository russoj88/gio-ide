// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/russoj88/gio-ide/internal/gopls/golang"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
)

func (s *server) ResolveCommand(ctx context.Context, param *protocol.ExecuteCommandParams) (*protocol.ExecuteCommandParams, error) {
	return golang.ResolveCommand(ctx, param, s.Options().ClientOptions)
}
