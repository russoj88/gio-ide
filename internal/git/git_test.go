package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestStatusAndGutter(t *testing.T) {
	dir := t.TempDir()
	repo := seed(t, dir, "a.go", "package p\nfunc A() {}\nfunc B() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package p\nfunc A() { return }\nfunc B() {}\nfunc C() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gone.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.Stage(ctx, []string{"gone.go"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add gone"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatal(err)
	}

	st, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]FileStatus{}
	for _, f := range st.Files {
		got[f.Path] = f
	}
	if got["a.go"].Work != byte(gogit.Modified) {
		t.Fatalf("modified %+v", got["a.go"])
	}
	if got["new.go"].Work != byte(gogit.Untracked) {
		t.Fatalf("untracked %+v", got["new.go"])
	}
	if err := repo.Stage(ctx, []string{"added.go"}); err != nil {
		t.Fatal(err)
	}
	st, err = repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]FileStatus{}
	for _, f := range st.Files {
		got[f.Path] = f
	}
	if got["added.go"].Staging != byte(gogit.Added) {
		t.Fatalf("added %+v", got["added.go"])
	}
	if got["gone.go"].Work != byte(gogit.Deleted) {
		t.Fatalf("deleted %+v", got["gone.go"])
	}

	idx, err := repo.IndexText(ctx, filepath.Join(dir, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	live := "package p\nfunc A() { return }\nfunc C() {}\n"
	marks := DiffMarks(string(idx), live)
	kinds := map[int]byte{}
	for _, m := range marks {
		kinds[m.Line] = m.Kind
	}
	if kinds[1] != 'm' {
		t.Fatalf("modified line marks %+v", marks)
	}
	if kinds[2] != 'a' && kinds[2] != 'm' {
		t.Fatalf("added line marks %+v", marks)
	}
	del := DiffMarks("a\nb\nc\n", "a\nc\n")
	foundDel := false
	for _, m := range del {
		if m.Kind == 'd' {
			foundDel = true
		}
	}
	if !foundDel {
		t.Fatalf("deleted marks %+v", del)
	}
}

func TestRepoCallsDoNotOverlap(t *testing.T) {
	dir := t.TempDir()
	repo := seed(t, dir, "f.txt", "one\n")
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := repo.Status(ctx); err != nil {
				t.Error(err)
			}
			if _, err := repo.DiffAgainstHead(ctx, "f.txt", []byte("one\n"), true, false); err != nil {
				t.Error(err)
			}
			if _, err := repo.IndexText(ctx, "f.txt"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

func TestStageCommitBranch(t *testing.T) {
	dir := t.TempDir()
	repo := seed(t, dir, "f.txt", "one\n")
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Stage(ctx, []string{"f.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	msg, err := HeadCommit(repo)
	if err != nil || msg != "second" {
		t.Fatalf("head %q %v", msg, err)
	}
	if err := repo.Switch(ctx, "topic", true); err != nil {
		t.Fatal(err)
	}
	br, err := repo.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var topic bool
	for _, b := range br {
		if b.Name == "topic" && b.Current {
			topic = true
		}
	}
	if !topic {
		t.Fatalf("branches %+v", br)
	}
	if err := repo.Switch(ctx, "master", false); err != nil {
		t.Fatal(err)
	}
}

func TestCheckoutWithoutForceRefusesLocalChanges(t *testing.T) {
	dir := t.TempDir()
	repo := seed(t, dir, "f.txt", "one\n")
	ctx := context.Background()
	if err := repo.Switch(ctx, "other", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	err := repo.Switch(ctx, "master", false)
	if !errors.Is(err, ErrDirty) {
		t.Fatalf("switch err %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "f.txt"))
	if string(after) != string(before) {
		t.Fatalf("switch overwrote %q", after)
	}

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
		Force:  false,
	})
	if !errors.Is(err, gogit.ErrUnstagedChanges) {
		t.Fatalf("go-git checkout without Force: %v", err)
	}
	after, _ = os.ReadFile(filepath.Join(dir, "f.txt"))
	if string(after) != "dirty\n" {
		t.Fatalf("checkout overwrote the file: %q", after)
	}
	head, err := repo.repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	// go-git sets HEAD before Reset. The reset then returns ErrUnstagedChanges,
	// so the file is kept and HEAD has already moved.
	if head.Name().Short() == "other" {
		t.Log("checkout without Force left HEAD on other")
	} else {
		t.Logf("checkout without Force moved HEAD to %s and then returned %v", head.Name().Short(), gogit.ErrUnstagedChanges)
	}
}

func TestRelRejectsParent(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := seed(t, dir, "f.txt", "one\n")
	_, err := repo.DiffAgainstHead(context.Background(), "../secret.txt", nil, false, false)
	if err == nil {
		t.Fatal("parent path was accepted")
	}
	inside, err := repo.DiffAgainstHead(context.Background(), "f.txt", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if inside.TooBig || inside.Binary {
		t.Fatalf("inside diff %+v", inside)
	}
}

func TestCommitNotice(t *testing.T) {
	dir := t.TempDir()
	repo := seed(t, dir, "f.txt", "one\n")
	hookDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(hookDir, "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.Notice() == "" {
		t.Fatal("expected a hook notice")
	}
}

func seed(t *testing.T, dir, name, body string) *Repo {
	t.Helper()
	r, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := r.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.User.Name = "Gio-ide Test"
	cfg.User.Email = "gio-ide@example.com"
	if err := r.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("init", &gogit.CommitOptions{}); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}
