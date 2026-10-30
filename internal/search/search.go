// Package search finds text in a file list. The UI calls it from a worker.
package search

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

// Query is one project search.
type Query struct {
	Pattern                         string
	Regex, CaseSensitive, WholeWord bool
	Include, Exclude                []string
}

// Result is one match.
type Result struct {
	Path                 string
	Line, Col            int
	LineText             string
	MatchStart, MatchEnd int
}

// Engine searches files and reports them in batches.
type Engine interface {
	Search(ctx context.Context, root string, files []string, q Query, out func([]Result)) error
}

// Builtin is the in-process searcher.
type Builtin struct{}

// Search scans files with runtime.NumCPU workers.
// Files larger than 8 MiB and files with a NUL in the first 8 KiB are skipped.
// out receives batches of at most 200 results.
func (Builtin) Search(ctx context.Context, root string, files []string, q Query, out func([]Result)) error {
	if q.Pattern == "" {
		return nil
	}
	var re *regexp.Regexp
	// Case-insensitive literals use the regexp so match indexes stay
	// byte offsets in the original line. strings.ToLower can change length.
	if q.Regex || q.WholeWord || !q.CaseSensitive {
		pat := q.Pattern
		if !q.Regex {
			pat = regexp.QuoteMeta(pat)
		}
		if q.WholeWord {
			pat = `\b` + pat + `\b`
		}
		if !q.CaseSensitive {
			pat = "(?i)" + pat
		}
		compiled, err := regexp.Compile(pat)
		if err != nil {
			return err
		}
		re = compiled
	}
	workers := max(runtime.NumCPU(), 1)
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var batch []Result
	flush := func(force bool) {
		mu.Lock()
		defer mu.Unlock()
		if len(batch) == 0 {
			return
		}
		if !force && len(batch) < 200 {
			return
		}
		n := min(len(batch), 200)
		out(append([]Result(nil), batch[:n]...))
		batch = append([]Result(nil), batch[n:]...)
	}
	for range workers {
		wg.Go(func() {
			for rel := range jobs {
				if ctx.Err() != nil {
					return
				}
				if skipped(rel, q.Include, q.Exclude) {
					continue
				}
				hits := scanFile(root, rel, q, re)
				if len(hits) == 0 {
					continue
				}
				mu.Lock()
				batch = append(batch, hits...)
				full := len(batch) >= 200
				mu.Unlock()
				if full {
					flush(true)
				}
			}
		})
	}
	for _, f := range files {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- f:
		}
	}
	close(jobs)
	wg.Wait()
	for {
		mu.Lock()
		left := len(batch)
		mu.Unlock()
		if left == 0 {
			break
		}
		flush(true)
	}
	return nil
}

func skipped(rel string, include, exclude []string) bool {
	rel = filepath.ToSlash(rel)
	if len(include) > 0 && !matchAny(rel, include) {
		return true
	}
	return matchAny(rel, exclude)
}

func matchAny(rel string, patterns []string) bool {
	base := pathBase(rel)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		trim := strings.TrimSuffix(p, "/")
		if rel == trim || strings.HasPrefix(rel, trim+"/") || strings.Contains(rel, "/"+trim+"/") {
			return true
		}
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
		if ok, _ := filepath.Match(p, rel); ok {
			return true
		}
	}
	return false
}

func pathBase(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[i+1:]
	}
	return rel
}

func scanFile(root, rel string, q Query, re *regexp.Regexp) []Result {
	path := filepath.Join(root, filepath.FromSlash(rel))
	got, err := filepath.Rel(root, path)
	if err != nil || got == ".." || strings.HasPrefix(got, ".."+string(filepath.Separator)) {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() > 8<<20 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	head := make([]byte, 8192)
	n, _ := f.Read(head)
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil
	}
	var out []Result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if !utf8.Valid(line) {
			continue
		}
		text := string(line)
		if re != nil {
			idx := re.FindAllStringIndex(text, -1)
			for _, pair := range idx {
				out = append(out, Result{
					Path: rel, Line: lineNo, Col: pair[0] + 1,
					LineText: text, MatchStart: pair[0], MatchEnd: pair[1],
				})
			}
			continue
		}
		hay, needle := text, q.Pattern
		from := 0
		for {
			i := strings.Index(hay[from:], needle)
			if i < 0 {
				break
			}
			at := from + i
			out = append(out, Result{
				Path: rel, Line: lineNo, Col: at + 1,
				LineText: text, MatchStart: at, MatchEnd: at + len(needle),
			})
			from = at + len(needle)
			if from >= len(hay) {
				break
			}
		}
	}
	return out
}
