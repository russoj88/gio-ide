package syntax

// themeKey maps a tree-sitter capture name to a theme key.
// Unknown names and plain variables use the editor foreground, which the
// painter already uses, so they are omitted.
func themeKey(capture string) string {
	switch capture {
	case "keyword":
		return "syntax.keyword"
	case "string", "escape":
		return "syntax.string"
	case "number":
		return "syntax.number"
	case "comment":
		return "syntax.comment"
	case "function", "function.method", "function.builtin":
		return "syntax.function"
	case "type":
		return "syntax.type"
	case "constant", "constant.builtin":
		return "syntax.constant"
	case "property":
		return "syntax.property"
	case "operator":
		return "syntax.operator"
	default:
		return "editor.foreground"
	}
}

func priority(capture string) int {
	switch capture {
	case "function.builtin":
		return 100
	case "function.method":
		return 90
	case "function":
		return 80
	case "keyword":
		return 70
	case "comment":
		return 65
	case "string", "escape":
		return 60
	case "number":
		return 55
	case "constant.builtin", "constant":
		return 50
	case "type":
		return 40
	case "property":
		return 30
	case "operator":
		return 20
	default:
		return 10
	}
}

type spanAbs struct {
	start, end int
	style      string
	pri        int
}

func coalesce(text []byte, raw []spanAbs, first, last int) [][]StyleSpan {
	nLines := max(last-first, 0)
	out := make([][]StyleSpan, nLines)
	if len(raw) == 0 || nLines == 0 {
		return out
	}
	// For each byte, keep the highest-priority style. The window is the
	// visible range, so this stays small.
	type cell struct {
		pri   int
		style string
	}
	winStart := lineStart(text, first)
	winEnd := lineStart(text, last)
	if last >= lineCount(text) {
		winEnd = len(text)
	}
	if winEnd < winStart {
		winEnd = winStart
	}
	cells := make([]cell, winEnd-winStart)
	for _, sp := range raw {
		s, e := sp.start, sp.end
		if s < winStart {
			s = winStart
		}
		if e > winEnd {
			e = winEnd
		}
		for i := s; i < e; i++ {
			idx := i - winStart
			if sp.pri >= cells[idx].pri {
				cells[idx] = cell{pri: sp.pri, style: sp.style}
			}
		}
	}
	// Group back into lines. Column is a byte offset within the line.
	line := 0
	col := 0
	lineBase := 0
	flush := func(from, to int, style string) {
		if style == "" || from >= to || line < 0 || line >= len(out) {
			return
		}
		out[line] = append(out[line], StyleSpan{Start: from - lineBase, End: to - lineBase, Style: style})
	}
	i := 0
	for i < len(cells) {
		if text[winStart+i] == '\n' {
			line++
			col = 0
			lineBase = i + 1
			i++
			continue
		}
		if cells[i].style == "" {
			i++
			col++
			continue
		}
		j := i + 1
		for j < len(cells) && text[winStart+j] != '\n' && cells[j].style == cells[i].style {
			j++
		}
		flush(i, j, cells[i].style)
		i = j
		_ = col
	}
	return out
}
