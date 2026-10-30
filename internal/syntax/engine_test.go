package syntax

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
)

func TestGrammarABI15(t *testing.T) {
	if got := ABI(); got != 15 {
		t.Fatalf("ABI = %d, want 15", got)
	}
}

func TestFixtureNoErrorNode(t *testing.T) {
	text := readFixture(t)
	_, hasErr, cleanup := ParseSexp(text)
	cleanup()
	if hasErr {
		t.Fatal("fixture parsed with an ERROR node")
	}
	if Live() != 0 {
		t.Fatalf("live objects = %d after parse cleanup", Live())
	}
}

func TestIncrementalParseEqualsFull(t *testing.T) {
	text := append([]byte(nil), readFixture(t)...)
	rng := rand.New(rand.NewSource(1))
	for step := range 25 {
		ed := randomEdit(rng, text)
		after, ok := applyEdit(text, ed)
		if !ok {
			t.Fatalf("step %d: edit does not apply", step)
		}
		inc, done := IncrementalSexp(text, ed, after)
		full, _, cleanup := ParseSexp(after)
		if inc != full {
			done()
			cleanup()
			t.Fatalf("step %d: incremental sexp differs from a full reparse", step)
		}
		done()
		cleanup()
		text = after
	}
	if Live() != 0 {
		t.Fatalf("live objects = %d", Live())
	}
}

func TestHighlightGolden(t *testing.T) {
	text := readFixture(t)
	var got bus.SyntaxSpans
	eng := New(func(ev bus.Event) {
		if sp, ok := ev.(bus.SyntaxSpans); ok {
			got = sp
		}
	})
	doc := buffer.FromBytes("fixture.go", text)
	eng.Open(doc.ID(), "go", doc.Snapshot())
	eng.RequestSpans(doc.ID(), doc.Version(), 0, doc.Snapshot().LineCount())
	eng.Flush()
	eng.Close(doc.ID())
	eng.Shutdown()
	if Live() != 0 {
		t.Fatalf("live objects = %d after close", Live())
	}
	formatted := formatSpans(got)
	path := filepath.Join("testdata", "fixture.golden")
	if os.Getenv("GIO_IDE_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v\n--- got ---\n%s", err, formatted)
	}
	if string(want) != formatted {
		t.Fatalf("highlight mismatch\n--- got ---\n%s", formatted)
	}
}

func TestLiveObjectsZero(t *testing.T) {
	text := readFixture(t)
	eng := New(nil)
	doc := buffer.FromBytes("fixture.go", text)
	eng.Open(doc.ID(), "go", doc.Snapshot())
	eng.RequestSpans(doc.ID(), 1, 0, 20)
	eng.RequestBracket(doc.ID(), 1, 0)
	eng.Flush()
	eng.Close(doc.ID())
	eng.Flush()
	if Live() == 0 {
		t.Fatal("parser should stay alive until shutdown")
	}
	eng.Shutdown()
	if Live() != 0 {
		t.Fatalf("live objects = %d after shutdown", Live())
	}
}

func formatSpans(sp bus.SyntaxSpans) string {
	var b strings.Builder
	for i, line := range sp.Spans {
		for _, s := range line {
			b.WriteString(strconv.Itoa(sp.FirstLine + i))
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(s.Start))
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(s.End))
			b.WriteByte(':')
			b.WriteString(s.Style)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "fixture.go"))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func randomEdit(rng *rand.Rand, text []byte) buffer.Edit {
	if len(text) == 0 {
		return buffer.Edit{Text: "a"}
	}
	start := rng.Intn(len(text) + 1)
	end := start
	if rng.Intn(2) == 0 && start < len(text) {
		end = min(start+1+rng.Intn(len(text)-start), len(text))
	}
	chunk := []byte("x\n")
	if rng.Intn(2) == 0 {
		chunk = []byte("fn")
	}
	if rng.Intn(4) == 0 {
		chunk = nil
	}
	return buffer.Edit{Start: buffer.Offset(start), End: buffer.Offset(end), Text: string(chunk)}
}
