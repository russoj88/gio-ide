package editor

import (
	"bytes"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
)

func TestTypeMoveUndo(t *testing.T) {
	doc := buffer.New("a.go")
	v := NewView(doc)
	v.Type("hé")
	v.Type("llo\n")
	v.Type("x")
	if got := string(doc.Snapshot().Bytes()); got != "héllo\nx" {
		t.Fatal(got)
	}
	v.MoveUp(false)
	line, col := v.CaretPos()
	if line != 0 {
		t.Fatalf("line %d col %d", line, col)
	}
	v.MoveLineEnd(false)
	if _, col := v.CaretPos(); col != len("héllo") {
		t.Fatalf("col %d", col)
	}
	if !v.Undo() {
		t.Fatal("undo")
	}
	// Typing was one group.
	if got := string(doc.Snapshot().Bytes()); got != "" {
		t.Fatalf("after undo %q", got)
	}
	if !v.Redo() || string(doc.Snapshot().Bytes()) != "héllo\nx" {
		t.Fatalf("redo %q", doc.Snapshot().Bytes())
	}
}

func TestScrollStopsAtLastLine(t *testing.T) {
	short := buffer.FromBytes("a.txt", bytes.Repeat([]byte("a\n"), 9))
	v := NewView(short)
	v.SetScrollLine(40)
	if got := v.ScrollLine(); got != 0 {
		t.Fatalf("short file scrolled to %d", got)
	}

	// 99 newlines is 100 lines. The default page is 40, so the last legal
	// first line is 60. A 10-line page stops at 90, including Ctrl+End.
	long := buffer.FromBytes("a.txt", bytes.Repeat([]byte("a\n"), 99))
	v = NewView(long)
	v.SetScrollLine(1000)
	if got := v.ScrollLine(); got != 60 {
		t.Fatalf("default page scroll %d, want 60", got)
	}
	v.pageLines = 10
	v.SetScrollLine(1000)
	if got := v.ScrollLine(); got != 90 {
		t.Fatalf("page scroll %d, want 90", got)
	}
	v.SetScrollLine(0)
	v.MoveDocEnd(false)
	if got := v.ScrollLine(); got != 90 {
		t.Fatalf("doc end scroll %d, want 90", got)
	}
}

func TestUndoStaysOnTheEdit(t *testing.T) {
	var b []byte
	for range 80 {
		b = append(b, []byte("line\n")...)
	}
	doc := buffer.FromBytes("a.go", b)
	v := NewView(doc)
	v.SetPageLines(10)
	v.Click(10, 0, false)
	v.Type("X")
	if v.ScrollLine() > 20 {
		t.Fatalf("scroll before undo %d", v.ScrollLine())
	}
	if !v.Undo() {
		t.Fatal("undo")
	}
	line, col := v.CaretPos()
	if line != 10 || col != 0 {
		t.Fatalf("caret after undo line %d col %d", line, col)
	}
	if v.ScrollLine() > 20 {
		t.Fatalf("undo scrolled to %d", v.ScrollLine())
	}
	if !v.Redo() {
		t.Fatal("redo")
	}
	line, col = v.CaretPos()
	if line != 10 || col != 1 {
		t.Fatalf("caret after redo line %d col %d", line, col)
	}
	if v.ScrollLine() > 20 {
		t.Fatalf("redo scrolled to %d", v.ScrollLine())
	}
}

func TestUndoDeleteRestoresCaret(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("abcdef\n"))
	v := NewView(doc)
	v.Click(0, 2, false)
	v.MoveRight(true)
	v.MoveRight(true)
	if v.SelectedText() != "cd" {
		t.Fatalf("sel %q", v.SelectedText())
	}
	v.DeleteForward()
	if !v.Undo() {
		t.Fatal("undo")
	}
	line, col := v.CaretPos()
	if line != 0 || col != 4 {
		t.Fatalf("caret line %d col %d", line, col)
	}
	if got := string(doc.Snapshot().Bytes()); got != "abcdef\n" {
		t.Fatalf("body %q", got)
	}
}

func TestUndoOnCleanDoesNotMove(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("abc\ndef\n"))
	v := NewView(doc)
	v.Click(0, 1, false)
	v.SetScrollLine(0)
	if v.Undo() {
		t.Fatal("undo of an unmodified document")
	}
	line, col := v.CaretPos()
	if line != 0 || col != 1 || v.ScrollLine() != 0 {
		t.Fatalf("line %d col %d scroll %d", line, col, v.ScrollLine())
	}
}

func TestMoveDownOntoShorterLine(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("hello\nhi"))
	v := NewView(doc)
	v.MoveLineEnd(false)
	v.MoveDown(false)
	line, col := v.CaretPos()
	if line != 1 || col != 2 {
		t.Fatalf("caret line %d col %d, want 1, 2", line, col)
	}
}

func TestByteToCellSkipsC0(t *testing.T) {
	line := []byte{0x01, 'a'}
	if got := byteToCell(line, 1, 4); got != 0 {
		t.Fatalf("C0 advanced a cell to %d", got)
	}
	if got := byteToCell(line, 2, 4); got != 1 {
		t.Fatalf("letter cell %d, want 1", got)
	}
	if got := byteToCell([]byte{'\t', 'a'}, 1, 4); got != 4 {
		t.Fatalf("tab cell %d, want 4", got)
	}
	if got := byteToCell([]byte{0x7f, 'b'}, 1, 4); got != 0 {
		t.Fatalf("DEL advanced a cell to %d", got)
	}
}

func TestSelectionReplaceAndClick(t *testing.T) {
	doc := buffer.FromBytes("a.go", []byte("abcdef\nsecond"))
	v := NewView(doc)
	v.Click(0, 1, false)
	v.MoveRight(true)
	v.MoveRight(true)
	if v.SelectedText() != "bc" {
		t.Fatalf("sel %q", v.SelectedText())
	}
	v.Type("XYZ")
	if got := string(doc.Snapshot().Bytes()); got != "aXYZdef\nsecond" {
		t.Fatal(got)
	}
	v.Click(1, 0, false)
	v.MoveDocEnd(true)
	if v.SelectedText() != "second" {
		t.Fatalf("sel %q", v.SelectedText())
	}
	cut := v.Cut()
	if cut != "second" {
		t.Fatal(cut)
	}
	v.Paste("Z")
	if got := string(doc.Snapshot().Bytes()); got != "aXYZdef\nZ" {
		t.Fatal(got)
	}
	v.SelectAll()
	if v.SelectedText() != "aXYZdef\nZ" {
		t.Fatal(v.SelectedText())
	}
}
