package editor

import (
	"strings"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
)

func TestMultiCursorAndLines(t *testing.T) {
	doc := buffer.FromBytes("t.go", []byte("alpha beta alpha\nsecond\n"))
	v := NewView(doc)
	v.Click(0, 0, false)
	v.AddNextOccurrence()
	if got := v.SelectedText(); got != "alpha" {
		t.Fatalf("word = %q", got)
	}
	v.AddNextOccurrence()
	if len(v.Selections()) != 2 {
		t.Fatalf("selections = %d, want 2", len(v.Selections()))
	}
	v.SelectAllOccurrences()
	if len(v.Selections()) != 2 {
		t.Fatalf("all = %d, want 2", len(v.Selections()))
	}
	v.Type("A")
	if !strings.Contains(string(doc.Snapshot().Bytes()), "A beta A") {
		t.Fatalf("body = %q", doc.Snapshot().Bytes())
	}
	if !doc.Undo() {
		t.Fatal("undo")
	}
	if string(doc.Snapshot().Bytes()) != "alpha beta alpha\nsecond\n" {
		t.Fatalf("undo body = %q", doc.Snapshot().Bytes())
	}

	v = NewView(doc)
	v.Click(0, 0, false)
	v.DuplicateLine()
	if string(doc.Snapshot().Bytes()) != "alpha beta alpha\nalpha beta alpha\nsecond\n" {
		t.Fatalf("dup = %q", doc.Snapshot().Bytes())
	}
	doc.Undo()
	v = NewView(doc)
	v.Click(1, 0, false)
	v.ToggleLineComment()
	if !strings.Contains(string(doc.Snapshot().Bytes()), "// second") {
		t.Fatalf("comment = %q", doc.Snapshot().Bytes())
	}
	v.ToggleLineComment()
	if strings.Contains(string(doc.Snapshot().Bytes()), "//") {
		t.Fatalf("uncomment = %q", doc.Snapshot().Bytes())
	}
	v.DeleteLine()
	if string(doc.Snapshot().Bytes()) != "alpha beta alpha\n" {
		t.Fatalf("delete = %q", doc.Snapshot().Bytes())
	}
}

func TestFindReplace(t *testing.T) {
	doc := buffer.FromBytes("t.go", []byte("one two one"))
	v := NewView(doc)
	if err := v.FindNext("one", false, 1); err != nil {
		t.Fatal(err)
	}
	if v.SelectedText() != "one" {
		t.Fatalf("sel = %q", v.SelectedText())
	}
	if err := v.ReplaceCurrent("one", "1", false); err != nil {
		t.Fatal(err)
	}
	if string(doc.Snapshot().Bytes()) != "1 two one" {
		t.Fatalf("replace = %q", doc.Snapshot().Bytes())
	}
	n, err := v.ReplaceAll("one", "1", false)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v body=%q", n, err, doc.Snapshot().Bytes())
	}
	doc = buffer.FromBytes("t.go", []byte("a1 a2"))
	v = NewView(doc)
	n, err = v.ReplaceAll(`a(\d)`, "b$1", true)
	if err != nil || n != 2 || string(doc.Snapshot().Bytes()) != "b1 b2" {
		t.Fatalf("re n=%d err=%v body=%q", n, err, doc.Snapshot().Bytes())
	}
}

func TestFindNextAndPrevious(t *testing.T) {
	doc := buffer.FromBytes("t.go", []byte("one two one"))
	v := NewView(doc)
	if err := v.FindNext("one", false, 1); err != nil {
		t.Fatal(err)
	}
	start, end := v.SelectionBounds()
	if start != 0 || end != 3 {
		t.Fatalf("first %d %d", start, end)
	}
	if err := v.FindNext("one", false, 1); err != nil {
		t.Fatal(err)
	}
	start, end = v.SelectionBounds()
	if start != 8 || end != 11 {
		t.Fatalf("next %d %d", start, end)
	}
	if err := v.FindNext("one", false, -1); err != nil {
		t.Fatal(err)
	}
	start, end = v.SelectionBounds()
	if start != 0 || end != 3 {
		t.Fatalf("prev %d %d", start, end)
	}
}
