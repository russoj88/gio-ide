package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch watches root and calls onChange after a short quiet period.
// If the kernel runs out of watches it calls onLimit once and returns.
// The caller should then rescan when the window gains focus.
func Watch(root string, stop <-chan struct{}, onChange func(), onLimit func()) error {
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
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	fire := func() {
		if onChange != nil {
			onChange()
		}
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
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(100*time.Millisecond, fire)
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
