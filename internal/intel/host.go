// Package intel is the in-process gopls host.
// The UI goroutine only enqueues jobs. One sequencer runs them in order.
package intel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/gopls/cache"
	"github.com/russoj88/gio-ide/internal/gopls/guard"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
	"github.com/russoj88/gio-ide/internal/gopls/server"
	"github.com/russoj88/gio-ide/internal/gopls/settings"
	"github.com/russoj88/gio-ide/internal/gopls/util/bug"
	"github.com/russoj88/gio-ide/internal/gopls/version"
)

// State is the code-intelligence session state.
type State int

const (
	Starting State = iota
	Loading
	Ready
	Degraded
	Restarting
	Stopped
)

func (s State) String() string {
	switch s {
	case Starting:
		return "Starting"
	case Loading:
		return "Loading"
	case Ready:
		return "Ready"
	case Degraded:
		return "Degraded"
	case Restarting:
		return "Restarting"
	case Stopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

// RequestID identifies one completion, hover, or navigation request.
type RequestID uint64

// Timeouts are the per-call limits from the [intel] settings.
type Timeouts struct {
	Completion time.Duration
	Hover      time.Duration
	Definition time.Duration
	References time.Duration
	Symbols    time.Duration
	Other      time.Duration
}

// DefaultTimeouts matches the settings file defaults.
func DefaultTimeouts() Timeouts {
	return Timeouts{
		Completion: 1500 * time.Millisecond,
		Hover:      1500 * time.Millisecond,
		Definition: 3 * time.Second,
		References: 10 * time.Second,
		Symbols:    5 * time.Second,
		Other:      30 * time.Second,
	}
}

// Config is how the shell builds a host.
type Config struct {
	Post     func(bus.Event)
	Timeouts Timeouts
	Gopls    map[string]any
	Logf     func(string)
}

// Host is the intel.Host surface from the build spec.
type Host interface {
	Start(root string) error
	State() State
	DidOpen(doc buffer.Document)
	DidChange(docID uint64, version int32, edits []buffer.Edit, after buffer.Snapshot)
	DidSave(docID uint64)
	DidClose(docID uint64)
	FilesChanged(changes []bus.FileChange)
	Completion(docID uint64, version int32, at buffer.Offset) RequestID
	Definition(docID uint64, version int32, at buffer.Offset) RequestID
	References(docID uint64, version int32, at buffer.Offset) RequestID
	Hover(docID uint64, version int32, at buffer.Offset) RequestID
	DocumentSymbols(docID uint64, version int32) RequestID
	WorkspaceSymbols(query string) RequestID
	Cancel(id RequestID)
	Restart() error
	Shutdown() error
	Configure(gopls map[string]any)
}

type openDoc struct {
	id      uint64
	path    string
	uri     protocol.DocumentURI
	version int32
	text    []byte
}

type job struct {
	kind    string
	seq     uint64
	id      RequestID
	version int32
	timeout time.Duration
	run     func(context.Context) error
}

type host struct {
	post     func(bus.Event)
	timeouts Timeouts
	gopls    map[string]any
	logf     func(string)

	mu       sync.Mutex
	cond     *sync.Cond
	q        []job
	closed   bool
	nextSeq  uint64
	nextReq  RequestID
	latest   map[string]uint64
	running  map[string]context.CancelFunc
	byID     map[RequestID]context.CancelFunc
	state    State
	detail   string
	root     string
	docs     map[uint64]*openDoc
	watches  []string
	filePend []bus.FileChange
	fileWait bool

	base   context.Context
	cancel context.CancelFunc
	srv    protocol.Server
	cl     *client
	cache  *cache.Cache
	sess   *cache.Session

	// observe is a test hook called when a job starts, in sequencer order.
	observe func(kind string, version int32)

	// exited is closed when the sequencer returns.
	// Shutdown is the last job, so the close is the stop signal.
	exited chan struct{}
}

// New starts the sequencer. Call Start to attach a project.
func New(cfg Config) Host {
	if cfg.Post == nil {
		cfg.Post = func(bus.Event) {}
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string) {}
	}
	if cfg.Timeouts.Other == 0 {
		cfg.Timeouts = DefaultTimeouts()
	}
	h := &host{
		post:     cfg.Post,
		timeouts: cfg.Timeouts,
		gopls:    cfg.Gopls,
		logf:     cfg.Logf,
		latest:   map[string]uint64{},
		running:  map[string]context.CancelFunc{},
		byID:     map[RequestID]context.CancelFunc{},
		docs:     map[uint64]*openDoc{},
		state:    Stopped,
		exited:   make(chan struct{}),
	}
	h.cond = sync.NewCond(&h.mu)
	h.cl = &client{h: h}
	h.armBugReport()
	go h.loop()
	return h
}

// armBugReport registers one handler. Handle keeps the handler only until
// the next bug, so the handler registers itself again before it reports.
func (h *host) armBugReport() {
	bug.Handle(func(b bug.Bug) {
		h.armBugReport()
		h.logf("gopls bug: " + b.Description)
		h.post(bus.Notification{Level: "warn", Text: "gopls: " + b.Description})
	})
}

func (h *host) State() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

func (h *host) setState(st State, detail string) {
	h.mu.Lock()
	h.state = st
	h.detail = detail
	h.mu.Unlock()
	h.post(bus.IntelStateChanged{State: st.String(), Detail: detail})
}

func (h *host) Start(root string) error {
	h.enqueue(job{kind: "start", run: func(ctx context.Context) error {
		h.setState(Starting, root)
		return h.startSession(ctx, root)
	}})
	return nil
}

func (h *host) Restart() error {
	h.enqueue(job{kind: "restart", run: func(ctx context.Context) error {
		return h.restart(ctx)
	}})
	return nil
}

func (h *host) Configure(gopls map[string]any) {
	h.enqueue(job{kind: "config", run: func(ctx context.Context) error {
		h.mu.Lock()
		h.gopls = gopls
		srv := h.srv
		h.mu.Unlock()
		if srv == nil {
			return nil
		}
		return srv.DidChangeConfiguration(protocol.WithClient(ctx, h.cl), &protocol.DidChangeConfigurationParams{
			Settings: h.goplsMap(),
		})
	}})
}

func (h *host) Shutdown() error {
	h.enqueue(job{kind: "shutdown", run: func(ctx context.Context) error {
		return h.shutdown(ctx)
	}})
	return nil
}

func (h *host) DidOpen(doc buffer.Document) {
	if doc == nil {
		return
	}
	text := append([]byte(nil), doc.Snapshot().Bytes()...)
	id, path, ver := doc.ID(), doc.Path(), doc.Version()
	h.enqueue(job{kind: "open", version: ver, run: func(ctx context.Context) error {
		return h.didOpen(ctx, id, path, ver, text)
	}})
}

func (h *host) DidChange(docID uint64, version int32, edits []buffer.Edit, after buffer.Snapshot) {
	var text []byte
	if after != nil {
		text = append([]byte(nil), after.Bytes()...)
	}
	eds := append([]buffer.Edit(nil), edits...)
	h.enqueue(job{kind: "change", version: version, run: func(ctx context.Context) error {
		return h.didChange(ctx, docID, version, eds, text)
	}})
}

func (h *host) DidSave(docID uint64) {
	h.enqueue(job{kind: "save", run: func(ctx context.Context) error {
		return h.didSave(ctx, docID)
	}})
}

func (h *host) DidClose(docID uint64) {
	h.enqueue(job{kind: "close", run: func(ctx context.Context) error {
		return h.didClose(ctx, docID)
	}})
}

func (h *host) FilesChanged(changes []bus.FileChange) {
	if len(changes) == 0 {
		return
	}
	h.mu.Lock()
	h.filePend = append(h.filePend, changes...)
	if !h.fileWait {
		h.fileWait = true
		time.AfterFunc(100*time.Millisecond, h.flushFiles)
	}
	h.mu.Unlock()
}

func (h *host) flushFiles() {
	h.mu.Lock()
	ch := h.filePend
	h.filePend = nil
	h.fileWait = false
	h.mu.Unlock()
	if len(ch) == 0 {
		return
	}
	h.enqueue(job{kind: "files", run: func(ctx context.Context) error {
		return h.didFiles(ctx, ch)
	}})
}

func (h *host) Completion(docID uint64, version int32, at buffer.Offset) RequestID {
	return h.request("completion", docID, version, at, h.completion)
}

func (h *host) Hover(docID uint64, version int32, at buffer.Offset) RequestID {
	return h.request("hover", docID, version, at, h.hover)
}

func (h *host) Definition(docID uint64, version int32, at buffer.Offset) RequestID {
	return h.request("definition", docID, version, at, h.definition)
}

func (h *host) References(docID uint64, version int32, at buffer.Offset) RequestID {
	return h.request("references", docID, version, at, h.references)
}

func (h *host) DocumentSymbols(docID uint64, version int32) RequestID {
	id := h.allocID()
	h.enqueue(job{kind: "symbols", id: id, version: version, run: func(ctx context.Context) error {
		return h.docSymbols(ctx, id, docID, version)
	}})
	return id
}

func (h *host) WorkspaceSymbols(query string) RequestID {
	id := h.allocID()
	h.enqueue(job{kind: "symbols", id: id, run: func(ctx context.Context) error {
		return h.wsSymbols(ctx, id, query)
	}})
	return id
}

func (h *host) Cancel(id RequestID) {
	h.mu.Lock()
	if c := h.byID[id]; c != nil {
		c()
	}
	h.mu.Unlock()
}

type posFn func(ctx context.Context, id RequestID, doc *openDoc, at buffer.Offset) error

func (h *host) request(kind string, docID uint64, version int32, at buffer.Offset, fn posFn) RequestID {
	id := h.allocID()
	h.enqueue(job{kind: kind, id: id, version: version, run: func(ctx context.Context) error {
		d := h.doc(docID)
		if d == nil {
			return fmt.Errorf("document %d is not open", docID)
		}
		return fn(ctx, id, d, at)
	}})
	return id
}

func (h *host) allocID() RequestID {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextReq++
	return h.nextReq
}

func cancellable(kind string) bool {
	switch kind {
	case "completion", "hover":
		return true
	default:
		return false
	}
}

func (h *host) enqueue(j job) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed && j.kind != "shutdown" {
		return
	}
	h.nextSeq++
	j.seq = h.nextSeq
	if cancellable(j.kind) {
		h.latest[j.kind] = j.seq
		if c := h.running[j.kind]; c != nil {
			c()
		}
	}
	h.q = append(h.q, j)
	h.cond.Signal()
}

func (h *host) loop() {
	defer close(h.exited)
	for {
		h.mu.Lock()
		for len(h.q) == 0 && !h.closed {
			h.cond.Wait()
		}
		if len(h.q) == 0 && h.closed {
			h.mu.Unlock()
			return
		}
		j := h.q[0]
		h.q = h.q[1:]
		if cap(h.q) > 64 && len(h.q) == 0 {
			h.q = nil
		}
		skip := cancellable(j.kind) && h.latest[j.kind] != j.seq
		h.mu.Unlock()
		if skip {
			continue
		}
		h.exec(j)
		if j.kind == "shutdown" {
			h.mu.Lock()
			h.closed = true
			h.mu.Unlock()
			return
		}
	}
}

func (h *host) exec(j job) {
	to := j.timeout
	if to == 0 {
		to = h.timeoutFor(j.kind)
	}
	parent := h.base
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, to)
	h.mu.Lock()
	h.running[j.kind] = cancel
	if j.id != 0 {
		h.byID[j.id] = cancel
	}
	obs := h.observe
	h.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			h.logf(fmt.Sprintf("intel panic: %v\n%s", r, debug.Stack()))
			h.setState(Degraded, fmt.Sprint(r))
			h.post(bus.Notification{Level: "error", Text: "code intelligence hit an error and is degraded"})
		}
		cancel()
		h.mu.Lock()
		delete(h.running, j.kind)
		delete(h.byID, j.id)
		h.mu.Unlock()
	}()
	if obs != nil {
		obs(j.kind, j.version)
	}
	err := j.run(ctx)
	if err == nil {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		h.post(bus.Notification{Level: "warn", Text: j.kind + ": " + err.Error()})
		h.failRequest(j, err)
		return
	}
	h.logf(j.kind + ": " + err.Error())
	h.failRequest(j, err)
}

func (h *host) failRequest(j job, err error) {
	if j.id == 0 {
		return
	}
	msg := err.Error()
	switch j.kind {
	case "completion":
		h.post(bus.CompletionReady{ID: uint64(j.id), Err: msg})
	case "hover":
		h.post(bus.HoverReady{ID: uint64(j.id), Err: msg})
	case "definition", "references":
		h.post(bus.LocationsReady{ID: uint64(j.id), Kind: j.kind, Err: msg})
	case "symbols":
		h.post(bus.SymbolsReady{ID: uint64(j.id), Err: msg})
	}
}

func (h *host) timeoutFor(kind string) time.Duration {
	switch kind {
	case "completion":
		return h.timeouts.Completion
	case "hover":
		return h.timeouts.Hover
	case "definition":
		return h.timeouts.Definition
	case "references":
		return h.timeouts.References
	case "symbols":
		return h.timeouts.Symbols
	default:
		if h.timeouts.Other > 0 {
			return h.timeouts.Other
		}
		return 30 * time.Second
	}
}

func (h *host) doc(id uint64) *openDoc {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.docs[id]
}

func (h *host) putDoc(d *openDoc) {
	h.mu.Lock()
	h.docs[d.id] = d
	h.mu.Unlock()
}

func (h *host) startSession(ctx context.Context, root string) error {
	if h.cancel != nil {
		h.cancel()
	}
	base, cancel := context.WithCancel(context.Background())
	h.base = base
	h.cancel = cancel
	h.root = root
	h.setState(Loading, root)

	version.VersionOverride = "v0.23.0-gio-ide.6"
	guard.Hook = func(recovered any, stack []byte) {
		h.logf(fmt.Sprintf("guard: %v\n%s", recovered, stack))
		h.setState(Degraded, fmt.Sprint(recovered))
	}

	c := cache.New(nil)
	sess := cache.NewSession(base, c)
	opts := settings.DefaultOptions()
	srv := server.New(sess, h.cl, opts)
	callCtx := protocol.WithClient(base, h.cl)
	h.cache = c
	h.sess = sess
	h.srv = srv

	params := &protocol.ParamInitialize{}
	params.ClientInfo = &protocol.ClientInfo{Name: "gio-ide", Version: "v1"}
	params.RootURI = protocol.URIFromPath(root)
	params.WorkspaceFolders = []protocol.WorkspaceFolder{{
		URI:  string(params.RootURI),
		Name: filepath.Base(root),
	}}
	params.Capabilities.Workspace.Configuration = true
	params.Capabilities.TextDocument.Completion.CompletionItem.SnippetSupport = true
	params.Capabilities.Window.WorkDoneProgress = true
	params.InitializationOptions = h.goplsMap()
	if _, err := srv.Initialize(callCtx, params); err != nil {
		h.setState(Degraded, err.Error())
		return err
	}
	if err := srv.Initialized(callCtx, &protocol.InitializedParams{}); err != nil {
		h.setState(Degraded, err.Error())
		return err
	}
	h.setState(Ready, root)
	return nil
}

func (h *host) goplsMap() map[string]any {
	out := map[string]any{}
	for k, v := range h.gopls {
		out[k] = lspValue(v)
	}
	if _, ok := out["staticcheck"]; !ok {
		out["staticcheck"] = false
	}
	return out
}

// lspValue turns TOML slices into the []any shape gopls decodes from JSON.
func lspValue(v any) any {
	s, ok := v.([]string)
	if !ok {
		return v
	}
	a := make([]any, len(s))
	for i, item := range s {
		a[i] = item
	}
	return a
}

func (h *host) restart(ctx context.Context) error {
	h.setState(Restarting, h.root)
	docs := h.snapshotDocs()
	if err := h.shutdown(ctx); err != nil {
		h.logf("shutdown before restart: " + err.Error())
	}
	h.mu.Lock()
	h.closed = false
	h.mu.Unlock()
	if err := h.startSession(ctx, h.root); err != nil {
		return err
	}
	for _, d := range docs {
		if err := h.didOpen(ctx, d.id, d.path, d.version, d.text); err != nil {
			h.logf("reopen: " + err.Error())
		}
	}
	return nil
}

func (h *host) snapshotDocs() []*openDoc {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*openDoc, 0, len(h.docs))
	for _, d := range h.docs {
		cp := *d
		cp.text = append([]byte(nil), d.text...)
		out = append(out, &cp)
	}
	return out
}

func (h *host) shutdown(ctx context.Context) error {
	srv := h.srv
	if srv != nil {
		_ = srv.Shutdown(ctx)
	}
	if h.cancel != nil {
		h.cancel()
	}
	h.srv = nil
	h.sess = nil
	h.cache = nil
	h.mu.Lock()
	h.docs = map[uint64]*openDoc{}
	h.mu.Unlock()
	h.setState(Stopped, "")
	return nil
}

func (h *host) didOpen(ctx context.Context, id uint64, path string, version int32, text []byte) error {
	if h.srv == nil {
		return errors.New("code intelligence is not running")
	}
	uri := protocol.URIFromPath(path)
	d := &openDoc{id: id, path: path, uri: uri, version: version, text: text}
	h.putDoc(d)
	return h.srv.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: protocol.LangGo,
			Version:    version,
			Text:       string(text),
		},
	})
}

func (h *host) didChange(ctx context.Context, id uint64, version int32, edits []buffer.Edit, after []byte) error {
	d := h.doc(id)
	if d == nil || h.srv == nil {
		return nil
	}
	ver := version
	if ver <= d.version {
		ver = d.version + 1
	}
	var changes []protocol.TextDocumentContentChangeEvent
	if len(edits) == 0 {
		changes = []protocol.TextDocumentContentChangeEvent{{Text: string(after)}}
	} else {
		m := protocol.NewMapper(d.uri, d.text)
		for _, e := range edits {
			rng, err := m.OffsetRange(int(e.Start), int(e.End))
			if err != nil {
				changes = []protocol.TextDocumentContentChangeEvent{{Text: string(after)}}
				break
			}
			changes = append(changes, protocol.TextDocumentContentChangeEvent{Range: &rng, Text: e.Text})
		}
	}
	err := h.srv.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			Version:                ver,
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: d.uri},
		},
		ContentChanges: changes,
	})
	if err != nil {
		return err
	}
	// PublishDiagnostics reads d.text on a gopls goroutine.
	h.mu.Lock()
	d.version = ver
	d.text = after
	h.docs[d.id] = d
	h.mu.Unlock()
	return nil
}

func (h *host) didSave(ctx context.Context, id uint64) error {
	d := h.doc(id)
	if d == nil || h.srv == nil {
		return nil
	}
	return h.srv.DidSave(ctx, &protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: d.uri},
	})
}

func (h *host) didClose(ctx context.Context, id uint64) error {
	d := h.doc(id)
	if d == nil || h.srv == nil {
		return nil
	}
	h.mu.Lock()
	delete(h.docs, id)
	h.mu.Unlock()
	return h.srv.DidClose(ctx, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: d.uri},
	})
}

func (h *host) didFiles(ctx context.Context, changes []bus.FileChange) error {
	if h.srv == nil {
		return nil
	}
	var ev []protocol.FileEvent
	for _, c := range changes {
		kind := protocol.Changed
		switch c.Kind {
		case "create":
			kind = protocol.Created
		case "remove":
			kind = protocol.Deleted
		}
		ev = append(ev, protocol.FileEvent{URI: protocol.URIFromPath(c.Path), Type: kind})
	}
	return h.srv.DidChangeWatchedFiles(ctx, &protocol.DidChangeWatchedFilesParams{Changes: ev})
}

func (h *host) pos(d *openDoc, at buffer.Offset) (protocol.Position, error) {
	m := protocol.NewMapper(d.uri, d.text)
	return m.OffsetPosition(int(at))
}

// posParams sets both Position and Range. gopls v0.23.0 reads Range, not Position.
func posParams(uri protocol.DocumentURI, p protocol.Position) protocol.TextDocumentPositionParams {
	return protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position:     p,
		Range:        protocol.Range{Start: p, End: p},
	}
}

func (h *host) completion(ctx context.Context, id RequestID, d *openDoc, at buffer.Offset) error {
	p, err := h.pos(d, at)
	if err != nil {
		return err
	}
	h.logf(fmt.Sprintf("completion %s:%d:%d off=%d text=%q", d.path, p.Line, p.Character, at, snippetAt(d.text, int(at))))
	list, err := h.srv.Completion(ctx, &protocol.CompletionParams{
		Context:                    protocol.CompletionContext{TriggerKind: protocol.Invoked},
		TextDocumentPositionParams: posParams(d.uri, p),
	})
	if err != nil {
		h.post(bus.CompletionReady{ID: uint64(id), DocID: d.id, Version: d.version, At: int(at), Err: err.Error()})
		return err
	}
	var items []bus.CompletionItem
	if list != nil {
		for _, it := range list.Items {
			items = append(items, toCompletion(it))
		}
	}
	h.post(bus.CompletionReady{ID: uint64(id), DocID: d.id, Version: d.version, At: int(at), Items: items})
	return nil
}

func (h *host) hover(ctx context.Context, id RequestID, d *openDoc, at buffer.Offset) error {
	p, err := h.pos(d, at)
	if err != nil {
		return err
	}
	hov, err := h.srv.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: posParams(d.uri, p),
	})
	if err != nil {
		h.post(bus.HoverReady{ID: uint64(id), DocID: d.id, Version: d.version, Err: err.Error()})
		return err
	}
	text := ""
	if hov != nil {
		text = hov.Contents.Value
	}
	h.post(bus.HoverReady{ID: uint64(id), DocID: d.id, Version: d.version, Markdown: text})
	return nil
}

func (h *host) definition(ctx context.Context, id RequestID, d *openDoc, at buffer.Offset) error {
	p, err := h.pos(d, at)
	if err != nil {
		return err
	}
	locs, err := h.srv.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: posParams(d.uri, p),
	})
	if err != nil {
		h.post(bus.LocationsReady{ID: uint64(id), Kind: "definition", DocID: d.id, Err: err.Error()})
		return err
	}
	h.post(bus.LocationsReady{ID: uint64(id), Kind: "definition", DocID: d.id, Locations: h.toLocs(locs)})
	return nil
}

func (h *host) references(ctx context.Context, id RequestID, d *openDoc, at buffer.Offset) error {
	p, err := h.pos(d, at)
	if err != nil {
		return err
	}
	locs, err := h.srv.References(ctx, &protocol.ReferenceParams{
		Context:                    protocol.ReferenceContext{IncludeDeclaration: true},
		TextDocumentPositionParams: posParams(d.uri, p),
	})
	if err != nil {
		h.post(bus.LocationsReady{ID: uint64(id), Kind: "references", DocID: d.id, Err: err.Error()})
		return err
	}
	h.post(bus.LocationsReady{ID: uint64(id), Kind: "references", DocID: d.id, Locations: h.toLocs(locs)})
	return nil
}

func (h *host) toLocs(locs []protocol.Location) []bus.Location {
	var out []bus.Location
	cache := map[protocol.DocumentURI][]byte{}
	for _, l := range locs {
		out = append(out, locationFromRange(l.URI, h.cachedBytes(l.URI, cache), l.Range))
	}
	return out
}

// locationFromRange converts an LSP range to byte columns.
// Click treats Col as a byte column. LSP Character is a UTF-16 count.
// Nil content keeps the UTF-16 column. A later line then has Offset 0.
func locationFromRange(uri protocol.DocumentURI, content []byte, rng protocol.Range) bus.Location {
	loc := bus.Location{
		Path:    uri.Path(),
		URI:     string(uri),
		Line:    int(rng.Start.Line) + 1,
		Col:     int(rng.Start.Character) + 1,
		EndLine: int(rng.End.Line) + 1,
		EndCol:  int(rng.End.Character) + 1,
	}
	if content == nil {
		return loc
	}
	m := protocol.NewMapper(uri, content)
	if off, err := m.PositionOffset(rng.Start); err == nil {
		loc.Offset = off
		loc.Line, loc.Col = m.OffsetLineCol8(off)
	}
	if off, err := m.PositionOffset(rng.End); err == nil {
		loc.EndOffset = off
		loc.EndLine, loc.EndCol = m.OffsetLineCol8(off)
	}
	return loc
}

func (h *host) cachedBytes(uri protocol.DocumentURI, cache map[protocol.DocumentURI][]byte) []byte {
	if b, ok := cache[uri]; ok {
		return b
	}
	b := h.bytesForURI(uri)
	cache[uri] = b
	return b
}

func (h *host) bytesForURI(uri protocol.DocumentURI) []byte {
	path := uri.Path()
	h.mu.Lock()
	for _, d := range h.docs {
		if d.uri == uri || (path != "" && d.path == path) {
			b := append([]byte(nil), d.text...)
			h.mu.Unlock()
			return b
		}
	}
	h.mu.Unlock()
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}

func (h *host) docSymbols(ctx context.Context, id RequestID, docID uint64, version int32) error {
	d := h.doc(docID)
	if d == nil || h.srv == nil {
		return errors.New("document is not open")
	}
	raw, err := h.srv.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: d.uri},
	})
	if err != nil {
		h.post(bus.SymbolsReady{ID: uint64(id), Kind: "document", DocID: docID, Err: err.Error()})
		return err
	}
	var syms []bus.Symbol
	cache := map[protocol.DocumentURI][]byte{}
	for _, item := range raw {
		switch s := item.(type) {
		case protocol.DocumentSymbol:
			flattenDocSymbol(s, "", &syms, d.path, d.text)
		case *protocol.DocumentSymbol:
			flattenDocSymbol(*s, "", &syms, d.path, d.text)
		case protocol.SymbolInformation:
			loc := locationFromRange(s.Location.URI, h.cachedBytes(s.Location.URI, cache), s.Location.Range)
			syms = append(syms, bus.Symbol{
				Name: s.Name, Kind: fmt.Sprint(s.Kind), Path: loc.Path,
				Line: loc.Line, Col: loc.Col,
				Container: s.ContainerName,
			})
		}
	}
	h.post(bus.SymbolsReady{ID: uint64(id), Kind: "document", DocID: docID, Symbols: syms})
	return nil
}

func flattenDocSymbol(s protocol.DocumentSymbol, parent string, out *[]bus.Symbol, path string, text []byte) {
	loc := locationFromRange(protocol.URIFromPath(path), text, s.SelectionRange)
	*out = append(*out, bus.Symbol{
		Name: s.Name, Kind: fmt.Sprint(s.Kind), Path: path,
		Line: loc.Line, Col: loc.Col,
		Container: parent,
	})
	for _, c := range s.Children {
		flattenDocSymbol(c, s.Name, out, path, text)
	}
}

func (h *host) wsSymbols(ctx context.Context, id RequestID, query string) error {
	if h.srv == nil {
		return errors.New("code intelligence is not running")
	}
	raw, err := h.srv.Symbol(ctx, &protocol.WorkspaceSymbolParams{Query: query})
	if err != nil {
		h.post(bus.SymbolsReady{ID: uint64(id), Kind: "workspace", Query: query, Err: err.Error()})
		return err
	}
	var syms []bus.Symbol
	cache := map[protocol.DocumentURI][]byte{}
	for _, s := range raw {
		loc := locationFromRange(s.Location.URI, h.cachedBytes(s.Location.URI, cache), s.Location.Range)
		syms = append(syms, bus.Symbol{
			Name: s.Name, Kind: fmt.Sprint(s.Kind), Path: loc.Path,
			Line: loc.Line, Col: loc.Col,
			Container: s.ContainerName,
		})
	}
	h.post(bus.SymbolsReady{ID: uint64(id), Kind: "workspace", Query: query, Symbols: syms})
	return nil
}

func snippetAt(text []byte, at int) string {
	if at < 0 || at > len(text) {
		return ""
	}
	a := max(at-12, 0)
	b := min(at+16, len(text))
	return string(text[a:b])
}

func toCompletion(it protocol.CompletionItem) bus.CompletionItem {
	insert := it.InsertText
	if insert == "" {
		insert = it.Label
	}
	snippet := it.InsertTextFormat != nil && *it.InsertTextFormat == protocol.SnippetTextFormat
	if it.TextEdit != nil {
		switch e := it.TextEdit.Value.(type) {
		case protocol.TextEdit:
			if e.NewText != "" {
				insert = e.NewText
			}
		case *protocol.TextEdit:
			if e != nil && e.NewText != "" {
				insert = e.NewText
			}
		}
	}
	return bus.CompletionItem{
		Label:   it.Label,
		Detail:  it.Detail,
		Insert:  insert,
		Snippet: snippet,
		Kind:    fmt.Sprint(it.Kind),
	}
}
