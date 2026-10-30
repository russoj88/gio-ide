package editor

import (
	"image/color"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
)

func TestToggleMark(t *testing.T) {
	note := Mark{Start: 1, End: 4, Kind: MarkNote}
	errMark := Mark{Start: 1, End: 4, Kind: MarkError}
	other := Mark{Start: 6, End: 8, Kind: MarkSmell}
	tests := []struct {
		name string
		in   []Mark
		add  Mark
		want []Mark
	}{
		{name: "add", add: note, want: []Mark{note}},
		{name: "toggle off", in: []Mark{note, other}, add: note, want: []Mark{other}},
		{name: "replace kind", in: []Mark{note}, add: errMark, want: []Mark{errMark}},
		{name: "ignore reversed", in: []Mark{other}, add: Mark{Start: 5, End: 4}, want: []Mark{other}},
		{name: "keep point", add: Mark{Kind: MarkNote}, want: []Mark{{Kind: MarkNote}}},
		{name: "keep other range", in: []Mark{other}, add: note, want: []Mark{other, note}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ToggleMark(tc.in, tc.add)
			if !sameMarks(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			if len(tc.in) > 0 && &got[0] == &tc.in[0] {
				t.Fatal("ToggleMark mutated the input slice")
			}
		})
	}
}

func TestLinesMark(t *testing.T) {
	snap := buffer.FromBytes("a.go", []byte("aaa\nbbb\nccc")).Snapshot()
	tests := []struct {
		name        string
		first, last int
		start, end  buffer.Offset
	}{
		{name: "middle", first: 1, last: 1, start: 4, end: 8},
		{name: "last", first: 2, last: 2, start: 8, end: 11},
		{name: "swapped", first: 1, last: 0, start: 0, end: 8},
		{name: "clamp", first: -3, last: 9, start: 0, end: 11},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := LinesMark(snap, tc.first, tc.last)
			if start != tc.start || end != tc.end {
				t.Fatalf("got %d,%d want %d,%d", start, end, tc.start, tc.end)
			}
		})
	}
	if start, end := LinesMark(nil, 0, 1); start != 0 || end != 0 {
		t.Fatalf("nil snapshot %d,%d", start, end)
	}
}

func TestSelectionMark(t *testing.T) {
	snap := buffer.FromBytes("a.go", []byte("aaa\nbbb\nccc\n")).Snapshot()
	tests := []struct {
		name      string
		start     buffer.Offset
		end       buffer.Offset
		fullLines bool
		want      Mark
		ok        bool
	}{
		{name: "caret line", start: 5, end: 5, want: Mark{Start: 4, End: 8, Kind: MarkError}, ok: true},
		{name: "partial", start: 5, end: 7, want: Mark{Start: 5, End: 7, Kind: MarkError}, ok: true},
		{name: "full lines", start: 5, end: 7, fullLines: true, want: Mark{Start: 4, End: 8, Kind: MarkError}, ok: true},
		{name: "ends on next line", start: 0, end: 4, fullLines: true, want: Mark{Start: 0, End: 4, Kind: MarkError}, ok: true},
		{name: "reversed", start: 7, end: 5, want: Mark{Start: 5, End: 7, Kind: MarkError}, ok: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := SelectionMark(snap, tc.start, tc.end, MarkError, tc.fullLines)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("got %+v %v, want %+v %v", got, ok, tc.want, tc.ok)
			}
		})
	}
	empty := buffer.FromBytes("a.go", nil).Snapshot()
	got, ok := SelectionMark(empty, 0, 0, MarkNote, false)
	if !ok || got.Start != 0 || got.End != 0 {
		t.Fatalf("empty buffer %+v %v", got, ok)
	}
	if _, ok := SelectionMark(nil, 0, 1, MarkNote, false); ok {
		t.Fatal("nil snapshot")
	}
}

func TestShiftMarks(t *testing.T) {
	old := []byte("abcdef")
	mark := []Mark{{Start: 1, End: 4, Kind: MarkSmell}}
	tests := []struct {
		name string
		ed   buffer.Edit
		want []Mark
	}{
		{name: "before", ed: buffer.Edit{Text: "X"}, want: []Mark{{Start: 2, End: 5, Kind: MarkSmell}}},
		{name: "after", ed: buffer.Edit{Start: 6, Text: "Z"}, want: mark},
		{name: "inside", ed: buffer.Edit{Start: 2, End: 2, Text: "YY"}, want: []Mark{{Start: 1, End: 6, Kind: MarkSmell}}},
		{name: "cover", ed: buffer.Edit{Start: 1, End: 4, Text: "Z"}, want: nil},
		{name: "overlap start", ed: buffer.Edit{End: 2, Text: "Q"}, want: []Mark{{Start: 0, End: 3, Kind: MarkSmell}}},
		{name: "overlap end", ed: buffer.Edit{Start: 3, End: 5, Text: "ZZZ"}, want: []Mark{{Start: 1, End: 6, Kind: MarkSmell}}},
		{name: "delete all", ed: buffer.Edit{End: 6}, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShiftMarks(old, tc.ed, mark)
			if !sameMarks(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestShiftMarksTwoSites(t *testing.T) {
	old := []byte("aaa\nbbb\nccc\n")
	mark := []Mark{{Start: 4, End: 8, Kind: MarkNote}}
	combined := buffer.Edit{End: buffer.Offset(len(old) - 1), Text: "XXX\nbbb\nZZZ"}
	got := ShiftMarks(old, combined, mark)
	if !sameMarks(got, mark) {
		t.Fatalf("combined got %#v", got)
	}
	neu := []byte("XXX\nbbb\nZZZ\n")
	if string(neu[got[0].Start:got[0].End]) != "bbb\n" {
		t.Fatalf("mark text %q", neu[got[0].Start:got[0].End])
	}

	// noteFormat applies the last hunk first, against the original bytes.
	got = ShiftMarks(old, buffer.Edit{Start: 8, End: 12, Text: "ZZZ\n"}, mark)
	got = ShiftMarks(old, buffer.Edit{End: 4, Text: "XXX\n"}, got)
	got = ClipMarks(got, len(neu))
	if !sameMarks(got, mark) {
		t.Fatalf("hunks got %#v", got)
	}
}

func TestClipMarks(t *testing.T) {
	in := []Mark{{Start: 0, End: 2}, {Start: 3, End: 9}, {End: 0}, {Start: 4, End: 3}}
	got := ClipMarks(in, 4)
	want := []Mark{{Start: 0, End: 2}, {End: 0}}
	if !sameMarks(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestFoldTailMark(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("{\ninside\n}\n"))
	v := NewView(doc)
	v.SetFolds(doc.Version(), []Fold{{Start: 0, End: 2, Suffix: "}"}})
	v.closed = map[int]struct{}{0: {}}
	v.SetMarks([]Mark{{Start: 2, End: 9, Kind: MarkError}})
	st := Style{Colors: Colors{MarkError: color.NRGBA{R: 9, A: 80}}}
	col, ok := v.foldTailMark(st, doc.Snapshot(), 0)
	if !ok || col.R != 9 || col.A != 80 {
		t.Fatalf("tail %+v %v", col, ok)
	}
	v.SetMarks(nil)
	if _, ok := v.foldTailMark(st, doc.Snapshot(), 0); ok {
		t.Fatal("empty marks still tint")
	}
}

func sameMarks(got, want []Mark) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
