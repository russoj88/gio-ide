package git

import (
	"strings"
	"time"

	"github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// Mark is one gutter flag. Line is 0-based in the live buffer.
// Kind is 'a' added, 'm' modified, or 'd' deleted.
type Mark struct {
	Line int
	Kind byte
}

// DiffMarks compares the index blob with the live buffer.
// The diff is limited to 200 ms.
func DiffMarks(indexText, live string) []Mark {
	if indexText == live {
		return nil
	}
	parts := diff.DoWithTimeout(indexText, live, 200*time.Millisecond)
	var out []Mark
	line := 0
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		switch p.Type {
		case diffmatchpatch.DiffEqual:
			line += countLines(p.Text)
		case diffmatchpatch.DiffDelete:
			if i+1 < len(parts) && parts[i+1].Type == diffmatchpatch.DiffInsert {
				n := countLines(parts[i+1].Text)
				for k := range n {
					out = append(out, Mark{Line: line + k, Kind: 'm'})
				}
				if n == 0 {
					out = append(out, Mark{Line: line, Kind: 'd'})
				} else if countLines(p.Text) > n {
					out[len(out)-1].Kind = 'm'
				}
				line += n
				i++
				continue
			}
			at := line
			if at > 0 {
				at--
			}
			out = append(out, Mark{Line: at, Kind: 'd'})
		case diffmatchpatch.DiffInsert:
			n := countLines(p.Text)
			if n == 0 {
				n = 1
			}
			for k := 0; k < n; k++ {
				out = append(out, Mark{Line: line + k, Kind: 'a'})
			}
			line += n
		}
	}
	return out
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if strings.HasSuffix(s, "\n") {
		return n
	}
	return n + 1
}
