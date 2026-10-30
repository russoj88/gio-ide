package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffSpan is a highlighted byte range in one side of a row.
// Kind is '-' on the before side and '+' on the after side.
type DiffSpan struct {
	Start int
	End   int
	Kind  byte
}

// DiffSide is one column of a side-by-side row.
// Line is 1-based. It is 0 when this side is only a pad.
// Kind is ' ' unchanged, '+' added, '-' removed, '~' modified, or 0 for a pad.
// Text is the line without a trailing newline and without a '+' or '-' prefix.
// Spans are intra-line ranges. They are set only for Kind '~'.
type DiffSide struct {
	Line  int
	Text  string
	Kind  byte
	Spans []DiffSpan
}

// DiffRow is one aligned pair. Left is HEAD. Right is the current file.
type DiffRow struct {
	Left, Right DiffSide
}

// Diff is a side-by-side diff of one path. Binary and TooBig leave Rows empty.
type Diff struct {
	Rows   []DiffRow
	Binary bool
	TooBig bool
}

// DiffAgainstHead diffs HEAD against the current bytes of path.
// path is worktree-relative or absolute and goes through rel.
// liveOK true uses live, including an empty slice.
// liveOK false reads the worktree file. A missing file is an empty after side.
// A repository with no commits, or a path that is not in HEAD, is an empty before side.
func (r *Repo) DiffAgainstHead(ctx context.Context, path string, live []byte, liveOK bool) (Diff, error) {
	if err := ctx.Err(); err != nil {
		return Diff{}, err
	}
	rel, err := r.rel(path)
	if err != nil {
		return Diff{}, err
	}
	before, err := r.headFile(rel)
	if err != nil {
		return Diff{}, err
	}
	after, err := r.workFile(rel, live, liveOK)
	if err != nil {
		return Diff{}, err
	}
	if len(before) > 1<<20 || len(after) > 1<<20 {
		return Diff{TooBig: true}, nil
	}
	if bytes.Contains(before, []byte{0}) || bytes.Contains(after, []byte{0}) {
		return Diff{Binary: true}, nil
	}
	if bytes.Equal(before, after) {
		return Diff{}, nil
	}
	return sideBySide(before, after), nil
}

// headFile reads rel from HEAD. A missing HEAD or a missing path is an empty
// before side. Other errors are returned.
func (r *Repo) headFile(rel string) ([]byte, error) {
	ref, err := r.repo.Head()
	if err != nil {
		return nil, nil
	}
	commit, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	f, err := commit.File(rel)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return nil, nil
		}
		return nil, err
	}
	rc, err := f.Reader()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// workFile is the after side. liveOK selects live, including an empty slice.
// A missing worktree file is an empty after side.
func (r *Repo) workFile(rel string, live []byte, liveOK bool) ([]byte, error) {
	if liveOK {
		return live, nil
	}
	b, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// sideBySide keeps every line. A run of unequal lines is one change:
// matching deletes and inserts become '~' rows, then leftover deletes
// and inserts are padded on the other side. Equal lines are copied across.
func sideBySide(before, after []byte) Diff {
	lines := classify(diff.Do(string(before), string(after)))
	rows := make([]DiffRow, 0, len(lines))
	dmp := diffmatchpatch.New()
	leftN, rightN := 1, 1
	for i := 0; i < len(lines); {
		if lines[i].kind == ' ' {
			rows = append(rows, DiffRow{
				Left:  DiffSide{Line: leftN, Text: lines[i].text, Kind: ' '},
				Right: DiffSide{Line: rightN, Text: lines[i].text, Kind: ' '},
			})
			leftN++
			rightN++
			i++
			continue
		}
		j := i + 1
		for j < len(lines) && lines[j].kind != ' ' {
			j++
		}
		var dels, adds []string
		for _, ln := range lines[i:j] {
			switch ln.kind {
			case '-':
				dels = append(dels, ln.text)
			case '+':
				adds = append(adds, ln.text)
			}
		}
		n := min(len(dels), len(adds))
		for k := range n {
			leftSpans, rightSpans := lineSpans(dmp, dels[k], adds[k])
			rows = append(rows, DiffRow{
				Left:  DiffSide{Line: leftN, Text: dels[k], Kind: '~', Spans: leftSpans},
				Right: DiffSide{Line: rightN, Text: adds[k], Kind: '~', Spans: rightSpans},
			})
			leftN++
			rightN++
		}
		for _, text := range dels[n:] {
			rows = append(rows, DiffRow{
				Left: DiffSide{Line: leftN, Text: text, Kind: '-'},
			})
			leftN++
		}
		for _, text := range adds[n:] {
			rows = append(rows, DiffRow{
				Right: DiffSide{Line: rightN, Text: text, Kind: '+'},
			})
			rightN++
		}
		i = j
	}
	return Diff{Rows: rows}
}

// lineSpans highlights the bytes that differ inside one '~' pair.
// Offsets are byte indexes. Empty edits are not emitted.
func lineSpans(dmp *diffmatchpatch.DiffMatchPatch, left, right string) (leftSpans, rightSpans []DiffSpan) {
	parts := dmp.DiffMain(left, right, false)
	parts = dmp.DiffCleanupSemantic(parts)
	var lc, rc int
	for _, part := range parts {
		n := len(part.Text)
		if n == 0 {
			continue
		}
		switch part.Type {
		case diffmatchpatch.DiffEqual:
			lc += n
			rc += n
		case diffmatchpatch.DiffDelete:
			leftSpans = append(leftSpans, DiffSpan{Start: lc, End: lc + n, Kind: '-'})
			lc += n
		case diffmatchpatch.DiffInsert:
			rightSpans = append(rightSpans, DiffSpan{Start: rc, End: rc + n, Kind: '+'})
			rc += n
		}
	}
	return leftSpans, rightSpans
}

// edit is one line from diff.Do, without its trailing newline.
type edit struct {
	kind byte
	text string
}

func classify(parts []diffmatchpatch.Diff) []edit {
	var lines []edit
	for _, p := range parts {
		var kind byte
		switch p.Type {
		case diffmatchpatch.DiffEqual:
			kind = ' '
		case diffmatchpatch.DiffDelete:
			kind = '-'
		case diffmatchpatch.DiffInsert:
			kind = '+'
		default:
			continue
		}
		for _, text := range splitChunk(p.Text) {
			lines = append(lines, edit{kind: kind, text: text})
		}
	}
	return lines
}

// splitChunk splits one diff.Do chunk on newlines.
// A chunk that ends in '\n' does not yield an extra empty line.
// A final line with no newline is still one line.
func splitChunk(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		parts = parts[:len(parts)-1]
	}
	return parts
}
