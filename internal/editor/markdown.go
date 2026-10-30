package editor

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// BlockKind is the kind of one markdown block.
type BlockKind int

const (
	// BlockBlank is an empty line.
	BlockBlank BlockKind = iota
	// BlockParagraph is wrapped prose.
	BlockParagraph
	// BlockHeading is an ATX heading. Level is 1 through 6.
	BlockHeading
	// BlockCode is a fenced code block. Lines are the body. Lang is the info tag.
	BlockCode
	// BlockList is one flat list item. Marker is "-" or "1." and the like.
	BlockList
	// BlockRule is a thematic break.
	BlockRule
)

// InlineKind is the kind of one run inside a paragraph, heading, or list item.
type InlineKind int

const (
	// InlineText is unstyled prose.
	InlineText InlineKind = iota
	// InlineCode is a `code` span.
	InlineCode
	// InlineEm is emphasis.
	InlineEm
	// InlineStrong is strong emphasis.
	InlineStrong
	// InlineLink is a [label](url). Text is the label. Href is the URL.
	InlineLink
)

// Inline is one styled run. Text has no raw tab and no rune below U+0020.
type Inline struct {
	Kind InlineKind
	Text string
	Href string
}

// Block is one markdown block from ParseMarkdown.
type Block struct {
	Kind    BlockKind
	Level   int
	Lang    string
	Marker  string
	Inlines []Inline
	Lines   []string
}

// ParseMarkdown splits src into a small markdown subset: ATX headings,
// paragraphs, blank lines, fenced code, one-level lists, inline code,
// emphasis, strong, links, and thematic rules. HTML, tables, images (the
// alt text is kept), reference links, and footnotes stay literal text.
func ParseMarkdown(src []byte) []Block {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	src = bytes.ReplaceAll(src, []byte("\r"), []byte("\n"))
	if len(src) == 0 {
		return nil
	}
	// A terminating newline is not a blank line. A blank line is a run of
	// empty entries between content.
	if src[len(src)-1] == '\n' {
		src = src[:len(src)-1]
	}
	lines := bytes.Split(src, []byte("\n"))
	var blocks []Block
	for i := 0; i < len(lines); {
		line := lines[i]
		trim := bytes.TrimSpace(line)
		if len(trim) == 0 {
			blocks = append(blocks, Block{Kind: BlockBlank})
			i++
			continue
		}
		if isRule(trim) {
			blocks = append(blocks, Block{Kind: BlockRule})
			i++
			continue
		}
		if marker, n, lang, ok := openFence(trim); ok {
			i++
			var body []string
			for i < len(lines) && !closeFence(lines[i], marker, n) {
				body = append(body, markdownPlain(string(lines[i]), 4))
				i++
			}
			if i < len(lines) {
				i++
			}
			blocks = append(blocks, Block{Kind: BlockCode, Lang: lang, Lines: body})
			continue
		}
		if level, rest, ok := atxHeading(line); ok {
			blocks = append(blocks, Block{Kind: BlockHeading, Level: level, Inlines: parseInlines(rest)})
			i++
			continue
		}
		if marker, rest, ok := listItem(line); ok {
			blocks = append(blocks, Block{Kind: BlockList, Marker: marker, Inlines: parseInlines(rest)})
			i++
			continue
		}
		var para []byte
		for i < len(lines) {
			l := lines[i]
			t := bytes.TrimSpace(l)
			if len(t) == 0 || isRule(t) || fenceStart(t) || headingStart(l) || listStart(l) {
				break
			}
			if len(para) > 0 {
				para = append(para, ' ')
			}
			para = append(para, t...)
			i++
		}
		blocks = append(blocks, Block{Kind: BlockParagraph, Inlines: parseInlines(string(para))})
	}
	return blocks
}

// markdownPlain expands tabs and drops other C0 controls and DEL so a shaper
// never sees a missing Go Mono glyph.
func markdownPlain(s string, tabWidth int) string {
	if tabWidth < 1 {
		tabWidth = 4
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch {
		case r == '\t':
			n := tabWidth - col%tabWidth
			for range n {
				b.WriteByte(' ')
				col++
			}
		case r < 0x20 || r == 0x7f:
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}

func parseInlines(s string) []Inline {
	s = markdownPlain(s, 4)
	var out []Inline
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, Inline{Kind: InlineText, Text: b.String()})
		b.Reset()
	}
	for i := 0; i < len(s); {
		if s[i] == '`' {
			j := strings.IndexByte(s[i+1:], '`')
			if j >= 0 {
				flush()
				out = append(out, Inline{Kind: InlineCode, Text: s[i+1 : i+1+j]})
				i += j + 2
				continue
			}
		}
		if strings.HasPrefix(s[i:], "**") || strings.HasPrefix(s[i:], "__") {
			marker := s[i : i+2]
			j := strings.Index(s[i+2:], marker)
			if j >= 0 {
				flush()
				out = append(out, Inline{Kind: InlineStrong, Text: s[i+2 : i+2+j]})
				i += j + 4
				continue
			}
		}
		if s[i] == '*' || s[i] == '_' {
			j := strings.IndexByte(s[i+1:], s[i])
			if j > 0 {
				flush()
				out = append(out, Inline{Kind: InlineEm, Text: s[i+1 : i+1+j]})
				i += j + 2
				continue
			}
		}
		if s[i] == '!' && i+1 < len(s) && s[i+1] == '[' {
			if text, next, ok := parseLink(s, i+1); ok {
				b.WriteString(text)
				i = next
				continue
			}
		}
		if s[i] == '[' {
			if text, next, href, ok := parseLinkHref(s, i); ok {
				flush()
				out = append(out, Inline{Kind: InlineLink, Text: text, Href: href})
				i = next
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += size
	}
	flush()
	return out
}

func parseLink(s string, i int) (text string, next int, ok bool) {
	text, next, _, ok = parseLinkHref(s, i)
	return text, next, ok
}

func parseLinkHref(s string, i int) (text string, next int, href string, ok bool) {
	if i >= len(s) || s[i] != '[' {
		return "", 0, "", false
	}
	closeRel := strings.IndexByte(s[i:], ']')
	if closeRel < 0 {
		return "", 0, "", false
	}
	close := i + closeRel
	if close+1 >= len(s) || s[close+1] != '(' {
		return "", 0, "", false
	}
	endRel := strings.IndexByte(s[close+2:], ')')
	if endRel < 0 {
		return "", 0, "", false
	}
	return s[i+1 : close], close + 2 + endRel + 1, s[close+2 : close+2+endRel], true
}

func isRule(trim []byte) bool {
	if len(trim) < 3 {
		return false
	}
	c := trim[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	n := 0
	for _, b := range trim {
		if b == c {
			n++
			continue
		}
		if b == ' ' || b == '\t' {
			continue
		}
		return false
	}
	return n >= 3
}

func openFence(trim []byte) (marker byte, n int, lang string, ok bool) {
	if len(trim) < 3 {
		return 0, 0, "", false
	}
	c := trim[0]
	if c != '`' && c != '~' {
		return 0, 0, "", false
	}
	for n < len(trim) && trim[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	lang = strings.TrimSpace(string(trim[n:]))
	if i := strings.IndexAny(lang, " \t"); i >= 0 {
		lang = lang[:i]
	}
	return c, n, lang, true
}

func fenceStart(trim []byte) bool {
	_, _, _, ok := openFence(trim)
	return ok
}

func closeFence(line []byte, marker byte, n int) bool {
	trim := bytes.TrimSpace(line)
	if len(trim) < n {
		return false
	}
	for i := range trim {
		if trim[i] != marker {
			return false
		}
	}
	return true
}

func atxHeading(line []byte) (int, string, bool) {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 3 {
		i++
	}
	if i >= len(line) || line[i] != '#' {
		return 0, "", false
	}
	level := 0
	for i < len(line) && line[i] == '#' {
		level++
		i++
	}
	if level < 1 || level > 6 {
		return 0, "", false
	}
	if i < len(line) && line[i] != ' ' && line[i] != '\t' {
		return 0, "", false
	}
	rest := bytes.TrimSpace(line[i:])
	rest = bytes.TrimRight(rest, " \t")
	if len(rest) > 0 && rest[len(rest)-1] == '#' {
		j := len(rest)
		for j > 0 && rest[j-1] == '#' {
			j--
		}
		if j == 0 || rest[j-1] == ' ' || rest[j-1] == '\t' {
			rest = bytes.TrimSpace(rest[:j])
		}
	}
	return level, string(rest), true
}

func headingStart(line []byte) bool {
	_, _, ok := atxHeading(line)
	return ok
}

func listItem(line []byte) (marker, rest string, ok bool) {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 3 {
		i++
	}
	if i >= len(line) {
		return "", "", false
	}
	if line[i] == '-' || line[i] == '*' || line[i] == '+' {
		if i+1 < len(line) && (line[i+1] == ' ' || line[i+1] == '\t') {
			return string(line[i : i+1]), string(bytes.TrimSpace(line[i+2:])), true
		}
		return "", "", false
	}
	j := i
	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
	}
	if j > i && j < len(line) && line[j] == '.' && j+1 < len(line) && (line[j+1] == ' ' || line[j+1] == '\t') {
		return string(line[i : j+1]), string(bytes.TrimSpace(line[j+2:])), true
	}
	return "", "", false
}

func listStart(line []byte) bool {
	_, _, ok := listItem(line)
	return ok
}
