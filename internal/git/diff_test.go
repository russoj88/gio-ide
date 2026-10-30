package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gogit "github.com/go-git/go-git/v5"
)

func TestDiffAgainstHead(t *testing.T) {
	ctx := context.Background()

	t.Run("modified", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "a.txt", "alpha\nbeta\ngamma\n")
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\nBETA\ngamma\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, err := repo.DiffAgainstHead(ctx, filepath.Join(dir, "a.txt"), nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if d.Binary || d.TooBig {
			t.Fatalf("flags %+v", d)
		}
		if len(d.Rows) != 3 {
			t.Fatalf("rows %d:\n%s", len(d.Rows), formatRows(d.Rows))
		}
		assertContext(t, d.Rows[0], "alpha", 1, 1)
		row := d.Rows[1]
		if row.Left.Kind != '~' || row.Right.Kind != '~' {
			t.Fatalf("row 1 kinds %q %q", row.Left.Kind, row.Right.Kind)
		}
		if row.Left.Text != "beta" || row.Right.Text != "BETA" {
			t.Fatalf("row 1 text %q %q", row.Left.Text, row.Right.Text)
		}
		if row.Left.Line != 2 || row.Right.Line != 2 {
			t.Fatalf("row 1 lines %d %d", row.Left.Line, row.Right.Line)
		}
		assertHighlight(t, row.Left, '-')
		assertHighlight(t, row.Right, '+')
		assertContext(t, d.Rows[2], "gamma", 3, 3)
		for _, row := range d.Rows {
			for _, text := range []string{row.Left.Text, row.Right.Text} {
				if strings.Contains(text, "/dev/null") || strings.Contains(text, "@@") {
					t.Fatalf("marker in %q", text)
				}
				if strings.HasPrefix(text, "+") || strings.HasPrefix(text, "-") {
					t.Fatalf("prefix in %q", text)
				}
			}
		}
	})

	t.Run("untracked", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "keep.txt", "x\n")
		if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("fresh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, err := repo.DiffAgainstHead(ctx, "new.txt", nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Rows) != 1 {
			t.Fatalf("rows %d:\n%s", len(d.Rows), formatRows(d.Rows))
		}
		row := d.Rows[0]
		if !isPad(row.Left) {
			t.Fatalf("left %+v", row.Left)
		}
		if row.Right.Kind != '+' || row.Right.Text != "fresh" || row.Right.Line != 1 || len(row.Right.Spans) != 0 {
			t.Fatalf("right %+v", row.Right)
		}
	})

	t.Run("deleted", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "gone.txt", "gone\n")
		if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
			t.Fatal(err)
		}
		d, err := repo.DiffAgainstHead(ctx, "gone.txt", nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Rows) != 1 {
			t.Fatalf("rows %d:\n%s", len(d.Rows), formatRows(d.Rows))
		}
		row := d.Rows[0]
		if row.Left.Kind != '-' || row.Left.Text != "gone" || row.Left.Line != 1 || len(row.Left.Spans) != 0 {
			t.Fatalf("left %+v", row.Left)
		}
		if !isPad(row.Right) {
			t.Fatalf("right %+v", row.Right)
		}
	})

	t.Run("live overrides disk", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "f.txt", "from-disk\n")
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("from-disk\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, err := repo.DiffAgainstHead(ctx, "f.txt", []byte("from-buffer\n"), true)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Rows) != 1 {
			t.Fatalf("rows %d:\n%s", len(d.Rows), formatRows(d.Rows))
		}
		row := d.Rows[0]
		if row.Left.Kind != '~' || row.Right.Kind != '~' {
			t.Fatalf("kinds %q %q", row.Left.Kind, row.Right.Kind)
		}
		if row.Left.Text != "from-disk" || row.Right.Text != "from-buffer" {
			t.Fatalf("text %q %q", row.Left.Text, row.Right.Text)
		}
		if row.Right.Text == "from-disk" {
			t.Fatal("right text came from disk")
		}
	})

	t.Run("identical", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "f.txt", "same\n")
		d, err := repo.DiffAgainstHead(ctx, "f.txt", []byte("same\n"), true)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Rows) != 0 || d.Binary || d.TooBig {
			t.Fatalf("%+v", d)
		}
	})

	t.Run("binary", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "f.txt", "plain\n")
		d, err := repo.DiffAgainstHead(ctx, "f.txt", []byte{'a', 0, 'b'}, true)
		if err != nil {
			t.Fatal(err)
		}
		if !d.Binary || d.TooBig || len(d.Rows) != 0 {
			t.Fatalf("%+v", d)
		}
	})

	t.Run("too big", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "f.txt", "plain\n")
		live := make([]byte, 1<<20+1)
		live[0] = 'x'
		d, err := repo.DiffAgainstHead(ctx, "f.txt", live, true)
		if err != nil {
			t.Fatal(err)
		}
		if !d.TooBig || d.Binary || len(d.Rows) != 0 {
			t.Fatalf("%+v", d)
		}
	})

	t.Run("no trailing newline", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "f.txt", "alpha\n")
		d, err := repo.DiffAgainstHead(ctx, "f.txt", []byte("alpha\nbeta"), true)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Rows) != 2 {
			t.Fatalf("rows %d:\n%s", len(d.Rows), formatRows(d.Rows))
		}
		assertContext(t, d.Rows[0], "alpha", 1, 1)
		row := d.Rows[1]
		if !isPad(row.Left) {
			t.Fatalf("left %+v", row.Left)
		}
		if row.Right.Kind != '+' || row.Right.Text != "beta" || len(row.Right.Spans) != 0 {
			t.Fatalf("right %+v", row.Right)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		dir := t.TempDir()
		repo := seed(t, dir, "f.txt", "same\n")
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := repo.DiffAgainstHead(cctx, "f.txt", nil, false)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err %v", err)
		}
	})
}

func TestDiffUnbornHead(t *testing.T) {
	dir := t.TempDir()
	if _, err := gogit.PlainInit(dir, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, err := repo.DiffAgainstHead(context.Background(), "a.txt", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != 1 {
		t.Fatalf("rows %d:\n%s", len(d.Rows), formatRows(d.Rows))
	}
	row := d.Rows[0]
	if !isPad(row.Left) {
		t.Fatalf("left %+v", row.Left)
	}
	if row.Right.Kind != '+' || row.Right.Text != "hi" || row.Right.Line != 1 || len(row.Right.Spans) != 0 {
		t.Fatalf("right %+v", row.Right)
	}
}

func TestDiffKeepsContextBetweenChanges(t *testing.T) {
	far := []string{
		"l01", "l02", "l03", "l04", "l05", "l06", "l07",
		"oldA",
		"m01", "m02", "m03", "m04", "m05", "m06", "m07",
		"oldB",
		"n01", "n02", "n03",
	}
	farNew := append([]string(nil), far...)
	farNew[7] = "newA"
	farNew[15] = "newB"
	d := diffLive(t, far, farNew)
	if len(d.Rows) != len(far) {
		t.Fatalf("rows %d want %d:\n%s", len(d.Rows), len(far), formatRows(d.Rows))
	}
	a, b := -1, -1
	for i, row := range d.Rows {
		if row.Left.Kind == '~' && row.Right.Kind == '~' && row.Left.Text == "oldA" && row.Right.Text == "newA" {
			a = i
		}
		if row.Left.Kind == '~' && row.Right.Kind == '~' && row.Left.Text == "oldB" && row.Right.Text == "newB" {
			b = i
		}
	}
	if a < 0 || b <= a {
		t.Fatalf("changes a=%d b=%d:\n%s", a, b, formatRows(d.Rows))
	}
	found := false
	for _, row := range d.Rows[a+1 : b] {
		if row.Left.Kind == ' ' && row.Right.Kind == ' ' && row.Left.Text == "m04" && row.Right.Text == "m04" {
			found = true
		}
	}
	if !found {
		t.Fatalf("context line m04 missing:\n%s", formatRows(d.Rows))
	}
}

func diffLive(t *testing.T, oldLines, newLines []string) Diff {
	t.Helper()
	dir := t.TempDir()
	old := strings.Join(oldLines, "\n") + "\n"
	repo := seed(t, dir, "f.txt", old)
	live := strings.Join(newLines, "\n") + "\n"
	d, err := repo.DiffAgainstHead(context.Background(), "f.txt", []byte(live), true)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func assertContext(t *testing.T, row DiffRow, text string, leftLine, rightLine int) {
	t.Helper()
	if row.Left.Kind != ' ' || row.Right.Kind != ' ' {
		t.Fatalf("kinds %q %q", row.Left.Kind, row.Right.Kind)
	}
	if row.Left.Text != text || row.Right.Text != text {
		t.Fatalf("text %q %q", row.Left.Text, row.Right.Text)
	}
	if row.Left.Line != leftLine || row.Right.Line != rightLine {
		t.Fatalf("lines %d %d", row.Left.Line, row.Right.Line)
	}
	if len(row.Left.Spans) != 0 || len(row.Right.Spans) != 0 {
		t.Fatalf("spans %+v %+v", row.Left.Spans, row.Right.Spans)
	}
}

func assertHighlight(t *testing.T, side DiffSide, kind byte) {
	t.Helper()
	ok := false
	for _, sp := range side.Spans {
		if sp.Start < 0 || sp.End <= sp.Start || sp.End > len(side.Text) {
			t.Fatalf("span %+v out of range for %q", sp, side.Text)
		}
		if sp.Kind == kind {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("no %q span in %+v", kind, side.Spans)
	}
}

func isPad(s DiffSide) bool {
	return s.Line == 0 && s.Text == "" && s.Kind == 0 && len(s.Spans) == 0
}

func formatRows(rows []DiffRow) string {
	var b strings.Builder
	for i, row := range rows {
		fmt.Fprintf(&b, "%d L %q %q line=%d spans=%v | R %q %q line=%d spans=%v\n",
			i, row.Left.Kind, row.Left.Text, row.Left.Line, row.Left.Spans,
			row.Right.Kind, row.Right.Text, row.Right.Line, row.Right.Spans)
	}
	return b.String()
}
