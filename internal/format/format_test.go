package format

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
)

func TestGofumpt(t *testing.T) {
	src := []byte("package p\n\nfunc F( x int ) int {\nreturn x\n}\n")
	got, err := Gofumpt(src, ModuleInfo{LangVersion: "go1.27"})
	if err != nil {
		t.Fatal(err)
	}
	want := "package p\n\nfunc F(x int) int {\n\treturn x\n}\n"
	if string(got) != want {
		t.Fatalf("got %q", got)
	}
}

func TestLookupModule(t *testing.T) {
	dir := t.TempDir()
	mod := "module example.com/x\n\ngo 1.27.2\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "p.go")
	info := LookupModule(file)
	if info.ModulePath != "example.com/x" {
		t.Fatalf("module %q", info.ModulePath)
	}
	if !strings.HasPrefix(info.LangVersion, "go1.27") {
		t.Fatalf("lang %q", info.LangVersion)
	}

	none := LookupModule("")
	if none.ModulePath != "" || none.LangVersion == "" {
		t.Fatalf("fallback %+v", none)
	}
}

func TestMapOffset(t *testing.T) {
	// "cde" at [2,5) becomes "XY".
	one := []Hunk{{Start: 2, End: 5, Text: "XY"}}
	two := []Hunk{
		{Start: 1, End: 2, Text: "QQ"},
		{Start: 4, End: 4, Text: "Z"},
	}
	tests := []struct {
		name  string
		off   int
		hunks []Hunk
		want  int
	}{
		{"before", 1, one, 1},
		{"inside", 3, one, 4},
		{"at end", 5, one, 4},
		{"after", 6, one, 5},
		{"before first of two", 0, two, 0},
		{"inside first", 1, two, 3},
		{"between", 3, two, 4},
		{"on insert", 4, two, 6},
		{"after both", 5, two, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapOffset(tt.off, tt.hunks); got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}

func TestHunksRoundTrip(t *testing.T) {
	old := []byte("abcdef")
	neu := []byte("abXYf")
	doc := buffer.FromBytes("x.go", old)
	var eds []buffer.Edit
	for _, h := range Hunks(old, neu) {
		eds = append(eds, buffer.Edit{
			Start: buffer.Offset(h.Start),
			End:   buffer.Offset(h.End),
			Text:  h.Text,
		})
	}
	if _, err := doc.Apply(eds, buffer.OriginCommand); err != nil {
		t.Fatal(err)
	}
	if got := string(doc.Snapshot().Bytes()); got != string(neu) {
		t.Fatalf("got %q", got)
	}
	if Hunks(old, old) != nil {
		t.Fatal("equal buffers produced hunks")
	}
}

func TestGoimportsAddsFmt(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatal("go is not on PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.27.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "p.go")
	src := []byte("package p\n\nfunc F() {\n\tfmt.Println(\"hi\")\n}\n")
	if err := os.WriteFile(file, src, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Goimports(context.Background(), file, src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"fmt"`) {
		t.Fatalf("missing fmt import:\n%s", got)
	}
}
