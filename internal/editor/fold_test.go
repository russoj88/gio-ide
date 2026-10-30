package editor

import (
	"bytes"
	"slices"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
)

func TestScrollRowsUncollapsed(t *testing.T) {
	// 80 newlines is 81 lines. The default page is 40, so line 3 is a legal top.
	doc := buffer.FromBytes("a.go", bytes.Repeat([]byte("a\n"), 80))
	v := NewView(doc)
	n := v.visualCount()
	if n != doc.Snapshot().LineCount() {
		t.Fatalf("visual rows %d, lines %d", n, doc.Snapshot().LineCount())
	}
	for row := range n {
		if got := v.lineAt(row); got != row {
			t.Fatalf("lineAt(%d) = %d", row, got)
		}
		if got := v.visualRow(row); got != row {
			t.Fatalf("visualRow(%d) = %d", row, got)
		}
	}
	v.scrollRows(3)
	if got := v.ScrollLine(); got != 3 {
		t.Fatalf("ScrollLine() = %d, want 3", got)
	}
}

func TestClosedFoldSkipsInterior(t *testing.T) {
	doc := buffer.FromBytes("a.go", bytes.Repeat([]byte("a\n"), 30))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 2}})
	v.Fold()

	// Header stays. The next visual row is the first line after the fold.
	if got := v.lineAt(0); got != 0 {
		t.Fatalf("lineAt(0) = %d, want 0", got)
	}
	if got := v.lineAt(1); got != 3 {
		t.Fatalf("lineAt(1) = %d, want 3", got)
	}
	for row := range v.visualCount() {
		line := v.lineAt(row)
		if line > 0 && line <= 2 {
			t.Fatalf("row %d shows hidden line %d", row, line)
		}
	}

	if line, _ := v.CaretPos(); line != 0 {
		t.Fatalf("caret line %d, want header", line)
	}
	v.MoveDown(false)
	if line, _ := v.CaretPos(); line != 3 {
		t.Fatalf("MoveDown line %d, want 3", line)
	}
	v.MoveUp(false)
	if line, _ := v.CaretPos(); line != 0 {
		t.Fatalf("MoveUp line %d, want 0", line)
	}

	// A short page so the top can move onto the line after the fold.
	v.SetPageLines(4)
	v.SetScrollLine(0)
	v.scrollRows(1)
	if got := v.ScrollLine(); got != 3 {
		t.Fatalf("scrollRows(1) = %d, want 3", got)
	}

	v.SetScrollLine(0)
	v.Click(0, 0, false)
	v.Page(1, false)
	// Four visual steps from the header: 3, 4, 5, 6.
	if line, _ := v.CaretPos(); line != 6 {
		t.Fatalf("Page line %d, want 6", line)
	}
}

func TestRevealAndClickOpenFold(t *testing.T) {
	body := []byte("header\nhidden\nend\nafter\n")
	folds := []Fold{{Start: 0, End: 2}}

	t.Run("click", func(t *testing.T) {
		v := foldedView(body, folds)
		v.Fold()
		v.Click(1, 0, false)
		if v.lineHidden(1) {
			t.Fatal("Click left the line hidden")
		}
		if line, col := v.CaretPos(); line != 1 || col != 0 {
			t.Fatalf("caret line %d col %d, want 1, 0", line, col)
		}
	})

	t.Run("reveal", func(t *testing.T) {
		v := foldedView(body, folds)
		v.Fold()
		off := v.Doc.Snapshot().LineStart(1)
		v.SetSelections([]Sel{{Anchor: off, Head: off}})
		if !v.lineHidden(1) {
			t.Fatal("line 1 was visible before Reveal")
		}
		v.Reveal()
		if v.lineHidden(1) {
			t.Fatal("Reveal left the line hidden")
		}
	})

	t.Run("nested", func(t *testing.T) {
		v := foldedView([]byte("a\nb\nc\nd\ne\nf\ng\n"), []Fold{{Start: 0, End: 5}, {Start: 2, End: 4}})
		v.FoldAll()
		if !v.lineHidden(3) {
			t.Fatal("line 3 was visible")
		}
		v.Click(3, 0, false)
		if v.foldClosed(0) || v.foldClosed(2) {
			t.Fatalf("Click left folds closed: %v", closedKeys(v))
		}
		if line, _ := v.CaretPos(); line != 3 {
			t.Fatalf("caret line %d, want 3", line)
		}

		v.FoldAll()
		off := v.Doc.Snapshot().LineStart(3)
		v.SetSelections([]Sel{{Anchor: off, Head: off}})
		v.Reveal()
		if v.foldClosed(0) || v.foldClosed(2) {
			t.Fatalf("Reveal left folds closed: %v", closedKeys(v))
		}
	})
}

func TestAdjustFolds(t *testing.T) {
	long := bytes.Repeat([]byte("x\n"), 80)
	header := []byte("header\nhidden\nend\nafter\n")
	tests := []struct {
		name        string
		old         []byte
		folds       []Fold
		scroll      int
		ed          buffer.Edit
		shiftScroll bool
		want        []Fold
		wantClosed  []int
		wantScroll  int
	}{
		{
			name:        "insert above shifts folds and scroll",
			old:         long,
			folds:       []Fold{{Start: 20, End: 22}, {Start: 30, End: 32}},
			scroll:      15,
			ed:          buffer.Edit{Text: "\n"},
			shiftScroll: true,
			want:        []Fold{{Start: 21, End: 23}, {Start: 31, End: 33}},
			wantClosed:  []int{21, 31},
			wantScroll:  16,
		},
		{
			name:        "insert above keeps scroll",
			old:         long,
			folds:       []Fold{{Start: 20, End: 22}},
			scroll:      15,
			ed:          buffer.Edit{Text: "\n"},
			shiftScroll: false,
			want:        []Fold{{Start: 21, End: 23}},
			wantClosed:  []int{21},
			wantScroll:  15,
		},
		{
			name:       "header edit stays closed",
			old:        header,
			folds:      []Fold{{Start: 0, End: 2}},
			ed:         buffer.Edit{End: 6, Text: "HEADER"},
			want:       []Fold{{Start: 0, End: 2}},
			wantClosed: []int{0},
		},
		{
			name:       "newline on the header shifts the end",
			old:        header,
			folds:      []Fold{{Start: 0, End: 2}},
			ed:         buffer.Edit{Start: 6, End: 6, Text: "\n"},
			want:       []Fold{{Start: 0, End: 3}},
			wantClosed: []int{0},
		},
		{
			name:  "interior edit opens the fold",
			old:   header,
			folds: []Fold{{Start: 0, End: 2}},
			ed:    buffer.Edit{Start: 7, End: 13, Text: "HIDDEN"},
			want:  []Fold{{Start: 0, End: 2}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := buffer.FromBytes("a.go", tc.old)
			v := NewView(doc)
			v.SetFolds(doc.Version(), tc.folds)
			v.FoldAll()
			v.SetScrollLine(tc.scroll)
			if got := v.ScrollLine(); got != tc.scroll {
				t.Fatalf("setup ScrollLine() = %d, want %d", got, tc.scroll)
			}
			v.AdjustFolds(tc.old, tc.ed, tc.shiftScroll)
			assertFolds(t, v, tc.want)
			assertClosed(t, v, tc.wantClosed...)
			if got := v.ScrollLine(); got != tc.wantScroll {
				t.Fatalf("ScrollLine() = %d, want %d", got, tc.wantScroll)
			}
		})
	}
}

func TestAdjustFoldsTwoSites(t *testing.T) {
	old := []byte("aaa\nbbb\nccc\nddd\neee\n")
	neu := []byte("AAA\nbbb\nccc\nddd\nEEE\n")
	doc := buffer.FromBytes("a.go", old)
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 1, End: 3}})
	v.FoldAll()
	v.AdjustFolds(old, buffer.Edit{End: buffer.Offset(len(old)), Text: string(neu)}, false)
	assertFolds(t, v, []Fold{{Start: 1, End: 3}})
	assertClosed(t, v, 1)
}

func TestHeaderNewlinePullsCaret(t *testing.T) {
	old := []byte("header\nhidden\nend\nafter\n")
	doc := buffer.FromBytes("a.go", old)
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 2}})
	v.Fold()
	ed := buffer.Edit{Start: 6, End: 6, Text: "\n"}
	if _, err := doc.Apply([]buffer.Edit{ed}, buffer.OriginCommand); err != nil {
		t.Fatal(err)
	}
	// The newline is the new hidden line. Offset 7 is its first byte.
	v.SetSelections([]Sel{{Anchor: 7, Head: 7}})
	v.AdjustFolds(old, ed, false)
	if v.lineHidden(0) {
		t.Fatal("header hidden")
	}
	if !v.foldClosed(0) {
		t.Fatal("fold opened")
	}
	line, col := v.CaretPos()
	if line != 0 || col != len("header") {
		t.Fatalf("caret line %d col %d, want header end", line, col)
	}
}

func TestSetFoldsPullsCoveredCaret(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("a\nb\nc\nd\ne\nf\ng\n"))
	v := NewView(doc)
	v.SetFolds(1, []Fold{{Start: 0, End: 5}})
	v.FoldAll()
	off := doc.Snapshot().LineStart(3)
	v.SetSelections([]Sel{{Anchor: off, Head: off}})
	if !v.lineHidden(3) {
		t.Fatal("line 3 was visible")
	}
	v.SetFolds(1, []Fold{{Start: 0, End: 5, Suffix: "}"}})
	line, _ := v.CaretPos()
	if line != 0 {
		t.Fatalf("caret line %d, want header", line)
	}
	if !v.lineHidden(3) {
		t.Fatal("fold opened")
	}
}

func TestSplitLineEditsRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		old, neu string
		wantN    int
	}{
		{name: "two sites", old: "aaa\nbbb\nccc\nddd\neee\n", neu: "AAA\nbbb\nccc\nddd\nEEE\n", wantN: 2},
		{name: "insert line", old: "header\nhidden\nend\n", neu: "header\n\nhidden\nend\n", wantN: 1},
		{name: "same", old: "a\nb\nc\n", neu: "a\nb\nc\n", wantN: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := buffer.Edit{End: buffer.Offset(len(tc.old)), Text: tc.neu}
			got := splitLineEdits([]byte(tc.old), ed)
			if len(got) != tc.wantN {
				t.Fatalf("%d edits, want %d: %+v", len(got), tc.wantN, got)
			}
			buf := append([]byte(nil), tc.old...)
			for _, e := range slices.Backward(got) {
				buf = append(buf[:e.Start], append([]byte(e.Text), buf[e.End:]...)...)
			}
			if string(buf) != tc.neu {
				t.Fatalf("replay %q, want %q", buf, tc.neu)
			}
		})
	}
}

func TestSetFoldsVersion(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("a\nb\nc\nd\ne\nf\ng\n"))
	v := NewView(doc)
	v.SetFolds(3, []Fold{{Start: 0, End: 5}, {Start: 2, End: 4}})
	v.FoldAll()

	v.SetFolds(2, []Fold{{Start: 1, End: 2, Suffix: "stale"}})
	assertFolds(t, v, []Fold{{Start: 0, End: 5}, {Start: 2, End: 4}})
	assertClosed(t, v, 0, 2)
	if v.foldVer != 3 {
		t.Fatalf("foldVer = %d, want 3", v.foldVer)
	}

	// Equal version replaces ranges. A closed header stays closed when some
	// new range still starts on that line.
	v.SetFolds(3, []Fold{{Start: 0, End: 3, Suffix: "tail"}})
	assertFolds(t, v, []Fold{{Start: 0, End: 3, Suffix: "tail"}})
	assertClosed(t, v, 0)
	if got := v.lineAt(1); got != 4 {
		t.Fatalf("lineAt(1) = %d, want 4", got)
	}

	v.SetFolds(3, []Fold{{Start: 1, End: 3}})
	assertFolds(t, v, []Fold{{Start: 1, End: 3}})
	assertClosed(t, v)
	if v.lineHidden(2) {
		t.Fatal("line 2 still hidden")
	}
}

func TestFoldClosesOuter(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("a\nb\nc\nd\ne\nf\ng\n"))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 5}, {Start: 2, End: 4}})
	v.Click(2, 0, false)
	v.Fold()
	if !v.foldClosed(2) {
		t.Fatal("inner fold stayed open")
	}
	if v.foldClosed(0) {
		t.Fatal("outer fold closed on the first Fold")
	}
	if line, _ := v.CaretPos(); line != 2 {
		t.Fatalf("caret line %d, want 2", line)
	}
	v.Fold()
	if !v.foldClosed(0) || !v.foldClosed(2) {
		t.Fatalf("closed %v, want 0 and 2", closedKeys(v))
	}
	if got := v.lineAt(1); got != 6 {
		t.Fatalf("lineAt(1) = %d, want 6", got)
	}
}

func TestUnfoldInnermost(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("a\nb\nc\nd\ne\nf\ng\n"))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 5}, {Start: 2, End: 4}})
	v.FoldAll()
	off := doc.Snapshot().LineStart(3)
	v.SetSelections([]Sel{{Anchor: off, Head: off}})

	v.Unfold()
	if v.foldClosed(2) {
		t.Fatal("inner fold stayed closed")
	}
	if !v.foldClosed(0) {
		t.Fatal("outer fold opened")
	}
	v.Unfold()
	if v.foldClosed(0) {
		t.Fatal("outer fold stayed closed")
	}
	if v.lineHidden(3) {
		t.Fatal("line 3 still hidden")
	}
}

func TestFoldAll(t *testing.T) {
	doc := buffer.FromBytes("a.go", bytes.Repeat([]byte("a\n"), 10))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 2}, {Start: 4, End: 6}})
	v.FoldAll()
	assertClosed(t, v, 0, 4)
	if got := v.lineAt(1); got != 3 {
		t.Fatalf("lineAt(1) = %d, want 3", got)
	}
	if got := v.lineAt(2); got != 4 {
		t.Fatalf("lineAt(2) = %d, want 4", got)
	}
	if got := v.lineAt(3); got != 7 {
		t.Fatalf("lineAt(3) = %d, want 7", got)
	}
}

func TestDeleteLineCollapsedFold(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("header\nhidden1\nhidden2\nafter"))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 2}})
	v.Fold()
	if got := v.lineAt(1); got != 3 {
		t.Fatalf("lineAt(1) = %d, want 3", got)
	}
	v.DeleteLine()
	if got := string(doc.Snapshot().Bytes()); got != "after" {
		t.Fatalf("buffer %q, want %q", got, "after")
	}
}

func foldedView(body []byte, folds []Fold) *View {
	doc := buffer.FromBytes("a.go", body)
	v := NewView(doc)
	v.SetFolds(doc.Version(), folds)
	return v
}

func assertFolds(t *testing.T, v *View, want []Fold) {
	t.Helper()
	if len(v.folds) != len(want) {
		t.Fatalf("folds %+v, want %+v", v.folds, want)
	}
	for i := range want {
		if v.folds[i] != want[i] {
			t.Fatalf("fold %d = %+v, want %+v", i, v.folds[i], want[i])
		}
	}
}

func assertClosed(t *testing.T, v *View, lines ...int) {
	t.Helper()
	if len(v.closed) != len(lines) {
		t.Fatalf("closed %v, want %v", closedKeys(v), lines)
	}
	for _, line := range lines {
		if !v.foldClosed(line) {
			t.Fatalf("line %d not closed, closed %v", line, closedKeys(v))
		}
	}
}

func closedKeys(v *View) []int {
	keys := make([]int, 0, len(v.closed))
	for k := range v.closed {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
