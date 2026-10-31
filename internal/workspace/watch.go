package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Change is one filesystem event delivered after the quiet period.
// Kind is create, write, remove, or rename.
type Change struct {
	Path string
	Kind string
}

// Watch watches root and calls onChange after a short quiet period.
// If the kernel runs out of watches it calls onLimit once and returns.
// The caller should then rescan when the window gains focus.
func Watch(root string, stop <-chan struct{}, onChange func([]Change), onLimit func()) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	limited := false
	noteLimit := func() {
		if limited {
			return
		}
		limited = true
		if onLimit != nil {
			onLimit()
		}
	}
	addDir := func(path string) bool {
		if limited {
			return false
		}
		if err := w.Add(path); err != nil && errors.Is(err, syscall.ENOSPC) {
			noteLimit()
			return false
		}
		return true
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !addDir(path) {
			return filepath.SkipAll
		}
		return nil
	})
	if limited {
		return nil
	}
	var timer *time.Timer
	var mu sync.Mutex
	var pending []Change
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	fire := func() {
		mu.Lock()
		batch := pending
		pending = nil
		mu.Unlock()
		if onChange != nil && len(batch) > 0 {
			onChange(batch)
		}
	}
	note := func(path, kind string) {
		if path == "" || kind == "" {
			return
		}
		mu.Lock()
		pending = append(pending, Change{Path: path, Kind: kind})
		mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(100*time.Millisecond, fire)
	}
	for {
		select {
		case <-stop:
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					if !addDir(ev.Name) {
						return nil
					}
				}
			}
			note(ev.Name, changeKind(ev.Op))
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if errors.Is(err, syscall.ENOSPC) {
				noteLimit()
				return nil
			}
		}
	}
}

// changeKind maps an fsnotify op to a FileChange kind.
// Remove wins over rename, then create, then write. Chmod is ignored.
func changeKind(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Remove != 0:
		return "remove"
	case op&fsnotify.Rename != 0:
		return "rename"
	case op&fsnotify.Create != 0:
		return "create"
	case op&fsnotify.Write != 0:
		return "write"
	default:
		return ""
	}
}
