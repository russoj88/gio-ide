package editor

import (
	"regexp"
	"slices"

	"github.com/russoj88/gio-ide/internal/buffer"
)

// FindAll returns non-overlapping matches of query. regex selects regexp syntax.
func (v *View) FindAll(query string, regex bool) ([]Sel, error) {
	if query == "" {
		return nil, nil
	}
	b := v.Doc.Snapshot().Bytes()
	if !regex {
		var out []Sel
		for from := 0; from <= len(b)-len(query); {
			idx := indexFrom(b, query, from)
			if idx < 0 {
				break
			}
			out = append(out, Sel{Anchor: buffer.Offset(idx), Head: buffer.Offset(idx + len(query))})
			from = idx + len(query)
		}
		return out, nil
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, err
	}
	idx := re.FindAllIndex(b, -1)
	out := make([]Sel, 0, len(idx))
	for _, pair := range idx {
		if pair[0] == pair[1] {
			continue
		}
		out = append(out, Sel{Anchor: buffer.Offset(pair[0]), Head: buffer.Offset(pair[1])})
	}
	return out, nil
}

// FindNext selects the next match after the caret. dir < 0 searches backward.
func (v *View) FindNext(query string, regex bool, dir int) error {
	matches, err := v.FindAll(query, regex)
	if err != nil || len(matches) == 0 {
		return err
	}
	at := int(v.primary().Head)
	pick := -1
	if dir >= 0 {
		for i, m := range matches {
			if int(m.min()) > at || (int(m.min()) == at && int(m.max()) > int(v.primary().max())) {
				pick = i
				break
			}
		}
		if pick < 0 {
			pick = 0
		}
	} else {
		for i, matche := range slices.Backward(matches) {
			if int(matche.min()) < at {
				pick = i
				break
			}
		}
		if pick < 0 {
			pick = len(matches) - 1
		}
	}
	v.sels = []Sel{matches[pick]}
	v.Doc.BreakGroup()
	v.Reveal()
	return nil
}

// ReplaceAll replaces every match. It returns how many replacements were made.
func (v *View) ReplaceAll(query, repl string, regex bool) (int, error) {
	matches, err := v.FindAll(query, regex)
	if err != nil || len(matches) == 0 {
		return 0, err
	}
	b := v.Doc.Snapshot().Bytes()
	var re *regexp.Regexp
	if regex {
		re, _ = regexp.Compile(query)
	}
	edits := make([]buffer.Edit, len(matches))
	for i, m := range matches {
		text := repl
		if re != nil {
			text = expandRepl(re, b[m.min():m.max()], repl)
		}
		edits[i] = buffer.Edit{Start: m.min(), End: m.max(), Text: text}
	}
	if _, err := v.Doc.Apply(edits, buffer.OriginCommand); err != nil {
		return 0, err
	}
	v.collapseToEnd()
	return len(matches), nil
}

// ReplaceCurrent replaces the primary selection if it matches, then selects the next match.
func (v *View) ReplaceCurrent(query, repl string, regex bool) error {
	s := v.primary()
	if s.empty() {
		return v.FindNext(query, regex, 1)
	}
	got := string(v.Doc.Snapshot().Slice(s.min(), s.max()))
	ok := got == query
	replText := repl
	if regex {
		re, err := regexp.Compile(query)
		if err != nil {
			return err
		}
		if !re.MatchString(got) {
			return v.FindNext(query, regex, 1)
		}
		replText = expandRepl(re, []byte(got), repl)
		ok = true
	}
	if !ok {
		return v.FindNext(query, regex, 1)
	}
	v.sels = []Sel{s}
	v.Insert(replText, buffer.OriginCommand)
	return v.FindNext(query, regex, 1)
}

func expandRepl(re *regexp.Regexp, src []byte, repl string) string {
	return string(re.Expand(nil, []byte(repl), src, re.FindSubmatchIndex(src)))
}
