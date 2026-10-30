package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestFuzzyRanking(t *testing.T) {
	files := []string{
		"internal/http/server.go",
		"internal/http/client.go",
		"cmd/hsrv/main.go",
		"server/other.go",
		"readme.md",
	}
	ix := &idx{root: ".", files: files}
	hits := ix.Match(context.Background(), "hsrv", 10)
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].Path != "internal/http/server.go" {
		t.Fatalf("first = %s, want internal/http/server.go (scores: %+v)", hits[0].Path, hits)
	}
}

func TestFuzzyScoreLongerLower(t *testing.T) {
	// İ lowercases to more UTF-8 bytes than it occupies.
	score, ok := FuzzyScore("ag", "aİ.go")
	if !ok || score <= 0 {
		t.Fatalf("score=%d ok=%v", score, ok)
	}
}

func TestFileListFixture(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a\n")
	write("internal/http/server.go", "package http\n")
	write(".gitignore", "*.o\nvendor/\n")
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.go", "internal/http/server.go", ".gitignore"} {
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: &object.Signature{
		Name: "gio-ide", Email: "gio-ide@example.com", When: time.Now(),
	}}); err != nil {
		t.Fatal(err)
	}
	write("notes.txt", "untracked\n")
	write("foo.o", "obj\n")
	write("vendor/x.go", "package vendor\n")
	ix, err := Open(dir, []string{"skipme/"})
	if err != nil {
		t.Fatal(err)
	}
	write("skipme/nope.go", "package skip\n")
	if err := ix.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := ix.Files()
	want := []string{".gitignore", "a.go", "internal/http/server.go", "notes.txt"}
	if len(got) != len(want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("files = %v, want %v", got, want)
		}
	}
}

func TestIndexSkipsTrackedEscape(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("a.go"); err != nil {
		t.Fatal(err)
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	idx.Entries = append(idx.Entries, &index.Entry{
		Name: "../secret.txt",
		Mode: filemode.Regular,
		Hash: plumbing.ZeroHash,
	})
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatal(err)
	}
	ix, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ix.Files() {
		if strings.Contains(f, "..") || strings.Contains(f, "secret") {
			t.Fatalf("files = %v", ix.Files())
		}
	}
	if got := ix.Files(); len(got) != 1 || got[0] != "a.go" {
		t.Fatalf("files = %v", got)
	}
}

func TestRecentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "recent.toml")
	if _, err := PushRecent(file, "/a.go"); err != nil {
		t.Fatal(err)
	}
	r, err := PushRecent(file, "/b.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Paths) != 2 || r.Paths[0] != "/b.go" {
		t.Fatalf("recent = %+v", r)
	}
	loaded, err := LoadRecent(file)
	if err != nil || loaded.Paths[0] != "/b.go" {
		t.Fatalf("loaded %+v err %v", loaded, err)
	}
}

func TestIndexThousandFiles(t *testing.T) {
	dir := t.TempDir()
	const n = 1000
	for i := range n {
		rel := filepath.Join("pkg", "f"+itoa(i)+".go")
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package pkg\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Files()) != n {
		t.Fatalf("files = %d, want %d", len(ix.Files()), n)
	}
	hits := ix.Match(context.Background(), "f42", 5)
	if len(hits) == 0 {
		t.Fatal("fuzzy match returned nothing")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
