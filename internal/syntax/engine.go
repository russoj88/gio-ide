// Package syntax colors buffers and computes fold ranges. Go uses tree-sitter.
// go.mod, go.sum, YAML, JSON, Dockerfiles, env files, JavaScript, HTML, and
// SQL use a scanner. Parsers stay on the syntax worker. The UI goroutine
// only enqueues jobs.
package syntax

import (
	_ "embed"
	"sync"
	"sync/atomic"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
)

//go:embed queries/highlights.scm
var highlightSCM string

// StyleSpan is one colored range inside a line.
type StyleSpan struct {
	Start, End int
	Style      string
}

// Engine is the syntax service. Close releases every tree, parser, and cursor.
type Engine interface {
	Open(docID uint64, lang string, s buffer.Snapshot)
	Edit(docID uint64, e buffer.Edit, after buffer.Snapshot)
	Close(docID uint64)
	RequestSpans(docID uint64, version int32, firstLine, lastLine int)
	RequestBracket(docID uint64, version int32, at buffer.Offset)
	Flush()
	Shutdown()
}

// Live counts trees, parsers, and cursors still open. Tests require zero after Shutdown.
func Live() int64 {
	return liveTrees.Load() + liveParsers.Load() + liveCursors.Load()
}

var (
	liveTrees   atomic.Int64
	liveParsers atomic.Int64
	liveCursors atomic.Int64
)

type kind int

const (
	kindOpen kind = iota
	kindEdit
	kindClose
	kindSpans
	kindBracket
	kindFlush
	kindStop
)

type job struct {
	kind    kind
	id      uint64
	lang    string
	text    []byte
	edit    buffer.Edit
	version int32
	first   int
	last    int
	at      buffer.Offset
	done    chan struct{}
}

type docTree struct {
	tree    *sitter.Tree
	text    []byte
	version int32
	lang    string
}

// New starts one syntax worker. post delivers results to the bus and may be nil in tests.
func New(post func(bus.Event)) Engine {
	if post == nil {
		post = func(bus.Event) {}
	}
	e := &eng{
		post: post,
		jobs: make(chan job, 1024),
		docs: map[uint64]*docTree{},
	}
	go e.loop()
	return e
}

type eng struct {
	post func(bus.Event)
	jobs chan job

	mu   sync.Mutex
	docs map[uint64]*docTree

	parser *sitter.Parser
	lang   *sitter.Language
	query  *sitter.Query
	names  []string
}

func (e *eng) Open(docID uint64, lang string, s buffer.Snapshot) {
	text := append([]byte(nil), s.Bytes()...)
	e.jobs <- job{kind: kindOpen, id: docID, lang: lang, text: text, version: s.Version()}
}

func (e *eng) Edit(docID uint64, ed buffer.Edit, after buffer.Snapshot) {
	text := append([]byte(nil), after.Bytes()...)
	e.jobs <- job{kind: kindEdit, id: docID, text: text, edit: ed, version: after.Version()}
}

func (e *eng) Close(docID uint64) {
	e.jobs <- job{kind: kindClose, id: docID}
}

func (e *eng) RequestSpans(docID uint64, version int32, firstLine, lastLine int) {
	select {
	case e.jobs <- job{kind: kindSpans, id: docID, version: version, first: firstLine, last: lastLine}:
	default:
	}
}

func (e *eng) RequestBracket(docID uint64, version int32, at buffer.Offset) {
	select {
	case e.jobs <- job{kind: kindBracket, id: docID, version: version, at: at}:
	default:
	}
}

func (e *eng) Flush() {
	done := make(chan struct{})
	e.jobs <- job{kind: kindFlush, done: done}
	<-done
}

func (e *eng) Shutdown() {
	done := make(chan struct{})
	e.jobs <- job{kind: kindStop, done: done}
	<-done
}

func (e *eng) loop() {
	e.parser = sitter.NewParser()
	liveParsers.Add(1)
	e.lang = sitter.NewLanguage(tsgo.Language())
	if err := e.parser.SetLanguage(e.lang); err != nil {
		e.post(bus.Notification{Level: "error", Text: "syntax: " + err.Error()})
	}
	q, qerr := sitter.NewQuery(e.lang, highlightSCM)
	if qerr != nil {
		e.post(bus.Notification{Level: "error", Text: "syntax query: " + qerr.Error()})
	} else {
		e.query = q
		e.names = q.CaptureNames()
	}
	for j := range e.jobs {
		switch j.kind {
		case kindOpen:
			e.open(j)
		case kindEdit:
			e.edit(j)
		case kindClose:
			e.closeDoc(j.id)
		case kindSpans:
			e.spans(j)
		case kindBracket:
			e.bracket(j)
		case kindFlush:
			close(j.done)
		case kindStop:
			e.stop(j.done)
			return
		}
	}
}

func (e *eng) stop(done chan struct{}) {
	e.mu.Lock()
	for id := range e.docs {
		e.closeLocked(id)
	}
	e.mu.Unlock()
	if e.query != nil {
		e.query.Close()
		e.query = nil
	}
	if e.parser != nil {
		e.parser.Close()
		e.parser = nil
		liveParsers.Add(-1)
	}
	close(done)
}

func (e *eng) open(j job) {
	e.mu.Lock()
	if old := e.docs[j.id]; old != nil {
		e.closeLocked(j.id)
	}
	d := &docTree{text: j.text, version: j.version, lang: j.lang}
	if j.lang == "go" || j.lang == "" {
		d.lang = "go"
		d.tree = e.parser.Parse(j.text, nil)
		if d.tree != nil {
			liveTrees.Add(1)
		}
	}
	e.docs[j.id] = d
	folds := foldDoc(d)
	ver := d.version
	e.mu.Unlock()
	e.post(bus.FoldRanges{DocID: j.id, Version: ver, Ranges: folds})
}

func (e *eng) edit(j job) {
	e.mu.Lock()
	d := e.docs[j.id]
	if d == nil {
		d = &docTree{lang: "go"}
		e.docs[j.id] = d
	}
	d.version = j.version
	if d.lang != "go" {
		d.text = j.text
		folds := foldDoc(d)
		ver := d.version
		e.mu.Unlock()
		e.post(bus.FoldRanges{DocID: j.id, Version: ver, Ranges: folds})
		return
	}
	next, ok := applyEdit(d.text, j.edit)
	incremental := ok && bytesEqual(next, j.text) && d.tree != nil
	if incremental {
		d.tree.Edit(inputEdit(d.text, j.text, j.edit))
		parsed := e.parser.Parse(j.text, d.tree)
		d.tree.Close()
		liveTrees.Add(-1)
		d.tree = parsed
		if parsed != nil {
			liveTrees.Add(1)
		}
	} else {
		if d.tree != nil {
			d.tree.Close()
			liveTrees.Add(-1)
			d.tree = nil
		}
		d.tree = e.parser.Parse(j.text, nil)
		if d.tree != nil {
			liveTrees.Add(1)
		}
	}
	d.text = j.text
	folds := foldDoc(d)
	ver := d.version
	e.mu.Unlock()
	e.post(bus.FoldRanges{DocID: j.id, Version: ver, Ranges: folds})
}

func (e *eng) closeDoc(id uint64) {
	e.mu.Lock()
	e.closeLocked(id)
	e.mu.Unlock()
}

func (e *eng) closeLocked(id uint64) {
	d := e.docs[id]
	if d == nil {
		return
	}
	if d.tree != nil {
		d.tree.Close()
		liveTrees.Add(-1)
	}
	delete(e.docs, id)
}

func (e *eng) spans(j job) {
	e.mu.Lock()
	d := e.docs[j.id]
	if d == nil {
		e.mu.Unlock()
		return
	}
	if d.lang != "" && d.lang != "go" {
		text := append([]byte(nil), d.text...)
		version := d.version
		first, last := j.first, j.last
		if first < 0 {
			first = 0
		}
		lines := lineCount(text)
		if last > lines {
			last = lines
		}
		if last < first {
			last = first
		}
		e.mu.Unlock()
		scanned := Scan(d.lang, text, first, last)
		if scanned == nil {
			return
		}
		out := make([][]bus.StyleSpan, len(scanned))
		for i, line := range scanned {
			if len(line) == 0 {
				continue
			}
			out[i] = make([]bus.StyleSpan, len(line))
			for k, sp := range line {
				out[i][k] = bus.StyleSpan{Start: sp.Start, End: sp.End, Style: sp.Style}
			}
		}
		e.post(bus.SyntaxSpans{DocID: j.id, Version: version, FirstLine: first, Spans: out})
		return
	}
	if d.tree == nil || e.query == nil {
		e.mu.Unlock()
		return
	}
	text := d.text
	version := d.version
	tree := d.tree
	first, last := j.first, j.last
	if first < 0 {
		first = 0
	}
	lines := lineCount(text)
	if last > lines {
		last = lines
	}
	if last < first {
		last = first
	}
	start := lineStart(text, first)
	end := lineStart(text, last)
	if last >= lines {
		end = len(text)
	}
	cur := sitter.NewQueryCursor()
	liveCursors.Add(1)
	cur.SetByteRange(uint(start), uint(end))
	matches := cur.Matches(e.query, tree.RootNode(), text)
	var raw []spanAbs
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		for _, c := range m.Captures {
			name := ""
			if int(c.Index) < len(e.names) {
				name = e.names[c.Index]
			}
			style := themeKey(name)
			if style == "" || style == "editor.foreground" {
				continue
			}
			s := int(c.Node.StartByte())
			en := int(c.Node.EndByte())
			if en < start || s > end || s >= en {
				continue
			}
			raw = append(raw, spanAbs{s, en, style, priority(name)})
		}
	}
	cur.Close()
	liveCursors.Add(-1)
	coalesced := coalesce(text, raw, first, last)
	e.mu.Unlock()

	out := make([][]bus.StyleSpan, len(coalesced))
	for i, line := range coalesced {
		if len(line) == 0 {
			continue
		}
		out[i] = make([]bus.StyleSpan, len(line))
		for k, sp := range line {
			out[i][k] = bus.StyleSpan{Start: sp.Start, End: sp.End, Style: sp.Style}
		}
	}
	e.post(bus.SyntaxSpans{DocID: j.id, Version: version, FirstLine: first, Spans: out})
}

func (e *eng) bracket(j job) {
	e.mu.Lock()
	defer e.mu.Unlock()
	d := e.docs[j.id]
	if d == nil || d.tree == nil {
		return
	}
	open, close, ok := matchBracket(d.tree.RootNode(), d.text, int(j.at))
	e.post(bus.BracketPair{DocID: j.id, Version: d.version, Open: open, Close: close, OK: ok})
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func applyEdit(text []byte, e buffer.Edit) ([]byte, bool) {
	if e.Start < 0 || e.End < e.Start || int(e.End) > len(text) {
		return nil, false
	}
	out := make([]byte, 0, len(text)-int(e.End-e.Start)+len(e.Text))
	out = append(out, text[:e.Start]...)
	out = append(out, e.Text...)
	out = append(out, text[e.End:]...)
	return out, true
}

func inputEdit(before, after []byte, e buffer.Edit) *sitter.InputEdit {
	newEnd := min(int(e.Start)+len(e.Text), len(after))
	return &sitter.InputEdit{
		StartByte:      uint(e.Start),
		OldEndByte:     uint(e.End),
		NewEndByte:     uint(newEnd),
		StartPosition:  pointAt(before, int(e.Start)),
		OldEndPosition: pointAt(before, int(e.End)),
		NewEndPosition: pointAt(after, newEnd),
	}
}

func pointAt(b []byte, off int) sitter.Point {
	if off < 0 {
		off = 0
	}
	if off > len(b) {
		off = len(b)
	}
	row := 0
	col := 0
	for i := 0; i < off; i++ {
		if b[i] == '\n' {
			row++
			col = 0
			continue
		}
		col++
	}
	return sitter.Point{Row: uint(row), Column: uint(col)}
}

func lineCount(b []byte) int {
	n := 1
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	if len(b) == 0 {
		return 1
	}
	return n
}

func lineStart(b []byte, line int) int {
	if line <= 0 {
		return 0
	}
	n := 0
	for i, c := range b {
		if c == '\n' {
			n++
			if n == line {
				return i + 1
			}
		}
	}
	return len(b)
}

// ABI returns the loaded Go grammar ABI. Tests require 15.
func ABI() uint32 {
	lang := sitter.NewLanguage(tsgo.Language())
	return lang.AbiVersion()
}

// ParseSexp parses text from scratch and returns the root S-expression.
// The caller must Shutdown-equivalent by closing the returned parser via the cleanup func.
func ParseSexp(text []byte) (sexp string, hasErr bool, cleanup func()) {
	p := sitter.NewParser()
	liveParsers.Add(1)
	lang := sitter.NewLanguage(tsgo.Language())
	_ = p.SetLanguage(lang)
	tree := p.Parse(text, nil)
	if tree != nil {
		liveTrees.Add(1)
	}
	root := tree.RootNode()
	sexp = root.ToSexp()
	hasErr = treeHasError(root)
	return sexp, hasErr, func() {
		if tree != nil {
			tree.Close()
			liveTrees.Add(-1)
		}
		p.Close()
		liveParsers.Add(-1)
	}
}

func treeHasError(n *sitter.Node) bool {
	if n == nil {
		return false
	}
	if n.IsError() || n.Kind() == "ERROR" {
		return true
	}
	cur := n.Walk()
	liveCursors.Add(1)
	defer func() {
		cur.Close()
		liveCursors.Add(-1)
	}()
	if !cur.GotoFirstChild() {
		return false
	}
	for {
		if treeHasError(cur.Node()) {
			return true
		}
		if !cur.GotoNextSibling() {
			return false
		}
	}
}

// IncrementalSexp applies edit to an existing tree and returns the new root S-expression.
func IncrementalSexp(before []byte, ed buffer.Edit, after []byte) (string, func()) {
	p := sitter.NewParser()
	liveParsers.Add(1)
	lang := sitter.NewLanguage(tsgo.Language())
	_ = p.SetLanguage(lang)
	tree := p.Parse(before, nil)
	liveTrees.Add(1)
	tree.Edit(inputEdit(before, after, ed))
	next := p.Parse(after, tree)
	tree.Close()
	liveTrees.Add(-1)
	liveTrees.Add(1)
	sexp := next.RootNode().ToSexp()
	return sexp, func() {
		next.Close()
		liveTrees.Add(-1)
		p.Close()
		liveParsers.Add(-1)
	}
}
