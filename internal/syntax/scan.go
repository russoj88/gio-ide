package syntax

import (
	"path/filepath"
	"strings"
)

// Lang reports the highlight language for path.
// Go source is "go" (tree-sitter). go.mod is "gomod", go.sum is "gosum",
// YAML is "yaml", JSON is "json", Dockerfiles are "docker", env files are
// "env", JavaScript is "javascript", HTML is "html", and SQL is "sql"
// (.sql, .pgsql, and .psql, highlighted as PostgreSQL). Anything else is
// empty, and the syntax worker leaves it uncolored.
func Lang(p string) string {
	base := strings.ToLower(filepath.Base(p))
	switch base {
	case "go.mod":
		return "gomod"
	case "go.sum":
		return "gosum"
	}
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return "docker"
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return "env"
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".go":
		return "go"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".dockerfile":
		return "docker"
	case ".env":
		return "env"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".html", ".htm":
		return "html"
	case ".sql", ".pgsql", ".psql":
		return "sql"
	default:
		return ""
	}
}

// Scan colors lines [first, last) of text. Columns are byte offsets within
// the line. "go", "", and unknown languages return nil so the tree-sitter
// path keeps Go. The slice length is the number of lines in the window.
// YAML, Docker, env, JavaScript, HTML, and SQL carry state from the start
// of the text, so a window that begins mid-document still closes strings,
// comments, and tags correctly.
func Scan(lang string, text []byte, first, last int) [][]StyleSpan {
	switch lang {
	case "gomod", "gosum", "json", "yaml", "docker", "env", "javascript", "html", "sql":
	default:
		return nil
	}
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
	switch lang {
	case "gomod":
		for i := first; i < last; i++ {
			out[i-first] = scanGoMod(lines[i])
		}
	case "gosum":
		for i := first; i < last; i++ {
			out[i-first] = scanGoSum(lines[i])
		}
	case "json":
		for i := first; i < last; i++ {
			out[i-first] = scanJSON(lines[i])
		}
	case "yaml":
		st := yamlCarry{block: -1}
		for i := 0; i < last; i++ {
			spans := scanYAMLLine(lines[i], &st)
			if i >= first {
				out[i-first] = spans
			}
		}
	case "docker":
		var st dockerCarry
		for i := 0; i < last; i++ {
			spans := scanDockerLine(lines[i], &st)
			if i >= first {
				out[i-first] = spans
			}
		}
	case "env":
		var st envCarry
		for i := 0; i < last; i++ {
			spans := scanEnvLine(lines[i], &st)
			if i >= first {
				out[i-first] = spans
			}
		}
	case "javascript":
		spans, _ := highlightJS(text, nil)
		return clipSpans(text, spans, first, last)
	case "html":
		return clipSpans(text, highlightHTML(text), first, last)
	case "sql":
		return clipSpans(text, highlightSQL(text), first, last)
	}
	return out
}

// splitScanLines matches lineCount: a trailing newline yields an empty last
// line, and empty text is one empty line.
func splitScanLines(text []byte) [][]byte {
	if len(text) == 0 {
		return [][]byte{nil}
	}
	var lines [][]byte
	start := 0
	for i, c := range text {
		if c == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
		}
	}
	lines = append(lines, text[start:])
	return lines
}

func scanGoMod(line []byte) []StyleSpan {
	var out []StyleSpan
	i := 0
	for i < len(line) {
		switch line[i] {
		case ' ', '\t', '\r':
			i++
			continue
		}
		if i+1 < len(line) && line[i] == '/' && line[i+1] == '/' {
			out = append(out, StyleSpan{Start: i, End: len(line), Style: "syntax.comment"})
			break
		}
		if line[i] == '"' || line[i] == '`' {
			out, i = appendQuoted(out, line, i)
			continue
		}
		if i+1 < len(line) && line[i] == '=' && line[i+1] == '>' {
			out = append(out, StyleSpan{Start: i, End: i + 2, Style: "syntax.operator"})
			i += 2
			continue
		}
		start := i
		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		tok := string(line[start:i])
		switch tok {
		case "module", "go", "toolchain", "require", "replace", "exclude", "retract", "ignore":
			out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.keyword"})
		default:
			if isModuleVersion(tok) {
				out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.number"})
			}
		}
	}
	return out
}

func scanGoSum(line []byte) []StyleSpan {
	var out []StyleSpan
	field := 0
	i := 0
	for i < len(line) {
		if line[i] == ' ' || line[i] == '\t' {
			i++
			continue
		}
		start := i
		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		tok := string(line[start:i])
		if field == 1 && isModuleVersion(tok) {
			out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.number"})
		} else if strings.HasPrefix(tok, "h1:") {
			out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.string"})
		}
		field++
	}
	return out
}

func isModuleVersion(tok string) bool {
	s := tok
	if len(s) > 1 && (s[0] == 'v' || s[0] == 'V') {
		s = s[1:]
	}
	if len(s) == 0 || s[0] < '0' || s[0] > '9' {
		return false
	}
	dot := false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c == '.':
			dot = true
		case c == '-' || c == '+' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		default:
			return false
		}
	}
	return dot
}

func appendQuoted(out []StyleSpan, line []byte, i int) ([]StyleSpan, int) {
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
			break
		}
		i++
	}
	return append(out, StyleSpan{Start: start, End: i, Style: "syntax.string"}), i
}

func scanJSON(line []byte) []StyleSpan {
	var out []StyleSpan
	i := 0
	for i < len(line) {
		c := line[i]
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		switch c {
		case '{', '}', '[', ']', ':', ',':
			out = append(out, StyleSpan{Start: i, End: i + 1, Style: "syntax.operator"})
			i++
		case '"':
			start := i
			i++
			for i < len(line) {
				if line[i] == '\\' && i+1 < len(line) {
					i += 2
					continue
				}
				if line[i] == '"' {
					i++
					break
				}
				i++
			}
			j := i
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			style := "syntax.string"
			if j < len(line) && line[j] == ':' {
				style = "syntax.property"
			}
			out = append(out, StyleSpan{Start: start, End: i, Style: style})
		default:
			if isJSONNumberStart(line, i) {
				start := i
				i = scanJSONNumber(line, i)
				out = append(out, StyleSpan{Start: start, End: i, Style: "syntax.number"})
				continue
			}
			if n, ok := jsonLiteral(line, i); ok {
				out = append(out, StyleSpan{Start: i, End: i + n, Style: "syntax.constant"})
				i += n
				continue
			}
			i++
		}
	}
	return out
}

func isJSONNumberStart(line []byte, i int) bool {
	if line[i] == '-' {
		return i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9'
	}
	return line[i] >= '0' && line[i] <= '9'
}

func scanJSONNumber(line []byte, i int) int {
	if line[i] == '-' {
		i++
	}
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i < len(line) && line[i] == '.' {
		i++
		for i < len(line) && line[i] >= '0' && line[i] <= '9' {
			i++
		}
	}
	if i < len(line) && (line[i] == 'e' || line[i] == 'E') {
		j := i + 1
		if j < len(line) && (line[j] == '+' || line[j] == '-') {
			j++
		}
		if j < len(line) && line[j] >= '0' && line[j] <= '9' {
			i = j
			for i < len(line) && line[i] >= '0' && line[i] <= '9' {
				i++
			}
		}
	}
	return i
}

func jsonLiteral(line []byte, i int) (int, bool) {
	for _, lit := range []string{"true", "false", "null"} {
		n := len(lit)
		if i+n > len(line) || string(line[i:i+n]) != lit {
			continue
		}
		if i+n < len(line) && isIdentByte(line[i+n]) {
			continue
		}
		return n, true
	}
	return 0, false
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// yamlCarry tracks quotes and block scalars that cross lines.
type yamlCarry struct {
	quote byte
	block int
}

func scanYAMLLine(line []byte, st *yamlCarry) []StyleSpan {
	if st.quote != 0 {
		return scanYAMLOpenQuote(line, st)
	}
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	trim := trimSpace(line)
	if st.block >= 0 {
		if len(trim) == 0 || indent > st.block {
			if len(line) == 0 {
				return nil
			}
			return []StyleSpan{{Start: 0, End: len(line), Style: "syntax.string"}}
		}
		st.block = -1
	}
	if len(trim) == 0 {
		return nil
	}
	if trim[0] == '#' {
		return []StyleSpan{{Start: indent, End: len(line), Style: "syntax.comment"}}
	}
	if isYAMLDoc(trim) {
		at := bytesIndex(line, trim)
		return []StyleSpan{{Start: at, End: at + len(trim), Style: "syntax.keyword"}}
	}
	var spans []StyleSpan
	colon := yamlKeyColon(line)
	i := indent
	if colon >= 0 {
		if i < colon && (line[i] == '-' || line[i] == '*' || line[i] == '+') && i+1 <= colon && i+1 < len(line) && line[i+1] == ' ' {
			spans = append(spans, StyleSpan{Start: i, End: i + 1, Style: "syntax.operator"})
			i += 2
			for i < colon && (line[i] == ' ' || line[i] == '\t') {
				i++
			}
		}
		if i < colon {
			spans = append(spans, StyleSpan{Start: i, End: colon, Style: "syntax.property"})
		}
		spans = append(spans, StyleSpan{Start: colon, End: colon + 1, Style: "syntax.operator"})
		i = colon + 1
		if yamlBlockValue(line[i:]) {
			st.block = indent
			return spans
		}
	} else if i < len(line) && (line[i] == '-' || line[i] == '*' || line[i] == '+') && i+1 < len(line) && line[i+1] == ' ' {
		spans = append(spans, StyleSpan{Start: i, End: i + 1, Style: "syntax.operator"})
		i += 2
	}
	spans = append(spans, scanYAMLValue(line, i, st)...)
	return spans
}

func scanYAMLOpenQuote(line []byte, st *yamlCarry) []StyleSpan {
	q := st.quote
	j := 0
	for j < len(line) {
		if q == '"' && line[j] == '\\' && j+1 < len(line) {
			j += 2
			continue
		}
		if line[j] == q {
			j++
			st.quote = 0
			break
		}
		j++
	}
	if st.quote != 0 {
		if len(line) == 0 {
			return nil
		}
		return []StyleSpan{{Start: 0, End: len(line), Style: "syntax.string"}}
	}
	spans := []StyleSpan{{Start: 0, End: j, Style: "syntax.string"}}
	if j >= len(line) {
		return spans
	}
	rest := scanYAMLLine(line[j:], st)
	for _, sp := range rest {
		spans = append(spans, StyleSpan{Start: sp.Start + j, End: sp.End + j, Style: sp.Style})
	}
	return spans
}

func yamlKeyColon(line []byte) int {
	in := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if in != 0 {
			if c == '\\' && in == '"' && i+1 < len(line) {
				i++
				continue
			}
			if c == in {
				in = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			in = c
			continue
		}
		if c == '#' {
			return -1
		}
		if c == ':' && (i+1 >= len(line) || line[i+1] != '/') {
			return i
		}
	}
	return -1
}

func yamlBlockValue(val []byte) bool {
	i := 0
	for i < len(val) && (val[i] == ' ' || val[i] == '\t') {
		i++
	}
	if i >= len(val) || (val[i] != '|' && val[i] != '>') {
		return false
	}
	for _, c := range val[i+1:] {
		if c == '#' {
			break
		}
		if c == ' ' || c == '\t' || c == '+' || c == '-' || (c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

func scanYAMLValue(line []byte, i int, st *yamlCarry) []StyleSpan {
	var spans []StyleSpan
	for i < len(line) {
		c := line[i]
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		if c == '#' {
			spans = append(spans, StyleSpan{Start: i, End: len(line), Style: "syntax.comment"})
			break
		}
		if c == '"' || c == '\'' {
			start := i
			q := c
			i++
			closed := false
			for i < len(line) {
				if q == '"' && line[i] == '\\' && i+1 < len(line) {
					i += 2
					continue
				}
				if line[i] == q {
					i++
					closed = true
					break
				}
				i++
			}
			spans = append(spans, StyleSpan{Start: start, End: i, Style: "syntax.string"})
			if !closed {
				st.quote = q
			}
			continue
		}
		start := i
		for i < len(line) && line[i] != ' ' && line[i] != '\t' && line[i] != '#' {
			i++
		}
		tok := string(line[start:i])
		switch tok {
		case "true", "false", "null", "yes", "no", "on", "off", "~":
			spans = append(spans, StyleSpan{Start: start, End: i, Style: "syntax.constant"})
		default:
			if isPlainNumber(tok) {
				spans = append(spans, StyleSpan{Start: start, End: i, Style: "syntax.number"})
			}
		}
	}
	return spans
}

func isPlainNumber(tok string) bool {
	if tok == "" {
		return false
	}
	i := 0
	if tok[0] == '-' || tok[0] == '+' {
		if len(tok) == 1 {
			return false
		}
		i++
	}
	if i >= len(tok) || tok[i] < '0' || tok[i] > '9' {
		return false
	}
	dot := false
	for ; i < len(tok); i++ {
		c := tok[i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.' && !dot:
			dot = true
		case c == 'e' || c == 'E':
			return i > 0 && scanExponent(tok[i+1:])
		default:
			return false
		}
	}
	return true
}

func scanExponent(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	if i >= len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isYAMLDoc(trim []byte) bool {
	return string(trim) == "---" || string(trim) == "..."
}

func trimSpace(line []byte) []byte {
	i := 0
	j := len(line)
	for i < j && (line[i] == ' ' || line[i] == '\t' || line[i] == '\r') {
		i++
	}
	for j > i && (line[j-1] == ' ' || line[j-1] == '\t' || line[j-1] == '\r') {
		j--
	}
	return line[i:j]
}

func bytesIndex(hay, needle []byte) int {
	return strings.Index(string(hay), string(needle))
}
