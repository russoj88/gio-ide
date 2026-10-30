package buffer

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
	"time"
)

func TestRandomEditsMatchNaive(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 40 {
		naive := []byte{}
		doc := New("")
		for range 30 {
			if len(naive) == 0 || rng.Intn(4) != 0 {
				at := 0
				if len(naive) > 0 {
					at = rng.Intn(len(naive) + 1)
				}
				text := randomText(rng)
				_, err := doc.Apply([]Edit{{Start: Offset(at), End: Offset(at), Text: text}}, OriginCommand)
				if err != nil {
					t.Fatal(err)
				}
				naive = append(naive[:at], append([]byte(text), naive[at:]...)...)
			} else {
				a := rng.Intn(len(naive))
				b := a + 1 + rng.Intn(len(naive)-a)
				_, err := doc.Apply([]Edit{{Start: Offset(a), End: Offset(b)}}, OriginCommand)
				if err != nil {
					t.Fatal(err)
				}
				naive = append(naive[:a], naive[b:]...)
			}
			assertMatch(t, doc.Snapshot(), naive)
		}
	}
}

func TestSimpleLineCol(t *testing.T) {
	doc := New("")
	text := "l\nd\npsz\n\téea"
	if _, err := doc.Apply([]Edit{{Text: text}}, OriginCommand); err != nil {
		t.Fatal(err)
	}
	assertMatch(t, doc.Snapshot(), []byte(text))
	// Fragment the same text.
	doc = New("")
	for _, ch := range []string{"l\nd\npsz\n", "\t", "é", "ea"} {
		at := doc.Snapshot().Len()
		if _, err := doc.Apply([]Edit{{Start: Offset(at), End: Offset(at), Text: ch}}, OriginCommand); err != nil {
			t.Fatal(err)
		}
	}
	assertMatch(t, doc.Snapshot(), []byte(text))
}

func TestUndoRedoAndSnapshotStability(t *testing.T) {
	doc := New("a.go")
	if _, err := doc.Apply([]Edit{{Text: "hello"}}, OriginCommand); err != nil {
		t.Fatal(err)
	}
	first := doc.Snapshot()
	if string(first.Bytes()) != "hello" {
		t.Fatal(string(first.Bytes()))
	}
	if _, err := doc.Apply([]Edit{{Start: 5, End: 5, Text: " world"}}, OriginCommand); err != nil {
		t.Fatal(err)
	}
	if string(first.Bytes()) != "hello" {
		t.Fatalf("snapshot changed: %q", first.Bytes())
	}
	if string(doc.Snapshot().Bytes()) != "hello world" {
		t.Fatal(string(doc.Snapshot().Bytes()))
	}
	if !doc.Undo() || string(doc.Snapshot().Bytes()) != "hello" {
		t.Fatalf("undo: %q", doc.Snapshot().Bytes())
	}
	if string(first.Bytes()) != "hello" {
		t.Fatal("old snapshot after undo")
	}
	if !doc.Redo() || string(doc.Snapshot().Bytes()) != "hello world" {
		t.Fatalf("redo: %q", doc.Snapshot().Bytes())
	}
	// Further edits must not rewrite the first snapshot.
	_, _ = doc.Apply([]Edit{{Start: 0, End: Offset(doc.Snapshot().Len()), Text: "zz"}}, OriginCommand)
	if string(first.Bytes()) != "hello" {
		t.Fatalf("snapshot after later edit: %q", first.Bytes())
	}

	// Typing merges into one undo step.
	d2 := New("")
	now := time.Now()
	_ = now
	for _, ch := range []string{"a", "b", "c"} {
		at := Offset(d2.Snapshot().Len())
		if _, err := d2.Apply([]Edit{{Start: at, End: at, Text: ch}}, OriginTyping); err != nil {
			t.Fatal(err)
		}
	}
	if string(d2.Snapshot().Bytes()) != "abc" {
		t.Fatal(string(d2.Snapshot().Bytes()))
	}
	if !d2.Undo() || string(d2.Snapshot().Bytes()) != "" {
		t.Fatalf("typing undo: %q", d2.Snapshot().Bytes())
	}
	if !d2.Redo() || string(d2.Snapshot().Bytes()) != "abc" {
		t.Fatalf("typing redo: %q", d2.Snapshot().Bytes())
	}
}

func TestCRLFAndBinary(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/a.go"
	if err := writeFile(path, []byte("a\r\nb\r\n")); err != nil {
		t.Fatal(err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(doc.Snapshot().Bytes()) != "a\nb\n" || !doc.CRLF() {
		t.Fatalf("crlf load %q %v", doc.Snapshot().Bytes(), doc.CRLF())
	}
	if _, err := doc.Apply([]Edit{{Start: 1, End: 1, Text: "c"}}, OriginCommand); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ac\r\nb\r\n" {
		t.Fatalf("saved %q", got)
	}

	bin := FromBytes(dir+"/b.bin", []byte{'a', 0, 'b'})
	if !bin.ReadOnly() || bin.Notice() == "" {
		t.Fatal("expected read-only binary")
	}
	if _, err := bin.Apply([]Edit{{Text: "x"}}, OriginCommand); err != ErrReadOnly {
		t.Fatal(err)
	}
	bad := FromBytes(dir+"/c.txt", []byte{0xff, 0xfe})
	if !bad.ReadOnly() {
		t.Fatal("invalid utf-8 should be read-only")
	}
	late := make([]byte, 8193)
	for i := range late {
		late[i] = 'a'
	}
	late[len(late)-1] = 0
	if !FromBytes(dir+"/late.bin", late).ReadOnly() {
		t.Fatal("NUL past the first 8 KiB should be read-only")
	}
}

func TestSaveKeepsModeAndSkipsSymlink(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/secret.go"
	if err := os.WriteFile(path, []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Apply([]Edit{{Start: 8, End: 8, Text: "x"}}, OriginCommand); err != nil {
		t.Fatal(err)
	}
	planted := path + ".gio-ide-tmp"
	outside := dir + "/outside.txt"
	if err := os.WriteFile(outside, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, planted); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package xp\n" {
		t.Fatalf("saved %q", got)
	}
	after, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode %o, want %o", after.Mode().Perm(), before.Mode().Perm())
	}
	kept, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "secret\n" {
		t.Fatalf("temp symlink target changed: %q", kept)
	}
	link, err := os.Lstat(planted)
	if err != nil {
		t.Fatal(err)
	}
	if link.Mode()&os.ModeSymlink == 0 {
		t.Fatal("planted temp name was replaced")
	}

	fresh := dir + "/new.go"
	created := FromBytes(fresh, []byte("package n\n"))
	if err := created.Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("new file mode %o", info.Mode().Perm())
	}

	target := dir + "/target.txt"
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := dir + "/link.go"
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}
	linked := FromBytes(linkPath, []byte("package q\n"))
	if err := linked.Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
		t.Fatalf("symlink was not replaced: %v", st.Mode())
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "keep\n" {
		t.Fatalf("symlink target changed: %q", body)
	}
}

func assertMatch(t *testing.T, s Snapshot, naive []byte) {
	t.Helper()
	if !bytes.Equal(s.Bytes(), naive) {
		t.Fatalf("bytes\n got %q\nwant %q", s.Bytes(), naive)
	}
	if s.Len() != len(naive) {
		t.Fatalf("len %d %d", s.Len(), len(naive))
	}
	wantLines := naiveLineCount(naive)
	if s.LineCount() != wantLines {
		t.Fatalf("lines %d want %d for %q", s.LineCount(), wantLines, naive)
	}
	for i := range wantLines {
		if got, want := int(s.LineStart(i)), naiveLineStart(naive, i); got != want {
			t.Fatalf("lineStart(%d)=%d want %d in %q", i, got, want, naive)
		}
		if !bytes.Equal(s.Line(i), naiveLine(naive, i)) {
			t.Fatalf("line %d\n got %q\nwant %q", i, s.Line(i), naiveLine(naive, i))
		}
	}
	for off := 0; off <= len(naive); off++ {
		gl, gc := s.OffsetToLineCol(Offset(off))
		wl, wc := naiveLineCol(naive, off)
		if gl != wl || gc != wc {
			t.Fatalf("off %d in %q: got %d:%d want %d:%d", off, naive, gl, gc, wl, wc)
		}
	}
	// Spot-check a slice.
	if len(naive) > 0 {
		mid := len(naive) / 2
		if !bytes.Equal(s.Slice(0, Offset(mid)), naive[:mid]) {
			t.Fatal("slice")
		}
	}
}

func naiveLineCount(b []byte) int { return bytes.Count(b, []byte("\n")) + 1 }

func naiveLineStart(b []byte, line int) int {
	if line <= 0 {
		return 0
	}
	seen := 0
	for i, c := range b {
		if c == '\n' {
			seen++
			if seen == line {
				return i + 1
			}
		}
	}
	return len(b)
}

func naiveLine(b []byte, line int) []byte {
	start := naiveLineStart(b, line)
	end := len(b)
	if line+1 < naiveLineCount(b) {
		end = naiveLineStart(b, line+1)
	}
	if end > start && b[end-1] == '\n' {
		end--
	}
	return b[start:end]
}

func naiveLineCol(b []byte, off int) (int, int) {
	line := 0
	col := 0
	for i := 0; i < off && i < len(b); i++ {
		if b[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return line, col
}

func randomText(rng *rand.Rand) string {
	n := 1 + rng.Intn(12)
	var b []byte
	for range n {
		switch rng.Intn(6) {
		case 0:
			b = append(b, '\n')
		case 1:
			b = append(b, '\t')
		case 2:
			b = append(b, "é"...)
		default:
			b = append(b, byte('a'+rng.Intn(26)))
		}
	}
	return string(b)
}

func writeFile(path string, b []byte) error { return os.WriteFile(path, b, 0o644) }
func readFile(path string) ([]byte, error)  { return os.ReadFile(path) }
