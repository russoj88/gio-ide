package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/russoj88/gio-ide/internal/bus"
)

func TestReadExitErrIgnoresPTYHangup(t *testing.T) {
	term := New(1, 10, nil)
	hangup := &os.PathError{Op: "read", Path: "/dev/ptmx", Err: syscall.EIO}
	if got := term.readExitErr(hangup); got != "" {
		t.Fatal(got)
	}
	if got := term.readExitErr(io.EOF); got != "" {
		t.Fatal(got)
	}
	if got := term.readExitErr(os.ErrClosed); got != "" {
		t.Fatal(got)
	}
	if got := term.readExitErr(errors.New("boom")); got != "boom" {
		t.Fatal(got)
	}
	if err := term.Close(); err != nil {
		t.Fatal(err)
	}
	if got := term.readExitErr(errors.New("boom")); got != "" {
		t.Fatal(got)
	}
}

func TestClosePostsNoPTYError(t *testing.T) {
	var exits []bus.TerminalExited
	term := New(7, 10, func(ev bus.Event) {
		ex, ok := ev.(bus.TerminalExited)
		if ok {
			exits = append(exits, ex)
		}
	})
	if err := term.Start(t.Context(), "/bin/sh", t.TempDir(), 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := term.Close(); err != nil {
		t.Fatal(err)
	}
	if len(exits) == 0 {
		t.Fatal("no exit")
	}
	for _, ex := range exits {
		if ex.Session != 7 || ex.Err != "" {
			t.Fatalf("%+v", ex)
		}
	}
}

func TestFailedShellReportsExitStatus(t *testing.T) {
	got := make(chan string, 4)
	term := New(3, 10, func(ev bus.Event) {
		ex, ok := ev.(bus.TerminalExited)
		if ok {
			got <- ex.Err
		}
	})
	if err := term.Start(t.Context(), "/bin/sh", t.TempDir(), 80, 24); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := term.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := term.Write([]byte("exit 3\n")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var notes []string
	for {
		select {
		case err := <-got:
			notes = append(notes, err)
			if slices.Contains(notes, "exit status 3") {
				return
			}
		case <-ctx.Done():
			t.Fatalf("notices %q: %v", notes, ctx.Err())
		}
	}
}
