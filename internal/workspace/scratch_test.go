package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCreate(t *testing.T) {
	dir := t.TempDir()
	steps := []struct {
		ext    string
		body   string
		remove string
		want   string
	}{
		{ext: ".go", body: "package p\n", want: "scratch.go"},
		{ext: ".txt", body: "hello", want: "scratch.txt"},
		{ext: ".go", body: "second", want: "scratch2.go"},
		{remove: "scratch2.go", ext: ".go", body: "reused", want: "scratch2.go"},
	}
	var first string
	for _, step := range steps {
		if step.remove != "" {
			if err := os.Remove(filepath.Join(dir, step.remove)); err != nil {
				t.Fatal(err)
			}
		}
		got, err := Create(dir, step.ext, []byte(step.body))
		if err != nil {
			t.Fatalf("Create %s: %v", step.want, err)
		}
		want := filepath.Join(dir, step.want)
		if got != want {
			t.Fatalf("Create = %s, want %s", got, want)
		}
		if !filepath.IsAbs(got) {
			t.Fatalf("Create path %s is not absolute", got)
		}
		body, err := os.ReadFile(got)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != step.body {
			t.Fatalf("body = %q, want %q", body, step.body)
		}
		st, err := os.Stat(got)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o644 {
			t.Fatalf("%s mode = %o, want 644", step.want, st.Mode().Perm())
		}
		if first == "" {
			first = got
		}
	}
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("dir mode = %o, want 755", st.Mode().Perm())
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	missing, err := List(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing dir = %v, want empty", missing)
	}

	dir := t.TempDir()
	for _, name := range []string{"b.txt", "M.go", "a.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "hidden.go"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(dir, "M.go"),
		filepath.Join(dir, "a.go"),
		filepath.Join(dir, "b.txt"),
	}
	if len(got) != len(want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List = %v, want %v", got, want)
		}
	}
}

func TestContains(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "scratch.go")
	if err := os.WriteFile(file, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(filepath.Dir(dir), "sibling.txt")
	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "dir", path: dir, want: true},
		{name: "file", path: file, want: true},
		{name: "sibling", path: sibling, want: false},
		{name: "prefix", path: dir + "-extra", want: false},
		{name: "dotdot", path: dir + "/../" + filepath.Base(sibling), want: false},
		{name: "join-dotdot", path: filepath.Join(dir, "..", "outside"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Contains(dir, tc.path); got != tc.want {
				t.Fatalf("Contains(%s) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scratch.go")
	body := []byte("package scratch\n")
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "taken.go"), []byte("taken"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "scratch.go")
	if err := os.WriteFile(outside, []byte("out"), 0o644); err != nil {
		t.Fatal(err)
	}

	fails := []struct {
		name string
		old  string
		base string
		keep string
	}{
		{name: "outside", old: outside, base: "note", keep: outside},
		{name: "exists", old: src, base: "taken", keep: src},
		{name: "slash", old: src, base: "a/b", keep: src},
		{name: "dotdot", old: src, base: "..", keep: src},
		{name: "empty", old: src, base: "", keep: src},
		{name: "dot", old: src, base: ".", keep: src},
	}
	for _, tc := range fails {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Rename(dir, tc.old, tc.base); err == nil {
				t.Fatal("expected error")
			}
			if _, err := os.Stat(tc.keep); err != nil {
				t.Fatalf("source missing after failed rename: %v", err)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dir, "note.go")); !os.IsNotExist(err) {
		t.Fatalf("note.go created on failed rename: %v", err)
	}

	got, err := Rename(dir, src, "scratch")
	if err != nil {
		t.Fatal(err)
	}
	if got != src {
		t.Fatalf("Rename same name = %s, want %s", got, src)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal(err)
	}

	got, err = Rename(dir, src, "note")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "note.go")
	if got != want {
		t.Fatalf("Rename = %s, want %s", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("Rename path %s is not absolute", got)
	}
	read, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, body) {
		t.Fatalf("body = %q, want %q", read, body)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("old path still exists: %v", err)
	}

	txt := filepath.Join(dir, "scratch.txt")
	if err := os.WriteFile(txt, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = Rename(dir, txt, "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "note.txt") {
		t.Fatalf("Rename = %s, want note.txt", got)
	}
}

func TestMove(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	body := []byte("data")
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(dir, "dest-dir")
	if err := os.Mkdir(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{existing, destDir} {
		if _, err := Move(src, dest); err == nil {
			t.Fatalf("Move onto %s succeeded", dest)
		}
		read, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(read, body) {
			t.Fatalf("source changed: %q", read)
		}
	}
	if read, err := os.ReadFile(existing); err != nil || string(read) != "old" {
		t.Fatalf("existing dest = %q, %v", read, err)
	}

	dest := filepath.Join(dir, "c.txt")
	got, err := Move(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if got != dest {
		t.Fatalf("Move = %s, want %s", got, dest)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	read, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, body) {
		t.Fatalf("dest = %q, want %q", read, body)
	}
}

func TestMoveEXDEV(t *testing.T) {
	orig := renameFile
	t.Cleanup(func() { renameFile = orig })
	renameFile = func(string, string) error { return syscall.EXDEV }

	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	body := []byte("payload")
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "b.txt")
	got, err := Move(src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if got != dest {
		t.Fatalf("Move = %s, want %s", got, dest)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	read, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, body) {
		t.Fatalf("dest = %q, want %q", read, body)
	}

	keep := filepath.Join(dir, "c.txt")
	kept := []byte("keep")
	if err := os.WriteFile(keep, kept, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "no-such-dir", "d.txt")
	if _, err := Move(keep, missing); err == nil {
		t.Fatal("expected copy failure")
	}
	read, err = os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, kept) {
		t.Fatalf("source = %q, want %q", read, kept)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("partial dest: %v", err)
	}
}
