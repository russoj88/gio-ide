package format

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Gofix applies the fixes that "go fix" would apply to src.
// filename is the file path; its directory is the package.
// The call stops after 30 seconds. It runs the go command.
//
// src is the buffer, which may differ from disk. go fix reads it
// through an overlay, so this call does not write filename or any
// other file in the package. Fixes that touch only other files are
// dropped. A generated file is returned unchanged. When go fix
// reports no diff for this file, src is returned unchanged.
func Gofix(ctx context.Context, filename string, src []byte) ([]byte, error) {
	if filename == "" {
		return nil, errors.New("go fix: file path is empty")
	}
	if generatedGo(src) {
		return src, nil
	}
	abs, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("go fix: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	overlay, err := os.CreateTemp("", "gio-ide-gofix-*")
	if err != nil {
		return nil, fmt.Errorf("go fix: %w", err)
	}
	overlayName := overlay.Name()
	defer os.Remove(overlayName)
	if _, err := overlay.Write(src); err != nil {
		overlay.Close()
		return nil, fmt.Errorf("go fix: %w", err)
	}
	if err := overlay.Close(); err != nil {
		return nil, fmt.Errorf("go fix: %w", err)
	}

	cfg, err := os.CreateTemp("", "gio-ide-gofix-overlay-*.json")
	if err != nil {
		return nil, fmt.Errorf("go fix: %w", err)
	}
	cfgName := cfg.Name()
	defer os.Remove(cfgName)
	js, err := json.Marshal(struct {
		Replace map[string]string `json:"Replace"`
	}{Replace: map[string]string{abs: overlayName}})
	if err != nil {
		cfg.Close()
		return nil, fmt.Errorf("go fix: %w", err)
	}
	if _, err := cfg.Write(js); err != nil {
		cfg.Close()
		return nil, fmt.Errorf("go fix: %w", err)
	}
	if err := cfg.Close(); err != nil {
		return nil, fmt.Errorf("go fix: %w", err)
	}

	cmd := exec.CommandContext(ctx, "go", "fix", "-overlay="+cfgName, "-diff", ".")
	cmd.Dir = filepath.Dir(abs)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		diffPresent := errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(bytes.TrimSpace(stdout.Bytes())) > 0
		if !diffPresent {
			detail := strings.TrimSpace(stderr.String())
			if detail == "" {
				return nil, fmt.Errorf("go fix: %w", err)
			}
			return nil, fmt.Errorf("go fix: %s: %w", detail, err)
		}
	}
	text := stdout.String()
	if strings.TrimSpace(text) == "" {
		return src, nil
	}
	section, ok := fileDiff(text, overlayName)
	if !ok {
		if strings.Contains(text, " (old)\n") {
			return src, nil
		}
		return nil, fmt.Errorf("go fix: unexpected output: %s", oneLine(text))
	}
	out, err := applyUnified(string(src), section)
	if err != nil {
		return nil, fmt.Errorf("go fix: apply diff: %w", err)
	}
	return []byte(out), nil
}

func generatedGo(src []byte) bool {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments)
	if err != nil {
		return false
	}
	return ast.IsGenerated(f)
}

// fileDiff returns the unified diff section whose old file is overlayPath.
// go fix labels that file "<overlay> (old)".
func fileDiff(diff, overlayPath string) (string, bool) {
	oldHeader := "--- " + overlayPath + " (old)"
	lines := strings.Split(diff, "\n")
	start := -1
	for i, line := range lines {
		if line == oldHeader {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for i := start + 2; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "--- ") && strings.HasSuffix(lines[i], " (old)") &&
			i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ") && strings.HasSuffix(lines[i+1], " (new)") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), true
}

// applyUnified applies one file's unified diff to src.
// Context and deleted lines must match src. A mismatch is an error.
func applyUnified(src, diff string) (string, error) {
	lines := strings.Split(diff, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	srcLines := strings.SplitAfter(src, "\n")
	var b strings.Builder
	si := 0
	inHunk := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "@@") {
			from, err := hunkStart(line)
			if err != nil {
				return "", err
			}
			if from < 1 {
				return "", fmt.Errorf("hunk %q starts before line 1", line)
			}
			target := from - 1
			if target < si || target > len(srcLines) {
				return "", fmt.Errorf("hunk %q starts at line %d after line %d", line, from, si+1)
			}
			for si < target {
				b.WriteString(srcLines[si])
				si++
			}
			inHunk = true
			continue
		}
		if !inHunk || line == `\ No newline at end of file` {
			continue
		}
		if line == "" {
			return "", errors.New("empty line in unified diff")
		}
		switch line[0] {
		case ' ', '-':
			text, consumed := diffText(lines, i)
			i = consumed
			if si >= len(srcLines) || srcLines[si] != text {
				got := ""
				if si < len(srcLines) {
					got = srcLines[si]
				}
				return "", fmt.Errorf("diff does not match source at line %d: diff %q source %q", si+1, text, got)
			}
			if line[0] == ' ' {
				b.WriteString(srcLines[si])
			}
			si++
		case '+':
			text, consumed := diffText(lines, i)
			i = consumed
			b.WriteString(text)
		default:
			return "", fmt.Errorf("invalid diff line %q", line)
		}
	}
	for si < len(srcLines) {
		b.WriteString(srcLines[si])
		si++
	}
	return b.String(), nil
}

func hunkStart(line string) (int, error) {
	rest, ok := strings.CutPrefix(line, "@@ -")
	if !ok {
		return 0, fmt.Errorf("invalid hunk %q", line)
	}
	num := rest
	if i := strings.IndexAny(rest, ", "); i >= 0 {
		num = rest[:i]
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return 0, fmt.Errorf("invalid hunk %q: %w", line, err)
	}
	return n, nil
}

// diffText returns the source text for a unified-diff body line.
// The diff stores the source newline as the line break, and marks a
// missing final newline with the following "\ No newline at end of file" line.
func diffText(lines []string, i int) (string, int) {
	raw := lines[i][1:]
	if i+1 < len(lines) && lines[i+1] == `\ No newline at end of file` {
		return raw, i + 1
	}
	return raw + "\n", i
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
