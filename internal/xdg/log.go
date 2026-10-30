package xdg

import (
	"fmt"
	"os"
	"sync"
)

const (
	logMaxBytes = 5 << 20
	logKeep     = 5 // gio-ide.log plus four rotated files
)

// Log is an append-only log that keeps logKeep files.
type Log struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int
}

// OpenLog opens $XDG_STATE_HOME/gio-ide/logs/gio-ide.log, creating directories.
func OpenLog() (*Log, error) {
	if err := EnsureDir(LogDir()); err != nil {
		return nil, err
	}
	path := LogPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Log{path: path, f: f, size: int(st.Size())}, nil
}

func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return 0, os.ErrClosed
	}
	if l.size+len(p) > logMaxBytes {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += n
	return n, err
}

func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

func (l *Log) rotate() error {
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
	// gio-ide.log.4 is the oldest. Drop it, shift the rest, then move the live file.
	oldest := fmt.Sprintf("%s.%d", l.path, logKeep-1)
	_ = os.Remove(oldest)
	for i := logKeep - 2; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", l.path, i)
		dst := fmt.Sprintf("%s.%d", l.path, i+1)
		_ = os.Rename(src, dst)
	}
	_ = os.Rename(l.path, l.path+".1")
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.f = f
	l.size = 0
	return nil
}
