package syntax

import (
	"bytes"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/russoj88/gio-ide/internal/bus"
)

const (
	braceJS = iota
	braceJSON
	braceCSS
)

// computeFolds returns collapsible ranges for lang. root is the Go tree
// and is ignored for every other language.
func computeFolds(lang string, text []byte, root *sitter.Node) []bus.FoldRange {
	var raw []bus.FoldRange
	switch lang {
	case "go":
		if root != nil {
			raw = goFolds(root, text)
		}
	case "html":
		raw = htmlFolds(text)
	case "javascript":
		raw = braceFolds(text, braceJS)
	case "json":
		raw = braceFolds(text, braceJSON)
	default:
		raw = indentFolds(text)
	}
	return dedupeFolds(raw, textLines(text))
}

func foldDoc(d *docTree) []bus.FoldRange {
	if d == nil {
		return nil
	}
	var root *sitter.Node
	if d.tree != nil {
		root = d.tree.RootNode()
	}
	return computeFolds(d.lang, d.text, root)
}

func textLines(text []byte) int {
	if len(text) == 0 {
		return 1
	}
	n := 1
	for _, c := range text {
		if c == '\n' {
			n++
		}
	}
	return n
}

func byteLine(text []byte, off int) int {
	if off < 0 {
		return 0
	}
	if off > len(text) {
		off = len(text)
	}
	n := 0
	for i := 0; i < off; i++ {
		if text[i] == '\n' {
			n++
		}
	}
	return n
}

func advanceLine(text []byte, line, from, to int) int {
	if from < 0 {
		from = 0
	}
	if to > len(text) {
		to = len(text)
	}
	for i := from; i < to; i++ {
		if text[i] == '\n' {
			line++
		}
	}
	return line
}

func dedupeFolds(in []bus.FoldRange, lines int) []bus.FoldRange {
	if len(in) == 0 {
		return nil
	}
	sorted := append([]bus.FoldRange(nil), in...)
	// Shorter range first so the same header keeps the inner fold.
	for i := 1; i < len(sorted); i++ {
		j := i
		for j > 0 && foldBefore(sorted[j], sorted[j-1]) {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			j--
		}
	}
	out := make([]bus.FoldRange, 0, len(sorted))
	for _, f := range sorted {
		if f.Start < 0 || f.End <= f.Start || f.End >= lines {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Start == f.Start {
			continue
		}
		out = append(out, f)
	}
	return out
}

func foldBefore(a, b bus.FoldRange) bool {
	if a.Start != b.Start {
		return a.Start < b.Start
	}
	if a.End != b.End {
		return a.End < b.End
	}
	return a.Suffix < b.Suffix
}

func goFolds(root *sitter.Node, text []byte) []bus.FoldRange {
	var out []bus.FoldRange
	var comments []commentFold
	walkGo(root, text, &out, &comments)
	out = append(out, groupLineComments(comments)...)
	return out
}

type commentFold struct {
	line, end int
}

func walkGo(n *sitter.Node, text []byte, out *[]bus.FoldRange, comments *[]commentFold) {
	if n == nil {
		return
	}
	kind := n.Kind()
	if kind == "comment" {
		if c, ok := goComment(n, text); ok {
			if c.block {
				*out = append(*out, c.fold)
			} else {
				*comments = append(*comments, commentFold{line: c.fold.Start, end: c.fold.End})
			}
		}
	} else if kind != "ERROR" && !n.IsError() {
		if f, ok := bracketFold(n, text); ok {
			*out = append(*out, f)
		}
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		walkGo(n.Child(i), text, out, comments)
	}
}

type parsedComment struct {
	fold  bus.FoldRange
	block bool
}

func goComment(n *sitter.Node, text []byte) (parsedComment, bool) {
	s := int(n.StartByte())
	e := int(n.EndByte())
	if s < 0 || e > len(text) || e <= s {
		return parsedComment{}, false
	}
	body := text[s:e]
	startLine := byteLine(text, s)
	endLine := byteLine(text, e-1)
	if bytes.HasPrefix(body, []byte("//")) {
		return parsedComment{fold: bus.FoldRange{Start: startLine, End: endLine}}, true
	}
	if startLine >= endLine {
		return parsedComment{}, false
	}
	suffix := ""
	trim := bytes.TrimRight(body, " \t\r\n")
	if bytes.HasSuffix(trim, []byte("*/")) {
		suffix = "*/"
	}
	return parsedComment{
		block: true,
		fold:  bus.FoldRange{Start: startLine, End: endLine, Suffix: suffix},
	}, true
}

func groupLineComments(comments []commentFold) []bus.FoldRange {
	if len(comments) == 0 {
		return nil
	}
	for i := 1; i < len(comments); i++ {
		j := i
		for j > 0 && comments[j].line < comments[j-1].line {
			comments[j], comments[j-1] = comments[j-1], comments[j]
			j--
		}
	}
	var out []bus.FoldRange
	start := comments[0].line
	end := comments[0].end
	flush := func() {
		if end > start {
			out = append(out, bus.FoldRange{Start: start, End: end})
		}
	}
	for _, c := range comments[1:] {
		if c.line <= end+1 {
			if c.end > end {
				end = c.end
			}
			continue
		}
		flush()
		start = c.line
		end = c.end
	}
	flush()
	return out
}

func bracketFold(n *sitter.Node, text []byte) (bus.FoldRange, bool) {
	s := int(n.StartByte())
	e := int(n.EndByte())
	if s < 0 || e > len(text) || e <= s {
		return bus.FoldRange{}, false
	}
	a, b, ok := trimFoldEnds(text, s, e)
	if !ok || !isOpen(text[a]) || !isClose(text[b-1]) || !pairs(text[a], text[b-1]) {
		return bus.FoldRange{}, false
	}
	startLine := byteLine(text, a)
	endLine := byteLine(text, b-1)
	if startLine >= endLine {
		return bus.FoldRange{}, false
	}
	return bus.FoldRange{Start: startLine, End: endLine, Suffix: string(text[b-1])}, true
}

func trimFoldEnds(text []byte, s, e int) (int, int, bool) {
	for s < e && isFoldSpace(text[s]) {
		s++
	}
	for e > s && isFoldSpace(text[e-1]) {
		e--
	}
	if s >= e {
		return 0, 0, false
	}
	return s, e, true
}

func isFoldSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func htmlFolds(text []byte) []bus.FoldRange {
	var out []bus.FoldRange
	var stack []htmlFrame
	i := 0
	for i < len(text) {
		if text[i] != '<' {
			i++
			continue
		}
		if hasPrefixFold(text[i:], []byte("<!--")) {
			end := len(text)
			if j := indexBytes(text[i+4:], []byte("-->")); j >= 0 {
				end = i + 4 + j + 3
			}
			i = end
			continue
		}
		if i+1 < len(text) && (text[i+1] == '!' || text[i+1] == '?') {
			end := len(text)
			if j := indexByte(text[i:], '>'); j >= 0 {
				end = i + j + 1
			}
			i = end
			continue
		}
		next, name, raw, closing, self := readHTMLTag(text, i)
		if next <= i {
			i++
			continue
		}
		if closing {
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.name != name {
					continue
				}
				el := byteLine(text, next-1)
				if top.line < el {
					sufRaw := raw
					if sufRaw == "" {
						sufRaw = top.raw
					}
					out = append(out, bus.FoldRange{
						Start:  top.line,
						End:    el,
						Suffix: "</" + sufRaw + ">",
					})
				}
				break
			}
			i = next
			continue
		}
		if self || name == "" || htmlVoid(name) {
			i = next
			continue
		}
		stack = append(stack, htmlFrame{name: name, line: byteLine(text, i), raw: raw})
		if name == "script" || name == "style" {
			stop := "</script"
			mode := braceJS
			if name == "style" {
				stop = "</style"
				mode = braceCSS
			}
			end := len(text)
			if rel := indexFold(text[next:], stop); rel >= 0 {
				end = next + rel
			}
			base := byteLine(text, next)
			for _, f := range braceFolds(text[next:end], mode) {
				f.Start += base
				f.End += base
				out = append(out, f)
			}
			i = end
			continue
		}
		i = next
	}
	return out
}

type htmlFrame struct {
	name string
	raw  string
	line int
}

func readHTMLTag(text []byte, i int) (next int, name, raw string, closing, self bool) {
	var spans []absSpan
	next, name, open := scanHTMLTag(text, i, &spans)
	if next <= i {
		return i, "", "", false, false
	}
	closing = i+1 < len(text) && text[i+1] == '/'
	self = !open && !closing
	j := i + 1
	if closing && j < len(text) && text[j] == '/' {
		j++
	}
	nameAt := j
	j = scanHTMLName(text, j)
	raw = string(text[nameAt:j])
	return next, name, raw, closing, self
}

func htmlVoid(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

func braceFolds(text []byte, mode int) []bus.FoldRange {
	var stack []braceFrame
	var out []bus.FoldRange
	line := 0
	prev := byte('o')
	var scratch []absSpan
	i := 0
	for i < len(text) {
		c := text[i]
		switch {
		case c == '\n':
			line++
			i++
			prev = 'o'
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case mode != braceJSON && c == '/' && i+1 < len(text) && text[i+1] == '/':
			for i < len(text) && text[i] != '\n' {
				i++
			}
			prev = 'o'
		case mode != braceJSON && c == '/' && i+1 < len(text) && text[i+1] == '*':
			i += 2
			for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
				if text[i] == '\n' {
					line++
				}
				i++
			}
			if i+1 < len(text) {
				i += 2
			} else {
				i = len(text)
			}
			prev = 'o'
		case mode == braceJS && c == '/' && jsRegexp(prev):
			n, ok := scanJSRegexp(text, i)
			if !ok || n <= i {
				i++
				prev = 'o'
				continue
			}
			line = advanceLine(text, line, i, n)
			i = n
			prev = 's'
		case c == '\'' || c == '"':
			n := scanJSString(text, i)
			if n <= i {
				i++
				continue
			}
			line = advanceLine(text, line, i, n)
			i = n
			prev = 's'
		case mode == braceJS && c == '`':
			n := scanTemplate(text, i, &scratch)
			if n <= i {
				i++
				continue
			}
			line = advanceLine(text, line, i, n)
			i = n
			prev = 's'
		case c == '{' || c == '(' || c == '[':
			stack = append(stack, braceFrame{ch: c, line: line})
			i++
			prev = c
		case c == '}' || c == ')' || c == ']':
			want := byte('{')
			switch c {
			case ')':
				want = '('
			case ']':
				want = '['
			}
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.ch != want {
					continue
				}
				if top.line < line {
					out = append(out, bus.FoldRange{Start: top.line, End: line, Suffix: string(c)})
				}
				break
			}
			i++
			prev = c
		default:
			if isIdentByte(c) {
				prev = 'i'
			} else if isDigit(c) {
				prev = 'n'
			} else {
				prev = 'o'
			}
			i++
		}
	}
	return out
}

type braceFrame struct {
	ch   byte
	line int
}

func indentFolds(text []byte) []bus.FoldRange {
	lines := bytes.Split(text, []byte{'\n'})
	n := len(lines)
	ind := make([]int, n)
	blank := make([]bool, n)
	for i, ln := range lines {
		ind[i], blank[i] = indentOf(ln)
	}
	end := make([]int, n)
	for i := range end {
		end[i] = -1
	}
	prevNonBlank := func(i int) int {
		for j := i - 1; j >= 0; j-- {
			if !blank[j] {
				return j
			}
		}
		return -1
	}
	var stack []int
	for i := range n {
		if blank[i] {
			continue
		}
		for len(stack) > 0 && ind[i] <= ind[stack[len(stack)-1]] {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if e := prevNonBlank(i); e > top {
				end[top] = e
			}
		}
		stack = append(stack, i)
	}
	last := prevNonBlank(n)
	for _, top := range stack {
		if last > top {
			end[top] = last
		}
	}
	var out []bus.FoldRange
	for i, e := range end {
		if e > i {
			out = append(out, bus.FoldRange{Start: i, End: e})
		}
	}
	return out
}

// indentOf counts one level per space or tab. The bool is true when the
// line has no other bytes. A tab is one level, the same as one space.
func indentOf(line []byte) (int, bool) {
	n := 0
	for _, c := range line {
		switch c {
		case ' ', '\t':
			n++
		case '\r':
			continue
		default:
			return n, false
		}
	}
	return n, true
}
