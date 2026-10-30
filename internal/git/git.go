// Package git is the go-git service for status, staging, commit, branches, and diffs.
// It does not call the git binary.
package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ErrDirty is returned when a branch switch would touch local changes.
var ErrDirty = errors.New("worktree has local changes")

// FileStatus is one path in the cached status.
type FileStatus struct {
	Path          string
	Staging, Work byte
}

// Status is a branch name plus changed files.
type Status struct {
	Branch string
	Files  []FileStatus
}

// Branch is one local branch.
type Branch struct {
	Name    string
	Current bool
}

// Repo is a worktree opened with DetectDotGit.
type Repo struct {
	mu     sync.Mutex
	repo   *gogit.Repository
	root   string
	status Status
	notice string
	branch []Branch
}

// Open opens the repository that owns root. root may be the worktree or a subdirectory.
func Open(root string) (*Repo, error) {
	r, err := gogit.PlainOpenWithOptions(root, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	return &Repo{repo: r, root: wt.Filesystem.Root()}, nil
}

// Root is the worktree root.
func (r *Repo) Root() string { return r.root }

// Notice is the commit-dialog warning cached by the last Status call.
func (r *Repo) Notice() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.notice
}

// Status reads the worktree and caches the result.
func (r *Repo) Status(ctx context.Context) (Status, error) {
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return Status{}, err
	}
	st, err := wt.StatusWithOptions(gogit.StatusOptions{})
	if err != nil {
		return Status{}, err
	}
	head := ""
	if ref, err := r.repo.Head(); err == nil {
		head = ref.Name().Short()
	}
	var files []FileStatus
	for path, fs := range st {
		if fs.Staging == gogit.Unmodified && fs.Worktree == gogit.Unmodified {
			continue
		}
		files = append(files, FileStatus{Path: path, Staging: byte(fs.Staging), Work: byte(fs.Worktree)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	out := Status{Branch: head, Files: files}
	notice := commitNotice(r.repo, r.root)
	branches, _ := r.readBranches(head)
	r.mu.Lock()
	r.status = out
	r.notice = notice
	r.branch = branches
	r.mu.Unlock()
	return out, nil
}

// CachedBranches returns the branch list from the last Status call.
func (r *Repo) CachedBranches() []Branch {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]Branch(nil), r.branch...)
	return out
}

// IndexText returns the index blob for a worktree-relative or absolute path.
// A missing index entry returns an empty slice and a nil error.
func (r *Repo) IndexText(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rel, err := r.rel(path)
	if err != nil {
		return nil, err
	}
	idx, err := r.repo.Storer.Index()
	if err != nil {
		return nil, err
	}
	e, err := idx.Entry(rel)
	if err != nil {
		return []byte{}, nil
	}
	blob, err := r.repo.BlobObject(e.Hash)
	if err != nil {
		return nil, err
	}
	rc, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Stage adds each path to the index.
func (r *Repo) Stage(ctx context.Context, paths []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return err
	}
	for _, p := range paths {
		rel, err := r.rel(p)
		if err != nil {
			return err
		}
		if _, err := wt.Add(rel); err != nil {
			return fmt.Errorf("stage %s: %w", rel, err)
		}
	}
	return nil
}

// Commit creates a commit. go-git reads the author from the Git config.
// Hooks are not run and commits are not signed.
func (r *Repo) Commit(ctx context.Context, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return errors.New("commit message is empty")
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return err
	}
	_, err = wt.Commit(msg, &gogit.CommitOptions{})
	return err
}

// Branches lists local branches.
func (r *Repo) Branches(ctx context.Context) ([]Branch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	head := ""
	if ref, err := r.repo.Head(); err == nil {
		head = ref.Name().Short()
	}
	list, err := r.readBranches(head)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.branch = list
	r.mu.Unlock()
	return list, nil
}

// Switch checks out name. create makes the branch at HEAD.
// Force is never set. A dirty worktree is refused before checkout.
func (r *Repo) Switch(ctx context.Context, name string, create bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("branch name is empty")
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return err
	}
	st, err := wt.Status()
	if err != nil {
		return err
	}
	if !st.IsClean() {
		return ErrDirty
	}
	opts := &gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
		Create: create,
	}
	return wt.Checkout(opts)
}

func (r *Repo) readBranches(head string) ([]Branch, error) {
	iter, err := r.repo.Branches()
	if err != nil {
		return nil, err
	}
	var list []Branch
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		n := ref.Name().Short()
		list = append(list, Branch{Name: n, Current: n == head})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func (r *Repo) rel(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(r.root, path)
	}
	rel, err := filepath.Rel(filepath.Clean(r.root), filepath.Clean(abs))
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the repository", path)
	}
	return filepath.ToSlash(rel), nil
}

func commitNotice(repo *gogit.Repository, root string) string {
	cfg, err := repo.Config()
	hooksPath := ""
	gpg := false
	if err == nil && cfg != nil && cfg.Raw != nil {
		hooksPath = cfg.Raw.Section("core").Option("hooksPath")
		gpg = strings.EqualFold(cfg.Raw.Section("commit").Option("gpgsign"), "true")
	}
	hookDir := filepath.Join(root, ".git", "hooks")
	if hooksPath != "" {
		if filepath.IsAbs(hooksPath) {
			hookDir = hooksPath
		} else {
			hookDir = filepath.Join(root, hooksPath)
		}
	}
	hooks := executableHooks(hookDir)
	if !hooks && hooksPath == "" && !gpg {
		return ""
	}
	return "gio-ide does not run Git hooks or sign commits. Use the terminal for git commands that need them."
}

func executableHooks(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".sample") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// HeadCommit returns the current commit message.
func HeadCommit(r *Repo) (string, error) {
	ref, err := r.repo.Head()
	if err != nil {
		return "", err
	}
	c, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(c.Message), nil
}
