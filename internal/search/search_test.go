package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuiltinSearchSkipsParent(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "proj")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var n int
	err := Builtin{}.Search(context.Background(), dir, []string{"../secret.txt"}, Query{Pattern: "secret-token"}, func(b []Result) {
		n += len(b)
	})
	if err != nil || n != 0 {
		t.Fatalf("err=%v hits=%d", err, n)
	}
}

func TestBuiltinSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package p\nfunc hello() {\n\tprintln(\"hello\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := append([]byte("abc"), 0, 'd')
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), bin, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor", "x.go"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []string{"hello.go", "blob.bin", "vendor/x.go"}
	var got []Result
	err := Builtin{}.Search(context.Background(), dir, files, Query{
		Pattern: "hello", Exclude: []string{"vendor/"},
	}, func(b []Result) { got = append(got, b...) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("literal hits = %d (%+v), want 2", len(got), got)
	}
	got = nil
	err = Builtin{}.Search(context.Background(), dir, files, Query{
		Pattern: "h.llo", Regex: true, Exclude: []string{"vendor/"},
	}, func(b []Result) { got = append(got, b...) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("regex hits = %d, want 2", len(got))
	}
}

func TestSearchMatchesRg(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
	dir := t.TempDir()
	body := "alpha beta\nalpha\nGamma\n"
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.txt"}
	q := Query{Pattern: "alpha", CaseSensitive: true}
	var builtin, rip []Result
	if err := (Builtin{}).Search(context.Background(), dir, files, q, func(b []Result) { builtin = append(builtin, b...) }); err != nil {
		t.Fatal(err)
	}
	if err := (RG{}).Search(context.Background(), dir, files, q, func(b []Result) { rip = append(rip, b...) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(builtin, rip) {
		t.Fatalf("builtin %+v\nrg %+v", builtin, rip)
	}
}

func TestCaseFoldKeepsByteColumns(t *testing.T) {
	dir := t.TempDir()
	line := "say İ"
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []Result
	err := Builtin{}.Search(context.Background(), dir, []string{"a.txt"}, Query{Pattern: "İ"}, func(b []Result) {
		got = append(got, b...)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("hits %+v", got)
	}
	hit := got[0]
	if hit.MatchStart < 0 || hit.MatchEnd > len(line) || line[hit.MatchStart:hit.MatchEnd] != "İ" {
		t.Fatalf("span %d:%d in %q", hit.MatchStart, hit.MatchEnd, line)
	}
}

func TestRGEmptyFileList(t *testing.T) {
	var n int
	err := (RG{}).Search(context.Background(), t.TempDir(), nil, Query{Pattern: "a"}, func([]Result) { n++ })
	if err != nil || n != 0 {
		t.Fatalf("err=%v calls=%d", err, n)
	}
}

func TestRGWholeWord(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
	dir := t.TempDir()
	body := "alpha alphabetical\nalpha\n"
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.txt"}
	q := Query{Pattern: "alpha", CaseSensitive: true, WholeWord: true}
	var builtin, rip []Result
	if err := (Builtin{}).Search(context.Background(), dir, files, q, func(b []Result) { builtin = append(builtin, b...) }); err != nil {
		t.Fatal(err)
	}
	if err := (RG{}).Search(context.Background(), dir, files, q, func(b []Result) { rip = append(rip, b...) }); err != nil {
		t.Fatal(err)
	}
	if len(builtin) != 2 || !reflect.DeepEqual(builtin, rip) {
		t.Fatalf("builtin %+v\nrg %+v", builtin, rip)
	}
}
