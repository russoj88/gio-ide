package syntax

import "strings"

// absSpan is a byte range in the whole buffer, not a column on one line.
type absSpan struct {
	start, end int
	style      string
}

func addSpan(dst *[]absSpan, start, end int, style string) {
	if style == "" || end <= start {
		return
	}
	*dst = append(*dst, absSpan{start: start, end: end, style: style})
}

// clipSpans splits absolute spans into per-line columns for [first, last).
// Newlines are not part of a span. Spans are clipped to each line.
func clipSpans(text []byte, raw []absSpan, first, last int) [][]StyleSpan {
	lines := splitScanLines(text)
	if first < 0 {
		first = 0
	}
	if last > len(lines) {
		last = len(lines)
	}
	if last < first {
		last = first
	}
	out := make([][]StyleSpan, last-first)
	off := make([]int, len(lines))
	p := 0
	for i, ln := range lines {
		off[i] = p
		p += len(ln)
		if p < len(text) && text[p] == '\n' {
			p++
		}
	}
	for _, sp := range raw {
		if sp.end <= sp.start {
			continue
		}
		for li := first; li < last; li++ {
			ls := off[li]
			le := ls + len(lines[li])
			if sp.end <= ls || sp.start >= le {
				continue
			}
			s := max(sp.start, ls) - ls
			e := min(sp.end, le) - ls
			if s < e {
				out[li-first] = append(out[li-first], StyleSpan{Start: s, End: e, Style: sp.style})
			}
		}
	}
	return out
}

func hasPrefixFold(s, prefix []byte) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i, c := range prefix {
		if foldByte(s[i]) != foldByte(c) {
			return false
		}
	}
	return true
}

func foldByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isHex(b byte) bool {
	return isDigit(b) || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// Dockerfiles.

type dockerCarry struct {
	cont bool
}

func dockerInstr(word string) bool {
	switch strings.ToUpper(word) {
	case "ADD", "ARG", "CMD", "COPY", "ENTRYPOINT", "ENV", "EXPOSE", "FROM",
		"HEALTHCHECK", "LABEL", "MAINTAINER", "ONBUILD", "RUN", "SHELL",
		"STOPSIGNAL", "USER", "VOLUME", "WORKDIR":
		return true
	default:
		return false
	}
}

func takeLetters(line []byte, i int) (string, int) {
	start := i
	for i < len(line) && isLetter(line[i]) {
		i++
	}
	return string(line[start:i]), i
}

func scanDockerLine(line []byte, st *dockerCarry) []StyleSpan {
	continued := st.cont
	st.cont = false
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t' || line[i] == '\r') {
		i++
	}
	if !continued && i < len(line) && line[i] == '#' {
		return []StyleSpan{{Start: i, End: len(line), Style: "syntax.comment"}}
	}
	var out []StyleSpan
	if !continued && i < len(line) && isLetter(line[i]) {
		word, next := takeLetters(line, i)
		if dockerInstr(word) {
			out = append(out, StyleSpan{Start: i, End: next, Style: "syntax.keyword"})
			i = next
			upper := strings.ToUpper(word)
			if upper == "ONBUILD" || upper == "HEALTHCHECK" {
				j := i
				for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
					j++
				}
				if j < len(line) && isLetter(line[j]) {
					nextWord, n := takeLetters(line, j)
					if dockerInstr(nextWord) {
						out = append(out, StyleSpan{Start: j, End: n, Style: "syntax.keyword"})
						i = n
					}
				}
			}
		}
	}
	out = append(out, scanDockerRest(line, i, st)...)
	return out
}

func scanDockerRest(line []byte, i int, st *dockerCarry) []StyleSpan {
	var out []StyleSpan
	for i < len(line) {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '\\' && dockerLineEnd(line, i):
			out = append(out, StyleSpan{Start: i, End: i + 1, Style: "syntax.operator"})
			st.cont = true
			return out
		case c == '"' || c == '\'':
			out, i = appendQuoted(out, line, i)
		case c == '#':
			out = append(out, StyleSpan{Start: i, End: len(line), Style: "syntax.comment"})
			return out
		case c == '$':
			start := i
			i = scanDockerVar(line, i)
			if i > start+1 {
				out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.constant"})
			}
		case c == '-' && i+1 < len(line) && line[i+1] == '-':
			start := i
			i += 2
			for i < len(line) && line[i] != ' ' && line[i] != '\t' && line[i] != '=' && line[i] != '"' && line[i] != '\'' {
				i++
			}
			out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.property"})
		case c == '[' || c == ']' || c == '{' || c == '}' || c == ',' || c == '=':
			out = append(out, StyleSpan{Start: i, End: i + 1, Style: "syntax.operator"})
			i++
		case isDigit(c):
			start := i
			i = scanDockerNumber(line, i)
			out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.number"})
		case isLetter(c):
			word, next := takeLetters(line, i)
			if strings.EqualFold(word, "AS") {
				out = append(out, StyleSpan{Start: i, End: next, Style: "syntax.keyword"})
			}
			i = next
		default:
			i++
		}
	}
	return out
}

func dockerLineEnd(line []byte, i int) bool {
	if i >= len(line) || line[i] != '\\' {
		return false
	}
	for _, c := range line[i+1:] {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}

func scanDockerVar(line []byte, i int) int {
	if i >= len(line) || line[i] != '$' {
		return i
	}
	if i+1 < len(line) && line[i+1] == '{' {
		j := i + 2
		for j < len(line) && line[j] != '}' {
			j++
		}
		if j < len(line) && line[j] == '}' {
			return j + 1
		}
		return j
	}
	j := i + 1
	if j >= len(line) || !isIdentByte(line[j]) {
		return i + 1
	}
	j++
	for j < len(line) && isIdentByte(line[j]) {
		j++
	}
	return j
}

func scanDockerNumber(line []byte, i int) int {
	for i < len(line) && isDigit(line[i]) {
		i++
	}
	if i+1 < len(line) && line[i] == '.' && isDigit(line[i+1]) {
		i++
		for i < len(line) && isDigit(line[i]) {
			i++
		}
	}
	return i
}

// Env files.

type envCarry struct {
	quote byte
}

func isEnvStart(b byte) bool {
	return b == '_' || isLetter(b)
}

func isEnvCont(b byte) bool {
	return isEnvStart(b) || isDigit(b)
}

func scanEnvLine(line []byte, st *envCarry) []StyleSpan {
	if st.quote != 0 {
		return finishEnvQuote(line, st)
	}
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t' || line[i] == '\r') {
		i++
	}
	if i >= len(line) {
		return nil
	}
	if line[i] == '#' {
		return []StyleSpan{{Start: i, End: len(line), Style: "syntax.comment"}}
	}
	var out []StyleSpan
	if hasEnvWord(line, i, "export") {
		j := i + len("export")
		if j >= len(line) || line[j] == ' ' || line[j] == '\t' {
			out = append(out, StyleSpan{Start: i, End: j, Style: "syntax.keyword"})
			i = j
			for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
				i++
			}
		}
	}
	if i >= len(line) || !isEnvStart(line[i]) {
		return out
	}
	key := i
	i++
	for i < len(line) && isEnvCont(line[i]) {
		i++
	}
	j := i
	for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
		j++
	}
	if j >= len(line) || line[j] != '=' {
		return out
	}
	out = append(out, StyleSpan{Start: key, End: i, Style: "syntax.property"})
	out = append(out, StyleSpan{Start: j, End: j + 1, Style: "syntax.operator"})
	out = append(out, scanEnvValue(line, j+1, st)...)
	return out
}

func hasEnvWord(line []byte, i int, word string) bool {
	if i+len(word) > len(line) || string(line[i:i+len(word)]) != word {
		return false
	}
	end := i + len(word)
	return end >= len(line) || !isEnvCont(line[end])
}

func scanEnvValue(line []byte, i int, st *envCarry) []StyleSpan {
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) {
		return nil
	}
	if line[i] == '"' || line[i] == '\'' {
		return scanEnvQuoted(line, i, st)
	}
	var out []StyleSpan
	for i < len(line) {
		if line[i] == ' ' || line[i] == '\t' {
			j := i + 1
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			if j < len(line) && line[j] == '#' {
				out = append(out, StyleSpan{Start: j, End: len(line), Style: "syntax.comment"})
				return out
			}
			i++
			continue
		}
		if line[i] == '$' {
			start := i
			i = scanEnvVar(line, i)
			if i > start+1 {
				out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.constant"})
			}
			continue
		}
		if line[i] == '#' {
			out = append(out, StyleSpan{Start: i, End: len(line), Style: "syntax.comment"})
			return out
		}
		i++
	}
	return out
}

func scanEnvQuoted(line []byte, i int, st *envCarry) []StyleSpan {
	q := line[i]
	start := i
	i++
	for i < len(line) {
		if q == '"' && line[i] == '\\' && i+1 < len(line) {
			i += 2
			continue
		}
		if line[i] == q {
			i++
			return []StyleSpan{{Start: start, End: i, Style: "syntax.string"}}
		}
		i++
	}
	st.quote = q
	return []StyleSpan{{Start: start, End: len(line), Style: "syntax.string"}}
}

func finishEnvQuote(line []byte, st *envCarry) []StyleSpan {
	q := st.quote
	i := 0
	for i < len(line) {
		if q == '"' && line[i] == '\\' && i+1 < len(line) {
			i += 2
			continue
		}
		if line[i] == q {
			i++
			st.quote = 0
			spans := []StyleSpan{{Start: 0, End: i, Style: "syntax.string"}}
			j := i
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			if j < len(line) && line[j] == '#' {
				spans = append(spans, StyleSpan{Start: j, End: len(line), Style: "syntax.comment"})
			}
			return spans
		}
		i++
	}
	if len(line) == 0 {
		return nil
	}
	return []StyleSpan{{Start: 0, End: len(line), Style: "syntax.string"}}
}

func scanEnvVar(line []byte, i int) int {
	if i >= len(line) || line[i] != '$' {
		return i
	}
	if i+1 < len(line) && line[i+1] == '{' {
		j := i + 2
		for j < len(line) && line[j] != '}' {
			j++
		}
		if j < len(line) && line[j] == '}' {
			return j + 1
		}
		return j
	}
	j := i + 1
	if j >= len(line) || !isEnvStart(line[j]) {
		return i + 1
	}
	j++
	for j < len(line) && isEnvCont(line[j]) {
		j++
	}
	return j
}

// JavaScript. scanJSCode and scanTemplate call each other so template
// ${} expressions and nested templates keep their own brace depth.

func highlightJS(text, stop []byte) ([]absSpan, int) {
	var spans []absSpan
	n := scanJSCode(text, 0, &spans, 0, stop)
	return spans, n
}

func scanJSCode(text []byte, i int, spans *[]absSpan, openBraces int, stop []byte) int {
	braces := openBraces
	prev := byte('o')
	want := 0
	for i < len(text) {
		if openBraces == 0 && matchStop(text, i, stop) {
			return i
		}
		c := text[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '\n':
			i++
			prev = 'o'
		case c == '/' && i+1 < len(text) && text[i+1] == '/':
			start := i
			for i < len(text) && text[i] != '\n' {
				i++
			}
			addSpan(spans, start, i, "syntax.comment")
			prev = 'o'
			want = 0
		case c == '/' && i+1 < len(text) && text[i+1] == '*':
			start := i
			i += 2
			for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
				i++
			}
			if i+1 < len(text) {
				i += 2
			} else {
				i = len(text)
			}
			addSpan(spans, start, i, "syntax.comment")
			prev = 'o'
			want = 0
		case c == '/' && i+1 < len(text) && text[i+1] == '=':
			addSpan(spans, i, i+2, "syntax.operator")
			i += 2
			prev = 'o'
			want = 0
		case c == '/' && jsRegexp(prev):
			n, ok := scanJSRegexp(text, i)
			if !ok {
				addSpan(spans, i, i+1, "syntax.operator")
				i++
				prev = 'o'
				want = 0
				continue
			}
			addSpan(spans, i, n, "syntax.string")
			i = n
			prev = 's'
			want = 0
		case c == '\'' || c == '"':
			n := scanJSString(text, i)
			addSpan(spans, i, n, "syntax.string")
			i = n
			prev = 's'
			want = 0
		case c == '`':
			i = scanTemplate(text, i, spans)
			prev = 's'
			want = 0
		case c == '.' && i+1 < len(text) && isDigit(text[i+1]):
			n := scanJSNumber(text, i)
			addSpan(spans, i, n, "syntax.number")
			i = n
			prev = 'n'
			want = 0
		case isDigit(c):
			n := scanJSNumber(text, i)
			addSpan(spans, i, n, "syntax.number")
			i = n
			prev = 'n'
			want = 0
		case isJSIdentStart(c):
			start := i
			i++
			for i < len(text) && isJSIdent(text[i]) {
				i++
			}
			word := string(text[start:i])
			style := jsWord(word)
			switch {
			case style != "":
				addSpan(spans, start, i, style)
				if word == "function" {
					want = 1
				} else if word == "class" {
					want = 2
				} else {
					want = 0
				}
				if style == "syntax.constant" || word == "this" || word == "super" {
					prev = 'i'
				} else {
					prev = 'o'
				}
			case want == 1:
				addSpan(spans, start, i, "syntax.function")
				want = 0
				prev = 'i'
			case want == 2:
				addSpan(spans, start, i, "syntax.type")
				want = 0
				prev = 'i'
			default:
				if jsCallAhead(text, i) {
					addSpan(spans, start, i, "syntax.function")
				}
				want = 0
				prev = 'i'
			}
		case c == '{':
			braces++
			addSpan(spans, i, i+1, "syntax.operator")
			i++
			prev = 'o'
			want = 0
		case c == '}':
			if braces > 0 {
				braces--
			}
			addSpan(spans, i, i+1, "syntax.operator")
			i++
			prev = 'o'
			want = 0
			if openBraces > 0 && braces == 0 {
				return i
			}
		default:
			if n := jsOpLen(text, i); n > 0 {
				addSpan(spans, i, i+n, "syntax.operator")
				i += n
				prev = 'o'
				want = 0
				continue
			}
			if isJSOp(c) {
				addSpan(spans, i, i+1, "syntax.operator")
				if c == ')' || c == ']' {
					prev = c
				} else {
					prev = 'o'
				}
				i++
				want = 0
				continue
			}
			i++
			want = 0
		}
	}
	return i
}

func scanTemplate(text []byte, i int, spans *[]absSpan) int {
	start := i
	i++
	for i < len(text) {
		if text[i] == '\\' && i+1 < len(text) {
			i += 2
			continue
		}
		if text[i] == '`' {
			i++
			addSpan(spans, start, i, "syntax.string")
			return i
		}
		if text[i] == '$' && i+1 < len(text) && text[i+1] == '{' {
			addSpan(spans, start, i, "syntax.string")
			addSpan(spans, i, i+2, "syntax.operator")
			i += 2
			i = scanJSCode(text, i, spans, 1, nil)
			start = i
			continue
		}
		i++
	}
	addSpan(spans, start, i, "syntax.string")
	return i
}

func matchStop(text []byte, i int, stop []byte) bool {
	if len(stop) == 0 || !hasPrefixFold(text[i:], stop) {
		return false
	}
	j := i + len(stop)
	return j >= len(text) || !isIdentByte(text[j])
}

func jsRegexp(prev byte) bool {
	switch prev {
	case 'i', 'n', 's', ')', ']':
		return false
	default:
		return true
	}
}

func scanJSRegexp(text []byte, i int) (int, bool) {
	if i >= len(text) || text[i] != '/' {
		return i, false
	}
	i++
	for i < len(text) && text[i] != '\n' {
		if text[i] == '\\' && i+1 < len(text) && text[i+1] != '\n' {
			i += 2
			continue
		}
		if text[i] == '[' {
			i++
			for i < len(text) && text[i] != '\n' && text[i] != ']' {
				if text[i] == '\\' && i+1 < len(text) && text[i+1] != '\n' {
					i += 2
					continue
				}
				i++
			}
			if i < len(text) && text[i] == ']' {
				i++
			}
			continue
		}
		if text[i] == '/' {
			i++
			for i < len(text) && isRegexpFlag(text[i]) {
				i++
			}
			return i, true
		}
		i++
	}
	return i, false
}

func isRegexpFlag(b byte) bool {
	switch b {
	case 'd', 'g', 'i', 'm', 's', 'u', 'v', 'y':
		return true
	default:
		return false
	}
}

func scanJSString(text []byte, i int) int {
	if i >= len(text) {
		return i
	}
	q := text[i]
	i++
	for i < len(text) && text[i] != '\n' {
		if text[i] == '\\' && i+1 < len(text) && text[i+1] != '\n' {
			i += 2
			continue
		}
		if text[i] == q {
			return i + 1
		}
		i++
	}
	return i
}

func scanJSNumber(text []byte, i int) int {
	if i >= len(text) {
		return i
	}
	if text[i] == '0' && i+1 < len(text) {
		switch text[i+1] {
		case 'x', 'X', 'b', 'B', 'o', 'O':
			j := i + 2
			for j < len(text) && isHex(text[j]) {
				j++
			}
			if j == i+2 {
				return i + 1
			}
			if j < len(text) && text[j] == 'n' {
				j++
			}
			return j
		}
	}
	if text[i] == '.' {
		if i+1 >= len(text) || !isDigit(text[i+1]) {
			return i
		}
	}
	start := i
	for i < len(text) && isDigit(text[i]) {
		i++
	}
	if i < len(text) && text[i] == '.' && i+1 < len(text) && isDigit(text[i+1]) {
		i++
		for i < len(text) && isDigit(text[i]) {
			i++
		}
	}
	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		j := i + 1
		if j < len(text) && (text[j] == '+' || text[j] == '-') {
			j++
		}
		if j < len(text) && isDigit(text[j]) {
			i = j
			for i < len(text) && isDigit(text[i]) {
				i++
			}
		}
	}
	if i > start && i < len(text) && text[i] == 'n' {
		i++
	}
	return i
}

func isJSIdentStart(b byte) bool {
	return b == '_' || b == '$' || isLetter(b)
}

func isJSIdent(b byte) bool {
	return isJSIdentStart(b) || isDigit(b)
}

func jsCallAhead(text []byte, i int) bool {
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return i < len(text) && text[i] == '('
}

func jsWord(word string) string {
	switch word {
	case "true", "false", "null", "undefined":
		return "syntax.constant"
	case "await", "break", "case", "catch", "class", "const", "continue", "debugger",
		"default", "delete", "do", "else", "enum", "export", "extends", "finally",
		"for", "function", "if", "implements", "import", "in", "instanceof",
		"interface", "let", "new", "package", "private", "protected", "public",
		"return", "static", "super", "switch", "this", "throw", "try", "typeof",
		"var", "void", "while", "with", "yield", "of", "as", "from", "async":
		return "syntax.keyword"
	default:
		return ""
	}
}

func jsOpLen(text []byte, i int) int {
	rest := text[i:]
	switch {
	case hasBytes(rest, "==="), hasBytes(rest, "!=="), hasBytes(rest, ">>>"),
		hasBytes(rest, "<<="), hasBytes(rest, ">>="), hasBytes(rest, "**="):
		return 3
	case hasBytes(rest, "=="), hasBytes(rest, "!="), hasBytes(rest, "<="),
		hasBytes(rest, ">="), hasBytes(rest, "&&"), hasBytes(rest, "||"),
		hasBytes(rest, "??"), hasBytes(rest, "++"), hasBytes(rest, "--"),
		hasBytes(rest, "+="), hasBytes(rest, "-="), hasBytes(rest, "*="),
		hasBytes(rest, "/="), hasBytes(rest, "%="), hasBytes(rest, "=>"),
		hasBytes(rest, "<<"), hasBytes(rest, ">>"), hasBytes(rest, "**"),
		hasBytes(rest, "?."), hasBytes(rest, "&="), hasBytes(rest, "|="),
		hasBytes(rest, "^="):
		return 2
	default:
		return 0
	}
}

func hasBytes(s []byte, prefix string) bool {
	return len(s) >= len(prefix) && string(s[:len(prefix)]) == prefix
}

func isJSOp(b byte) bool {
	switch b {
	case '(', ')', '[', ']', '{', '}', '.', ',', ';', '?', ':', '~', '^', '!', '%', '&', '|', '+', '-', '*', '=', '<', '>', '/':
		return true
	default:
		return false
	}
}

// HTML.

func highlightHTML(text []byte) []absSpan {
	var spans []absSpan
	i := 0
	for i < len(text) {
		if text[i] != '<' {
			if text[i] == '&' {
				if n := scanEntity(text, i); n > i {
					addSpan(&spans, i, n, "syntax.constant")
					i = n
					continue
				}
			}
			i++
			continue
		}
		if hasPrefixFold(text[i:], []byte("<!--")) {
			end := len(text)
			if j := indexBytes(text[i+4:], []byte("-->")); j >= 0 {
				end = i + 4 + j + 3
			}
			addSpan(&spans, i, end, "syntax.comment")
			i = end
			continue
		}
		if i+1 < len(text) && (text[i+1] == '!' || text[i+1] == '?') {
			end := len(text)
			if j := indexByte(text[i:], '>'); j >= 0 {
				end = i + j + 1
			}
			addSpan(&spans, i, end, "syntax.keyword")
			i = end
			continue
		}
		next, name, open := scanHTMLTag(text, i, &spans)
		if next <= i {
			i++
			continue
		}
		i = next
		if !open {
			continue
		}
		switch name {
		case "script":
			js, n := highlightJS(text[i:], []byte("</script"))
			for _, sp := range js {
				addSpan(&spans, sp.start+i, sp.end+i, sp.style)
			}
			i += n
		case "style":
			if rel := indexFold(text[i:], "</style"); rel >= 0 {
				i += rel
			} else {
				i = len(text)
			}
		}
	}
	return spans
}

func scanHTMLTag(text []byte, i int, spans *[]absSpan) (int, string, bool) {
	start := i
	i++
	closing := false
	if i < len(text) && text[i] == '/' {
		closing = true
		i++
	}
	addSpan(spans, start, i, "syntax.operator")
	nameStart := i
	i = scanHTMLName(text, i)
	name := strings.ToLower(string(text[nameStart:i]))
	if nameStart < i {
		addSpan(spans, nameStart, i, "syntax.keyword")
	}
	self := false
	for i < len(text) && text[i] != '>' && text[i] != '<' {
		if text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r' {
			i++
			continue
		}
		if text[i] == '/' {
			self = true
			addSpan(spans, i, i+1, "syntax.operator")
			i++
			continue
		}
		attr := i
		i = scanHTMLName(text, i)
		if i == attr {
			i++
			continue
		}
		addSpan(spans, attr, i, "syntax.property")
		j := i
		for j < len(text) && (text[j] == ' ' || text[j] == '\t' || text[j] == '\n' || text[j] == '\r') {
			j++
		}
		if j >= len(text) || text[j] != '=' {
			i = j
			continue
		}
		addSpan(spans, j, j+1, "syntax.operator")
		i = j + 1
		for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
			i++
		}
		if i >= len(text) {
			break
		}
		if text[i] == '"' || text[i] == '\'' {
			q := text[i]
			v := i
			i++
			for i < len(text) && text[i] != q {
				i++
			}
			if i < len(text) {
				i++
			}
			addSpan(spans, v, i, "syntax.string")
			continue
		}
		v := i
		for i < len(text) && text[i] != ' ' && text[i] != '\t' && text[i] != '\n' && text[i] != '\r' && text[i] != '>' && text[i] != '/' {
			i++
		}
		addSpan(spans, v, i, "syntax.string")
	}
	if i < len(text) && text[i] == '>' {
		addSpan(spans, i, i+1, "syntax.operator")
		i++
	}
	return i, name, !closing && !self && name != ""
}

func scanHTMLName(text []byte, i int) int {
	if i >= len(text) || !isLetter(text[i]) {
		return i
	}
	i++
	for i < len(text) && (isIdentByte(text[i]) || text[i] == '-' || text[i] == ':') {
		i++
	}
	return i
}

func scanEntity(text []byte, i int) int {
	if i >= len(text) || text[i] != '&' {
		return i
	}
	j := i + 1
	if j < len(text) && text[j] == '#' {
		j++
		hex := false
		if j < len(text) && (text[j] == 'x' || text[j] == 'X') {
			hex = true
			j++
		}
		start := j
		for j < len(text) && (isDigit(text[j]) || (hex && isHex(text[j]))) {
			j++
		}
		if j > start && j < len(text) && text[j] == ';' {
			return j + 1
		}
		return i
	}
	start := j
	for j < len(text) && isIdentByte(text[j]) {
		j++
	}
	if j > start && j < len(text) && text[j] == ';' {
		return j + 1
	}
	return i
}

func indexBytes(s, needle []byte) int {
	return strings.Index(string(s), string(needle))
}

func indexByte(s []byte, c byte) int {
	for i, b := range s {
		if b == c {
			return i
		}
	}
	return -1
}

func indexFold(s []byte, needle string) int {
	n := []byte(needle)
	if len(n) == 0 || len(s) < len(n) {
		return -1
	}
	for i := 0; i+len(n) <= len(s); i++ {
		if hasPrefixFold(s[i:], n) {
			j := i + len(n)
			if j >= len(s) || !isIdentByte(s[j]) {
				return i
			}
		}
	}
	return -1
}
