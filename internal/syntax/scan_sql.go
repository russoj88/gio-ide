package syntax

// highlightSQL colors text with PostgreSQL lexical rules.
// Block comments nest. Dollar-quoted bodies are scanned again so a function
// body keeps keyword color; the quote delimiters stay strings. A data literal
// that contains keywords is colored the same way.
func highlightSQL(text []byte) []absSpan {
	var spans []absSpan
	scanSQL(text, 0, len(text), &spans)
	return spans
}

// scanSQL colors text[i:end]. end may stop before a closing dollar quote.
func scanSQL(text []byte, i, end int, spans *[]absSpan) {
	if end > len(text) {
		end = len(text)
	}
	for i < end {
		c := text[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if i+1 < end && c == '-' && text[i+1] == '-' {
			i = scanSQLLineComment(text, i, end, spans)
			continue
		}
		if i+1 < end && c == '/' && text[i+1] == '*' {
			i = scanSQLBlockComment(text, i, end, spans)
			continue
		}
		if isSQLStringPrefix(c) && i+1 < end && text[i+1] == '\'' {
			i = scanSQLString(text, i, i+1, end, spans, c == 'E' || c == 'e')
			continue
		}
		if (c == 'U' || c == 'u') && i+2 < end && text[i+1] == '&' {
			switch text[i+2] {
			case '\'':
				i = scanSQLString(text, i, i+2, end, spans, false)
				continue
			case '"':
				i = scanSQLIdentQuote(text, i, i+2, end, spans)
				continue
			}
		}
		switch c {
		case '\'':
			i = scanSQLString(text, i, i, end, spans, false)
			continue
		case '"':
			i = scanSQLIdentQuote(text, i, i, end, spans)
			continue
		}
		if c == '$' {
			if i+1 < end && isDigit(text[i+1]) {
				j := i + 2
				for j < end && isDigit(text[j]) {
					j++
				}
				addSpan(spans, i, j, "syntax.number")
				i = j
				continue
			}
			if j, ok := scanSQLDollar(text, i, end, spans); ok {
				i = j
				continue
			}
			i++
			continue
		}
		if isDigit(c) || (c == '.' && i+1 < end && isDigit(text[i+1])) {
			j := scanSQLNumber(text, i, end)
			if j > i {
				addSpan(spans, i, j, "syntax.number")
				i = j
				continue
			}
		}
		if isSQLIdentStart(c) {
			j := i + 1
			for j < end && isSQLIdentCont(text[j]) {
				j++
			}
			if style := sqlWord(string(text[i:j])); style != "" {
				addSpan(spans, i, j, style)
			}
			i = j
			continue
		}
		if c == ':' && i+1 < end && (text[i+1] == ':' || text[i+1] == '=') {
			addSpan(spans, i, i+2, "syntax.operator")
			i += 2
			continue
		}
		if c == '.' && i+1 < end && text[i+1] == '.' {
			addSpan(spans, i, i+2, "syntax.operator")
			i += 2
			continue
		}
		if isSQLOpChar(c) {
			j := scanSQLOp(text, i, end)
			if j > i {
				addSpan(spans, i, j, "syntax.operator")
				i = j
				continue
			}
		}
		i++
	}
}

func scanSQLLineComment(text []byte, i, end int, spans *[]absSpan) int {
	j := i + 2
	for j < end && text[j] != '\n' {
		j++
	}
	addSpan(spans, i, j, "syntax.comment")
	return j
}

// scanSQLBlockComment colors a nested /* */ comment. An unclosed comment
// runs to end.
func scanSQLBlockComment(text []byte, i, end int, spans *[]absSpan) int {
	start := i
	i += 2
	depth := 1
	for i < end && depth > 0 {
		if i+1 < end && text[i] == '/' && text[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if i+1 < end && text[i] == '*' && text[i+1] == '/' {
			depth--
			i += 2
			continue
		}
		i++
	}
	addSpan(spans, start, i, "syntax.comment")
	return i
}

// scanSQLString colors a quoted literal. start is the first byte of the
// token, so a prefix such as E or U& is included. quote is the opening
// quote. escape is set for E'...' strings, where a backslash skips the next
// byte. Standard strings treat a backslash as an ordinary character and
// escape a quote by doubling it. The literal may cross lines.
func scanSQLString(text []byte, start, quote, end int, spans *[]absSpan, escape bool) int {
	i := quote + 1
	for i < end {
		if escape && text[i] == '\\' && i+1 < end {
			i += 2
			continue
		}
		if text[i] == '\'' {
			if i+1 < end && text[i+1] == '\'' {
				i += 2
				continue
			}
			i++
			addSpan(spans, start, i, "syntax.string")
			return i
		}
		i++
	}
	addSpan(spans, start, i, "syntax.string")
	return i
}

// scanSQLIdentQuote colors a double-quoted identifier. "" is one quote.
func scanSQLIdentQuote(text []byte, start, quote, end int, spans *[]absSpan) int {
	i := quote + 1
	for i < end {
		if text[i] == '"' {
			if i+1 < end && text[i+1] == '"' {
				i += 2
				continue
			}
			i++
			addSpan(spans, start, i, "syntax.property")
			return i
		}
		i++
	}
	addSpan(spans, start, i, "syntax.property")
	return i
}

// scanSQLDollar colors a $tag$ ... $tag$ or $$ ... $$ quote.
// The tag is an identifier with no '$' and is case-sensitive.
// The body is scanned as SQL. An unclosed quote runs to end as a string.
// The bool is false when the '$' is not a dollar quote.
func scanSQLDollar(text []byte, i, end int, spans *[]absSpan) (int, bool) {
	j := i + 1
	tagStart := j
	if j < end && isSQLIdentStart(text[j]) {
		j++
		for j < end && isSQLTagCont(text[j]) {
			j++
		}
	}
	if j >= end || text[j] != '$' {
		return i, false
	}
	tag := text[tagStart:j]
	openEnd := j + 1
	closeAt := findSQLDollarClose(text, openEnd, end, tag)
	if closeAt < 0 {
		addSpan(spans, i, end, "syntax.string")
		return end, true
	}
	addSpan(spans, i, openEnd, "syntax.string")
	scanSQL(text, openEnd, closeAt, spans)
	closeEnd := closeAt + len(tag) + 2
	addSpan(spans, closeAt, closeEnd, "syntax.string")
	return closeEnd, true
}

func findSQLDollarClose(text []byte, from, end int, tag []byte) int {
	n := len(tag) + 2
	for i := from; i+n <= end; i++ {
		if text[i] != '$' || text[i+n-1] != '$' {
			continue
		}
		match := true
		for k := range tag {
			if text[i+1+k] != tag[k] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// scanSQLNumber returns the end of a numeric literal, or i when text[i]
// is not one. Leading + and - are operators. Underscores may sit between
// digits. 1..2 keeps both dots out of the number. 0x, 0o, and 0b are
// non-decimal integers.
func scanSQLNumber(text []byte, i, end int) int {
	if i+1 < end && text[i] == '0' {
		var j int
		ok := false
		switch text[i+1] {
		case 'x', 'X':
			j, ok = scanSQLRadix(text, i+2, end, isHex)
		case 'o', 'O':
			j, ok = scanSQLRadix(text, i+2, end, isOct)
		case 'b', 'B':
			j, ok = scanSQLRadix(text, i+2, end, isBin)
		}
		if ok {
			return j
		}
	}
	if text[i] == '.' {
		j := scanSQLDigits(text, i+1, end)
		if j == i+1 {
			return i
		}
		return scanSQLExponent(text, j, end)
	}
	j := scanSQLDigits(text, i, end)
	if j == i {
		return i
	}
	if j < end && text[j] == '.' && !(j+1 < end && text[j+1] == '.') {
		if j+1 < end && isDigit(text[j+1]) {
			j = scanSQLDigits(text, j+1, end)
		} else {
			j++
		}
	}
	return scanSQLExponent(text, j, end)
}

func scanSQLDigits(text []byte, i, end int) int {
	if i >= end || !isDigit(text[i]) {
		return i
	}
	i++
	for i < end {
		if text[i] == '_' {
			if i+1 < end && isDigit(text[i+1]) {
				i += 2
				continue
			}
			break
		}
		if !isDigit(text[i]) {
			break
		}
		i++
	}
	return i
}

func scanSQLRadix(text []byte, i, end int, digit func(byte) bool) (int, bool) {
	j := i
	saw := false
	for j < end {
		k := j
		if text[k] == '_' {
			k++
		}
		if k >= end || !digit(text[k]) {
			break
		}
		saw = true
		j = k + 1
	}
	if !saw {
		return i, false
	}
	return j, true
}

func scanSQLExponent(text []byte, i, end int) int {
	if i >= end || (text[i] != 'e' && text[i] != 'E') {
		return i
	}
	j := i + 1
	if j < end && (text[j] == '+' || text[j] == '-') {
		j++
	}
	k := scanSQLDigits(text, j, end)
	if k == j {
		return i
	}
	return k
}

// scanSQLOp returns the end of one operator starting at i.
// The run stops before -- or /*. A multi-character operator cannot end in
// + or - unless it also contains ~ ! @ # % ^ & | ` or ?.
func scanSQLOp(text []byte, i, end int) int {
	start := i
	special := false
	for i < end && isSQLOpChar(text[i]) {
		if text[i] == '-' && i+1 < end && text[i+1] == '-' {
			break
		}
		if text[i] == '/' && i+1 < end && text[i+1] == '*' {
			break
		}
		if sqlOpSpecial(text[i]) {
			special = true
		}
		i++
	}
	if !special {
		for i-start > 1 && (text[i-1] == '+' || text[i-1] == '-') {
			i--
		}
	}
	return i
}

func isSQLStringPrefix(b byte) bool {
	switch b {
	case 'E', 'e', 'B', 'b', 'X', 'x', 'N', 'n':
		return true
	default:
		return false
	}
}

func isSQLIdentStart(b byte) bool {
	return isLetter(b) || b == '_' || b >= 0x80
}

func isSQLIdentCont(b byte) bool {
	return isSQLIdentStart(b) || isDigit(b) || b == '$'
}

func isSQLTagCont(b byte) bool {
	return isSQLIdentStart(b) || isDigit(b)
}

func isSQLOpChar(b byte) bool {
	switch b {
	case '+', '-', '*', '/', '<', '>', '=', '~', '!', '@',
		'#', '%', '^', '&', '|', '`', '?':
		return true
	default:
		return false
	}
}

func sqlOpSpecial(b byte) bool {
	switch b {
	case '~', '!', '@', '#', '%', '^', '&', '|', '`', '?':
		return true
	default:
		return false
	}
}

func isOct(b byte) bool {
	return b >= '0' && b <= '7'
}

func isBin(b byte) bool {
	return b == '0' || b == '1'
}
