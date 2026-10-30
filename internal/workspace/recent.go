package workspace

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Recent is a most-recent-first list of file paths.
type Recent struct {
	Paths []string `toml:"paths"`
}

const recentCap = 50

// LoadRecent reads a recent-files file. A missing file is an empty list.
func LoadRecent(path string) (Recent, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Recent{}, nil
		}
		return Recent{}, err
	}
	var r Recent
	if err := toml.Unmarshal(b, &r); err != nil {
		return Recent{}, err
	}
	return r, nil
}

// PushRecent records path at the front and writes the file.
func PushRecent(file, path string) (Recent, error) {
	r, err := LoadRecent(file)
	if err != nil {
		r = Recent{}
	}
	path = filepath.Clean(path)
	out := []string{path}
	for _, p := range r.Paths {
		if filepath.Clean(p) == path {
			continue
		}
		out = append(out, p)
		if len(out) >= recentCap {
			break
		}
	}
	r.Paths = out
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return r, err
	}
	b, err := toml.Marshal(r)
	if err != nil {
		return r, err
	}
	return r, os.WriteFile(file, b, 0o644)
}
