package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/russoj88/gio-ide/internal/bus"
)

// TestRecordedStreams feeds reduced recordings of the sequences bash, ls --color,
// less, and vim emit, and checks the screen.
func TestRecordedStreams(t *testing.T) {
	t.Run("bash prompt", func(t *testing.T) {
		sc := NewScreen(60, 4, 100)
		// OSC 0 title, bold green user, default, bold blue directory, reset, prompt.
		sc.Feed([]byte("\x1b]0;bash\x07\x1b[01;32muser@host\x1b[00m:\x1b[01;34m~\x1b[00m$ "))
		got := sc.LineText(0)
		if got != "user@host:~$" {
			t.Fatalf("prompt %q", got)
		}
		if sc.curX != 13 {
			t.Fatalf("prompt cursor %d", sc.curX)
		}
		if sc.title != "bash" {
			t.Fatalf("title %q", sc.title)
		}
		c := sc.grid()[0]
		if !c.A.Bold || c.A.FG.Kind != 1 || c.A.FG.Idx != 2 {
			t.Fatalf("user color %+v", c.A)
		}
	})

	t.Run("ls --color", func(t *testing.T) {
		sc := NewScreen(40, 4, 100)
		// Bold blue directory, bold green executable, plain file.
		sc.Feed([]byte("\x1b[01;34mdir\x1b[0m  \x1b[01;32mscript\x1b[0m  file\r\n"))
		if got := sc.LineText(0); got != "dir  script  file" {
			t.Fatalf("ls %q", got)
		}
		dir := sc.grid()[0]
		if !dir.A.Bold || dir.A.FG.Idx != 4 {
			t.Fatalf("dir attr %+v", dir.A)
		}
		// "dir  " is 5 cells, script starts at column 5.
		ex := sc.grid()[5]
		if ex.R != 's' || !ex.A.Bold || ex.A.FG.Idx != 2 {
			t.Fatalf("script cell %+v", ex)
		}
	})

	t.Run("less", func(t *testing.T) {
		sc := NewScreen(20, 3, 100)
		sc.Feed([]byte("primary\r\n"))
		// less enters the alternate screen, hides the cursor, draws a line, then quits.
		sc.Feed([]byte("\x1b[?1049h\x1b[?25l\x1b[HREADME\r\nline\x1b[?25h\x1b[?1049l"))
		if sc.usingAlt {
			t.Fatal("less quit left the alternate screen up")
		}
		if got := sc.LineText(0); got != "primary" {
			t.Fatalf("restored %q", got)
		}
		if !sc.cursorOn {
			t.Fatal("cursor stayed hidden")
		}
	})

	t.Run("vim", func(t *testing.T) {
		sc := NewScreen(24, 4, 100)
		sc.Feed([]byte("before\r\n"))
		// vim start: alt screen, app cursor, a status-like row, then quit.
		sc.Feed([]byte("\x1b[?1049h\x1b[?1h\x1b[2J\x1b[H\x1b[1;1Hpackage main\x1b[4;1H\x1b[7m~\x1b[27m"))
		if !sc.usingAlt || !sc.appCursor {
			t.Fatalf("vim start alt=%v app=%v", sc.usingAlt, sc.appCursor)
		}
		if got := sc.LineText(0); got != "package main" {
			t.Fatalf("vim text %q", got)
		}
		mark := sc.grid()[3*sc.cols]
		if mark.R != '~' || !mark.A.Inverse {
			t.Fatalf("vim tilde %+v", mark)
		}
		sc.Feed([]byte("\x1b[?1l\x1b[?1049l"))
		if sc.usingAlt || sc.appCursor {
			t.Fatal("vim quit did not restore the primary screen")
		}
		if got := sc.LineText(0); got != "before" {
			t.Fatalf("after vim %q", got)
		}
	})
}

func TestWideRuneAndUnknown(t *testing.T) {
	sc := NewScreen(6, 2, 10)
	sc.Feed([]byte("你"))
	if sc.grid()[0].W != 2 || sc.grid()[1].W != -1 {
		t.Fatalf("wide %+v %+v", sc.grid()[0], sc.grid()[1])
	}
	if sc.curX != 2 {
		t.Fatalf("cursor %d", sc.curX)
	}
	before := sc.unknown
	sc.Feed([]byte("\x1b[?9999h\x1b[99Z"))
	if sc.unknown <= before {
		t.Fatal("unknown sequences were not counted")
	}
}

func TestScrollRegionAndHistory(t *testing.T) {
	sc := NewScreen(4, 3, 10)
	sc.Feed([]byte("a\r\nb\r\nc\r\n"))
	if sc.LineText(0) != "b" || sc.HistoryCount() != 1 {
		t.Fatalf("line %q hist %d", sc.LineText(0), sc.HistoryCount())
	}
	sc.Feed([]byte("\x1b[1;2r\x1b[2;1HA\r\nB"))
	if sc.scrollTop != 0 || sc.scrollBot != 1 {
		t.Fatalf("region %d %d", sc.scrollTop, sc.scrollBot)
	}
}

func TestTruecolorAndBracketed(t *testing.T) {
	sc := NewScreen(8, 2, 4)
	sc.Feed([]byte("\x1b[38;2;1;2;3mX\x1b[?2004h"))
	c := sc.grid()[0]
	if c.A.FG.Kind != 2 || c.A.FG.R != 1 || c.A.FG.G != 2 || c.A.FG.B != 3 {
		t.Fatalf("rgb %+v", c.A.FG)
	}
	if !sc.bracketed {
		t.Fatal("bracketed paste mode")
	}
	if string(BracketPaste("hi", true)) != "\x1b[200~hi\x1b[201~" {
		t.Fatal("paste wrap")
	}
}

func TestPTYEcho(t *testing.T) {
	dirty := make(chan struct{}, 256)
	term := New(1, 100, func(e bus.Event) {
		if _, ok := e.(bus.TerminalDirty); !ok {
			return
		}
		select {
		case dirty <- struct{}{}:
		default:
		}
	})
	if err := term.Start(t.Context(), "/bin/sh", t.TempDir(), 80, 24); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = term.Close() })
	if _, err := term.Write([]byte("printf 'gio-ide-pty-ok\\n'\n")); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		if strings.Contains(term.Snapshot().Text(), "gio-ide-pty-ok") {
			return
		}
		select {
		case <-dirty:
		case <-timer.C:
			if strings.Contains(term.Snapshot().Text(), "gio-ide-pty-ok") {
				return
			}
			t.Fatalf("pty screen:\n%s", term.Snapshot().Text())
		}
	}
}

func TestShellEnv(t *testing.T) {
	env := shellEnv([]string{"TERM=dumb", "COLORTERM=no", "HOME=/tmp"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "TERM=dumb") || strings.Contains(joined, "COLORTERM=no") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "TERM=xterm-256color") || !strings.Contains(joined, "COLORTERM=truecolor") {
		t.Fatal(joined)
	}
}

func (sc *Screen) HistoryCount() int { return len(sc.hist) }
