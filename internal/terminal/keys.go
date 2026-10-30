package terminal

import "strings"

// EncodeKey maps a gio-ide key name to the bytes the PTY should receive.
// appCursor selects the DECCKM sequences. text is the printable insertion.
func EncodeKey(name, text string, ctrl, alt, appCursor bool) []byte {
	if ctrl && len(name) == 1 {
		r := name[0]
		if r >= 'a' && r <= 'z' {
			return []byte{r - 'a' + 1}
		}
	}
	seq := ""
	switch name {
	case "enter":
		seq = "\r"
	case "tab":
		seq = "\t"
	case "backspace":
		seq = "\x7f"
	case "escape":
		seq = "\x1b"
	case "up":
		seq = arrow('A', appCursor)
	case "down":
		seq = arrow('B', appCursor)
	case "right":
		seq = arrow('C', appCursor)
	case "left":
		seq = arrow('D', appCursor)
	case "home":
		seq = "\x1b[H"
	case "end":
		seq = "\x1b[F"
	case "delete":
		seq = "\x1b[3~"
	case "pageup":
		seq = "\x1b[5~"
	case "pagedown":
		seq = "\x1b[6~"
	default:
		if strings.HasPrefix(name, "f") && len(name) > 1 {
			seq = functionKey(name)
		}
	}
	if seq == "" {
		seq = text
	}
	if seq == "" {
		return nil
	}
	if alt {
		return append([]byte{0x1b}, seq...)
	}
	return []byte(seq)
}

func arrow(final byte, app bool) string {
	if app {
		return "\x1bO" + string(final)
	}
	return "\x1b[" + string(final)
}

func functionKey(name string) string {
	switch name {
	case "f1":
		return "\x1bOP"
	case "f2":
		return "\x1bOQ"
	case "f3":
		return "\x1bOR"
	case "f4":
		return "\x1bOS"
	case "f5":
		return "\x1b[15~"
	case "f6":
		return "\x1b[17~"
	case "f7":
		return "\x1b[18~"
	case "f8":
		return "\x1b[19~"
	case "f9":
		return "\x1b[20~"
	case "f10":
		return "\x1b[21~"
	case "f11":
		return "\x1b[23~"
	case "f12":
		return "\x1b[24~"
	default:
		return ""
	}
}

// BracketPaste wraps pasted text when mode 2004 is set.
func BracketPaste(text string, on bool) []byte {
	if !on {
		return []byte(text)
	}
	return []byte("\x1b[200~" + text + "\x1b[201~")
}
