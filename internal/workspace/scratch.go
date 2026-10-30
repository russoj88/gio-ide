package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Create makes dir (0755) and writes the next scratch file (0644).
// ext must be ".go" or ".txt". The first name is scratch.go or scratch.txt,
// then scratch2, scratch3, and so on, using the lowest free number.
// body may be empty. Create returns the absolute path.
func Create(dir, ext string, body []byte) (string, error) {
	if ext != ".go" && ext != ".txt" {
		return "", fmt.Errorf("create scratch: extension must be .go or .txt, not %q", ext)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("create scratch: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create scratch: %w", err)
	}
	if err := os.Chmod(abs, 0o755); err != nil {
		return "", fmt.Errorf("create scratch: %w", err)
	}
	for n := 1; ; n++ {
		name := "scratch" + ext
		if n > 1 {
			name = fmt.Sprintf("scratch%d%s", n, ext)
		}
		path := filepath.Join(abs, name)
		_, err := os.Lstat(path)
		if err == nil {
			continue
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("create scratch: %w", err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return "", fmt.Errorf("create scratch: %w", err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			return "", fmt.Errorf("create scratch: %w", err)
		}
		return path, nil
	}
}

// List returns absolute paths of files directly in dir, sorted by base name.
// A missing directory is an empty list and a nil error. Subdirectories are skipped.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list scratches: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("list scratches: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, filepath.Join(abs, e.Name()))
	}
	slices.SortFunc(out, func(a, b string) int {
		return strings.Compare(filepath.Base(a), filepath.Base(b))
	})
	return out, nil
}

// Contains reports whether path is dir or a file inside dir, after cleaning.
// A path that escapes dir with ".." is false.
func Contains(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Rename changes the base name of oldPath, which must be a path inside dir
// and not dir itself. newBase must be a single path element: no separators,
// and not "", ".", or "..". If newBase has no extension, the old extension is
// kept, so scratch.go renamed to "note" becomes note.go. A newBase that already
// has an extension is used as given. Rename refuses when a different target
// exists. Renaming a file to its current name returns that path. The result
// is the new absolute path.
func Rename(dir, oldPath, newBase string) (string, error) {
	if newBase == "" || newBase == "." || newBase == ".." || strings.ContainsRune(newBase, filepath.Separator) {
		return "", fmt.Errorf("rename scratch: %q is not a single path element", newBase)
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("rename scratch: %w", err)
	}
	oldAbs, err := filepath.Abs(oldPath)
	if err != nil {
		return "", fmt.Errorf("rename scratch: %w", err)
	}
	if !Contains(dirAbs, oldAbs) || filepath.Clean(oldAbs) == filepath.Clean(dirAbs) {
		return "", fmt.Errorf("rename scratch: %s is outside %s", oldPath, dir)
	}
	base := newBase
	if filepath.Ext(newBase) == "" {
		base += filepath.Ext(oldAbs)
	}
	newPath := filepath.Join(filepath.Dir(oldAbs), base)
	if newPath == oldAbs {
		return oldAbs, nil
	}
	if _, err := os.Lstat(newPath); err == nil {
		return "", fmt.Errorf("rename scratch: %s already exists", newPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("rename scratch: %w", err)
	}
	if err := os.Rename(oldAbs, newPath); err != nil {
		return "", fmt.Errorf("rename scratch: %w", err)
	}
	return newPath, nil
}

// renameFile renames a file. Tests replace it to simulate a cross-device EXDEV.
var renameFile = os.Rename

// Move moves oldPath to dest. Dest's parent must already exist; Move does not
// create it. Move refuses when dest exists. It uses renameFile (os.Rename).
// If that returns syscall.EXDEV, Move copies to dest and then removes the
// source. A copy failure leaves the source in place. Move returns dest.
func Move(oldPath, dest string) (string, error) {
	if _, err := os.Lstat(dest); err == nil {
		return "", fmt.Errorf("move scratch: %s already exists", dest)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("move scratch: %w", err)
	}
	if err := renameFile(oldPath, dest); err == nil {
		return dest, nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return "", fmt.Errorf("move scratch: %w", err)
	}
	if err := copyScratch(oldPath, dest); err != nil {
		return "", err
	}
	if err := os.Remove(oldPath); err != nil {
		return "", fmt.Errorf("move scratch: %w", err)
	}
	return dest, nil
}

func copyScratch(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("move scratch: %w", err)
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return fmt.Errorf("move scratch: %w", err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, st.Mode().Perm())
	if err != nil {
		return fmt.Errorf("move scratch: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		if rmErr := os.Remove(dst); rmErr != nil {
			return fmt.Errorf("move scratch: %w", errors.Join(copyErr, closeErr, rmErr))
		}
		if copyErr != nil {
			return fmt.Errorf("move scratch: %w", copyErr)
		}
		return fmt.Errorf("move scratch: %w", closeErr)
	}
	if err := os.Chmod(dst, st.Mode().Perm()); err != nil {
		if rmErr := os.Remove(dst); rmErr != nil {
			return fmt.Errorf("move scratch: %w", errors.Join(err, rmErr))
		}
		return fmt.Errorf("move scratch: %w", err)
	}
	return nil
}
