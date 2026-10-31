package app

import (
	"strings"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/syntax"
)

// docPaint is the latest syntax color for one document, in absolute bytes.
type docPaint struct {
	version int32
	spans   []absSpan
	open    int
	close   int
	pair    bool
}

type absSpan struct {
	start, end int
	style      string
}

func isGoPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".go")
}

func (s *shell) applySpans(ev bus.SyntaxSpans) {
	if s.colors == nil {
		s.colors = map[uint64]*docPaint{}
	}
	p := s.colors[ev.DocID]
	if p == nil {
		p = &docPaint{}
		s.colors[ev.DocID] = p
	}
	if ev.Version < p.version {
		return
	}
	p.version = ev.Version
	// Replace spans that fall in the returned window. Other lines stay until shifted.
	var kept []absSpan
	// We do not know the window in bytes here, so replace the whole set when the
	// version matches a full visible query. Visible queries are the only source.
	p.spans = kept
	// Reconstruct absolute offsets from the line-relative spans using any open view.
	snap := s.snapshot(ev.DocID)
	if snap == nil {
		return
	}
	var spans []absSpan
	for i, line := range ev.Spans {
		ln := ev.FirstLine + i
		if ln < 0 || ln >= snap.LineCount() {
			continue
		}
		base := int(snap.LineStart(ln))
		for _, sp := range line {
			spans = append(spans, absSpan{start: base + sp.Start, end: base + sp.End, style: sp.Style})
		}
	}
	p.spans = spans
}

func (s *shell) applyFolds(ev bus.FoldRanges) {
	folds := make([]editor.Fold, len(ev.Ranges))
	for i, r := range ev.Ranges {
		folds[i] = editor.Fold{Start: r.Start, End: r.End, Suffix: r.Suffix}
	}
	s.eachDocView(ev.DocID, func(v *editor.View) {
		v.SetFolds(ev.Version, folds)
	})
}

func (s *shell) applyBracket(ev bus.BracketPair) {
	if s.colors == nil {
		s.colors = map[uint64]*docPaint{}
	}
	p := s.colors[ev.DocID]
	if p == nil {
		p = &docPaint{}
		s.colors[ev.DocID] = p
	}
	p.pair = ev.OK
	p.open = ev.Open
	p.close = ev.Close
}

func (s *shell) shiftPaint(id uint64, ed buffer.Edit) {
	p := s.colors[id]
	if p == nil {
		return
	}
	delta := len(ed.Text) - int(ed.End-ed.Start)
	var out []absSpan
	for _, sp := range p.spans {
		if sp.end <= int(ed.Start) {
			out = append(out, sp)
			continue
		}
		if sp.start >= int(ed.End) {
			sp.start += delta
			sp.end += delta
			out = append(out, sp)
		}
	}
	p.spans = out
	if p.pair {
		p.open = shiftOff(p.open, ed, delta)
		p.close = shiftOff(p.close, ed, delta)
	}
}

func shiftOff(off int, ed buffer.Edit, delta int) int {
	if off < int(ed.Start) {
		return off
	}
	if off >= int(ed.End) {
		return off + delta
	}
	return int(ed.Start)
}

func (s *shell) snapshot(id uint64) buffer.Snapshot {
	s.docsMu.Lock()
	doc := s.docs[id]
	s.docsMu.Unlock()
	if doc == nil {
		return nil
	}
	return doc.Snapshot()
}

func (s *shell) spansFor(view *editor.View) map[int][]editor.Span {
	if view == nil {
		return nil
	}
	p := s.colors[view.Doc.ID()]
	snap := view.Doc.Snapshot()
	out := map[int][]editor.Span{}
	add := func(line, start, end int, style string) {
		if start >= end {
			return
		}
		out[line] = append(out[line], editor.Span{Start: start, End: end, Color: s.theme.Color(style)})
	}
	if p != nil && p.pair {
		s.addAbs(snap, p.open, p.open+1, "editor.bracketMatch", add)
		s.addAbs(snap, p.close, p.close+1, "editor.bracketMatch", add)
	}
	if p != nil {
		for _, sp := range p.spans {
			s.addAbs(snap, sp.start, sp.end, sp.style, add)
		}
	}
	return out
}

func (s *shell) addAbs(snap buffer.Snapshot, start, end int, style string, add func(line, start, end int, style string)) {
	if snap == nil || start >= end || start < 0 {
		return
	}
	if end > snap.Len() {
		end = snap.Len()
	}
	for off := start; off < end; {
		line, col := snap.OffsetToLineCol(buffer.Offset(off))
		lineEnd := snap.Len()
		if line+1 < snap.LineCount() {
			lineEnd = int(snap.LineStart(line + 1))
		}
		chunk := min(end, lineEnd)
		// Do not color the newline itself.
		body := int(snap.LineStart(line)) + len(snap.Line(line))
		if chunk > body {
			chunk = body
		}
		if off < chunk {
			add(line, col, col+(chunk-off), style)
		}
		if chunk <= off {
			break
		}
		off = chunk
		if off < end && off < snap.Len() && off == body {
			off++ // skip newline
		}
	}
}

func (s *shell) requestSyntax(t *edTab) {
	if s.syn == nil || t == nil {
		return
	}
	lang := syntax.Lang(t.view.Doc.Path())
	if lang == "" {
		return
	}
	doc := t.view.Doc
	ver := doc.Version()
	first, last := t.view.SyntaxWindow()
	if t.reqVer != ver || t.reqFirst != first || t.reqLast != last {
		if s.syn.RequestSpans(doc.ID(), ver, first, last) {
			t.reqVer = ver
			t.reqFirst = first
			t.reqLast = last
		}
	}
	if lang != "go" {
		return
	}
	at := int(t.view.CaretOffset())
	if t.bracketAt != at {
		if s.syn.RequestBracket(doc.ID(), ver, buffer.Offset(at)) {
			t.bracketAt = at
		}
	}
}

func oneEdit(old, neu []byte) buffer.Edit {
	i := 0
	for i < len(old) && i < len(neu) && old[i] == neu[i] {
		i++
	}
	j := 0
	for i+j < len(old) && i+j < len(neu) && old[len(old)-1-j] == neu[len(neu)-1-j] {
		j++
	}
	return buffer.Edit{
		Start: buffer.Offset(i),
		End:   buffer.Offset(len(old) - j),
		Text:  string(neu[i : len(neu)-j]),
	}
}
