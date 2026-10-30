package search

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

// RG runs `rg --json` when the user selects the rg engine.
type RG struct{}

// Search runs ripgrep. It returns an error when rg is not on PATH.
func (RG) Search(ctx context.Context, root string, files []string, q Query, out func([]Result)) error {
	if q.Pattern == "" || len(files) == 0 {
		return nil
	}
	if _, err := exec.LookPath("rg"); err != nil {
		return errors.New("rg is not installed")
	}
	args := []string{"--json", "--line-number"}
	if !q.CaseSensitive {
		args = append(args, "--ignore-case")
	}
	if !q.Regex {
		args = append(args, "--fixed-strings")
	}
	if q.WholeWord {
		args = append(args, "--word-regexp")
	}
	for _, p := range q.Include {
		args = append(args, "--glob", p)
	}
	for _, p := range q.Exclude {
		args = append(args, "--glob", "!"+strings.TrimPrefix(p, "!"))
	}
	// Flags stay before "--". Ripgrep treats later tokens as paths.
	args = append(args, "--", q.Pattern)
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var batch []Result
	flush := func() {
		if len(batch) == 0 {
			return
		}
		n := min(len(batch), 200)
		out(append([]Result(nil), batch[:n]...))
		batch = append([]Result(nil), batch[n:]...)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	allow := map[string]bool{}
	for _, f := range files {
		allow[f] = true
	}
	for sc.Scan() {
		var ev rgEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Type != "match" {
			continue
		}
		rel := ev.Data.Path.Text
		if len(allow) > 0 && !allow[rel] {
			continue
		}
		line := strings.TrimRight(ev.Data.Lines.Text, "\n")
		for _, sm := range ev.Data.Submatches {
			batch = append(batch, Result{
				Path: rel, Line: ev.Data.LineNumber, Col: sm.Start + 1,
				LineText: line, MatchStart: sm.Start, MatchEnd: sm.End,
			})
			if len(batch) >= 200 {
				flush()
			}
		}
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		var exitErr *exec.ExitError
		// rg exits 1 when nothing matches. That is an empty result, not a failure.
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 {
			waitErr = nil
		}
	}
	for len(batch) > 0 {
		flush()
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return waitErr
}

type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}
