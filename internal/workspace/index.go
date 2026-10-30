// Package workspace indexes the project tree, scratch files, and recent paths.
package workspace

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Node is one file or directory in the project tree.
type Node struct {
	Name string
	Path string
	Dir  bool
	Kids []*Node
}

// Index lists the files of a project and answers fuzzy queries.
type Index interface {
	Root() string
	Files() []string
	Match(ctx context.Context, query string, limit int) []Match
	Tree() *Node
	Refresh(ctx context.Context) error
}

// Open builds an index for root. root may be outside a Git repository.
func Open(root string, exclude []string) (Index, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	ix := &idx{root: abs, exclude: append([]string(nil), exclude...)}
	if err := ix.Refresh(context.Background()); err != nil {
		return nil, err
	}
	return ix, nil
}

type idx struct {
	mu      sync.Mutex
	root    string
	exclude []string
	files   []string
	tree    *Node
}

func (i *idx) Root() string { return i.root }

func (i *idx) Files() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]string, len(i.files))
	copy(out, i.files)
	return out
}

func (i *idx) Tree() *Node {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.tree
}

func (i *idx) Match(ctx context.Context, query string, limit int) []Match {
	files := i.Files()
	if limit <= 0 {
		limit = 50
	}
	q := strings.TrimSpace(query)
	var hits []Match
	for n, f := range files {
		if n&255 == 0 {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
		}
		if q == "" {
			hits = append(hits, Match{Path: f})
			if len(hits) >= limit {
				break
			}
			continue
		}
		sc, ok := FuzzyScore(q, f)
		if !ok {
			continue
		}
		hits = append(hits, Match{Path: f, Score: sc})
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].Score != hits[b].Score {
			return hits[a].Score > hits[b].Score
		}
		return hits[a].Path < hits[b].Path
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func (i *idx) Refresh(ctx context.Context) error {
	files, err := listFiles(ctx, i.root, i.exclude)
	if err != nil {
		return err
	}
	i.mu.Lock()
	i.files = files
	i.tree = buildTree(files)
	i.mu.Unlock()
	return nil
}

func listFiles(ctx context.Context, root string, exclude []string) ([]string, error) {
	tracked := map[string]bool{}
	var patterns []gitignore.Pattern
	repo, err := git.PlainOpenWithOptions(root, &git.PlainOpenOptions{DetectDotGit: true})
	if err == nil {
		if st, err := repo.Storer.Index(); err == nil {
			for _, e := range st.Entries {
				tracked[filepath.ToSlash(e.Name)] = true
			}
		}
		if wt, err := repo.Worktree(); err == nil {
			if ps, err := gitignore.ReadPatterns(wt.Filesystem, nil); err == nil {
				patterns = append(patterns, ps...)
			}
		}
	}
	fs := osfs.New("/")
	if ps, err := gitignore.LoadGlobalPatterns(fs); err == nil {
		patterns = append(ps, patterns...)
	}
	if ps, err := gitignore.LoadSystemPatterns(fs); err == nil {
		patterns = append(ps, patterns...)
	}
	for _, ex := range exclude {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(ex, nil))
	}
	matcher := gitignore.NewMatcher(patterns)

	seen := map[string]bool{}
	var files []string
	add := func(rel string) {
		rel = filepath.ToSlash(rel)
		if rel == "" || seen[rel] {
			return
		}
		seen[rel] = true
		files = append(files, rel)
	}
	// Tracked paths are listed even when a gitignore pattern would match them.
	for rel := range tracked {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if !staysInRoot(rel) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			continue
		}
		add(rel)
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		name := d.Name()
		if name == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		if matcher.Match(parts, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if tracked[rel] {
			return nil
		}
		add(rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// staysInRoot reports whether rel is a relative path inside the index root.
// Absolute paths and paths whose clean form starts with ".." are false.
func staysInRoot(rel string) bool {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || filepath.IsAbs(clean) {
		return false
	}
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func buildTree(files []string) *Node {
	root := &Node{Name: "", Dir: true}
	for _, f := range files {
		parts := strings.Split(f, "/")
		cur := root
		for i, part := range parts {
			last := i == len(parts)-1
			var child *Node
			for _, k := range cur.Kids {
				if k.Name == part {
					child = k
					break
				}
			}
			if child == nil {
				child = &Node{Name: part, Dir: !last}
				if last {
					child.Path = f
				} else {
					if cur.Path == "" {
						child.Path = part
					} else {
						child.Path = cur.Path + "/" + part
					}
				}
				cur.Kids = append(cur.Kids, child)
			}
			cur = child
		}
	}
	return root
}
