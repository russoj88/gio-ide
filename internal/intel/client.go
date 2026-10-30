package intel

import (
	"context"
	"fmt"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
)

// client is the protocol.ClientCloser gopls calls back into.
type client struct {
	h *host
}

func (c *client) Close() error { return nil }

func (c *client) PublishDiagnostics(_ context.Context, p *protocol.PublishDiagnosticsParams) error {
	if p == nil {
		return nil
	}
	items := make([]bus.Diagnostic, 0, len(p.Diagnostics))
	content := c.h.bytesForURI(p.URI)
	for _, d := range p.Diagnostics {
		item := bus.Diagnostic{
			Start:    int(d.Range.Start.Character),
			End:      int(d.Range.End.Character),
			Line:     int(d.Range.Start.Line) + 1,
			Col:      int(d.Range.Start.Character) + 1,
			Message:  d.Message,
			Severity: int(d.Severity),
			Source:   d.Source,
		}
		if content != nil {
			loc := locationFromRange(p.URI, content, d.Range)
			item.Start = loc.Offset
			item.End = loc.EndOffset
			item.Line = loc.Line
			item.Col = loc.Col
		}
		items = append(items, item)
	}
	c.h.post(bus.DiagnosticsUpdated{URI: string(p.URI), Version: p.Version, Items: items})
	return nil
}

func (c *client) ShowMessage(_ context.Context, p *protocol.ShowMessageParams) error {
	if p != nil {
		c.h.post(bus.Notification{Level: "info", Text: p.Message})
	}
	return nil
}

func (c *client) LogMessage(_ context.Context, p *protocol.LogMessageParams) error {
	if p != nil {
		c.h.logf(p.Message)
	}
	return nil
}

func (c *client) LogTrace(_ context.Context, p *protocol.LogTraceParams) error {
	if p != nil {
		c.h.logf(p.Message)
	}
	return nil
}

func (c *client) Event(_ context.Context, _ *any) error { return nil }

func (c *client) ShowMessageRequest(ctx context.Context, p *protocol.ShowMessageRequestParams) (*protocol.MessageActionItem, error) {
	if p == nil {
		return nil, nil
	}
	actions := make([]string, len(p.Actions))
	for i, a := range p.Actions {
		actions[i] = a.Title
	}
	reply := make(chan int, 1)
	c.h.post(bus.DialogRequest{Title: "Code intelligence", Message: p.Message, Actions: actions, Reply: reply})
	select {
	case i := <-reply:
		if i >= 0 && i < len(p.Actions) {
			item := p.Actions[i]
			return &item, nil
		}
		return nil, nil
	case <-ctx.Done():
		return nil, nil
	}
}

func (c *client) Progress(_ context.Context, p *protocol.ProgressParams) error {
	detail := ""
	if p != nil {
		detail = fmt.Sprint(p.Token)
	}
	st := c.h.State()
	c.h.post(bus.IntelStateChanged{State: st.String(), Detail: detail})
	return nil
}

func (c *client) WorkDoneProgressCreate(_ context.Context, p *protocol.WorkDoneProgressCreateParams) error {
	if p != nil {
		c.h.post(bus.IntelStateChanged{State: c.h.State().String(), Detail: fmt.Sprint(p.Token)})
	}
	return nil
}

func (c *client) RegisterCapability(_ context.Context, p *protocol.RegistrationParams) error {
	if p == nil {
		return nil
	}
	c.h.mu.Lock()
	for _, r := range p.Registrations {
		c.h.watches = append(c.h.watches, r.Method)
	}
	c.h.mu.Unlock()
	return nil
}

func (c *client) UnregisterCapability(_ context.Context, p *protocol.UnregistrationParams) error {
	if p == nil {
		return nil
	}
	drop := map[string]bool{}
	for _, u := range p.Unregisterations {
		drop[u.Method] = true
	}
	c.h.mu.Lock()
	var keep []string
	for _, m := range c.h.watches {
		if !drop[m] {
			keep = append(keep, m)
		}
	}
	c.h.watches = keep
	c.h.mu.Unlock()
	return nil
}

func (c *client) Configuration(_ context.Context, p *protocol.ParamConfiguration) ([]protocol.LSPAny, error) {
	n := 1
	if p != nil && len(p.Items) > 0 {
		n = len(p.Items)
	}
	cfg := c.h.goplsMap()
	out := make([]protocol.LSPAny, n)
	for i := range out {
		out[i] = cfg
	}
	return out, nil
}

func (c *client) WorkspaceFolders(context.Context) ([]protocol.WorkspaceFolder, error) {
	if c.h.root == "" {
		return nil, nil
	}
	uri := protocol.URIFromPath(c.h.root)
	return []protocol.WorkspaceFolder{{URI: string(uri), Name: "gio-ide"}}, nil
}

func (c *client) ApplyEdit(context.Context, *protocol.ApplyWorkspaceEditParams) (*protocol.ApplyWorkspaceEditResult, error) {
	return &protocol.ApplyWorkspaceEditResult{Applied: false, FailureReason: "not supported in v1"}, nil
}

func (c *client) ShowDocument(_ context.Context, p *protocol.ShowDocumentParams) (*protocol.ShowDocumentResult, error) {
	if p != nil {
		c.h.post(bus.OpenFileRequest{Path: protocol.DocumentURI(p.URI).Path(), Select: true})
	}
	return &protocol.ShowDocumentResult{Success: true}, nil
}

func (c *client) CodeLensRefresh(context.Context) error       { return nil }
func (c *client) DiagnosticRefresh(context.Context) error     { return nil }
func (c *client) FoldingRangeRefresh(context.Context) error   { return nil }
func (c *client) InlayHintRefresh(context.Context) error      { return nil }
func (c *client) InlineValueRefresh(context.Context) error    { return nil }
func (c *client) SemanticTokensRefresh(context.Context) error { return nil }
func (c *client) TextDocumentContentRefresh(context.Context, *protocol.TextDocumentContentRefreshParams) error {
	return nil
}
