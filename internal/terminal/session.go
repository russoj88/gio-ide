// Package terminal is the PTY session and the on-screen terminal grid.
package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"

	"github.com/russoj88/gio-ide/internal/bus"
)

// Session is the PTY handle the shell drives.
// The UI goroutine may call Snapshot. Start, Write, Resize, and Close run off the UI goroutine.
type Session interface {
	Start(ctx context.Context, shell, dir string, cols, rows int) error
	Write(p []byte) (int, error)
	Resize(cols, rows int) error
	Snapshot() ScreenSnapshot
	Close() error
}

// Term is one terminal tab: a PTY, a screen, and scrollback.
type Term struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	id     uint64
	scr    *Screen
	ptmx   *os.File
	cmd    *exec.Cmd
	post   func(bus.Event)
	closed bool
	follow bool
	top    int
	selOn  bool
	sel    [4]int
}

// New returns a terminal that is not started yet.
func New(id uint64, scrollback int, post func(bus.Event)) *Term {
	if post == nil {
		post = func(bus.Event) {}
	}
	if scrollback <= 0 {
		scrollback = 10000
	}
	sc := NewScreen(80, 24, scrollback)
	sc.logf = func(msg string) {}
	return &Term{id: id, scr: sc, post: post, follow: true}
}

// Start launches the shell on a PTY. An empty shell uses $SHELL, then /bin/sh.
func (t *Term) Start(ctx context.Context, shell, dir string, cols, rows int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if cols < 2 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	cmd := exec.CommandContext(ctx, shell)
	cmd.Dir = dir
	cmd.Env = shellEnv(os.Environ())
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return err
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		f.Close()
		_ = cmd.Process.Kill()
		return errors.New("terminal closed")
	}
	t.scr.Resize(cols, rows)
	t.ptmx = f
	t.cmd = cmd
	t.wg.Add(2)
	t.mu.Unlock()
	go t.readLoop(f)
	go t.wait(cmd)
	return nil
}

func (t *Term) readLoop(f *os.File) {
	defer t.wg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.scr.Feed(buf[:n])
			t.mu.Unlock()
			t.post(bus.TerminalDirty{Session: t.id})
		}
		if err != nil {
			t.post(bus.TerminalExited{Session: t.id, Err: t.readExitErr(err)})
			return
		}
	}
}

// readExitErr is the notice for a PTY read that ended the session.
// EOF, a closed file, and EIO mean the slave or the master went away.
// Linux reports that hangup as EIO ("read /dev/ptmx: input/output error"),
// including when Close kills the session. Close sets closed first.
func (t *Term) readExitErr(err error) string {
	if err == nil || t.isClosed() {
		return ""
	}
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO) {
		return ""
	}
	return err.Error()
}

func (t *Term) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func (t *Term) wait(cmd *exec.Cmd) {
	defer t.wg.Done()
	err := cmd.Wait()
	// Kill from Close surfaces here as "signal: killed" or "signal: hangup".
	if err == nil || t.isClosed() {
		return
	}
	t.post(bus.TerminalExited{Session: t.id, Err: err.Error()})
}

// Write sends bytes to the PTY.
func (t *Term) Write(p []byte) (int, error) {
	t.mu.Lock()
	f := t.ptmx
	t.follow = true
	t.mu.Unlock()
	if f == nil {
		return 0, errors.New("terminal is not running")
	}
	return f.Write(p)
}

// Resize updates the PTY and the screen.
func (t *Term) Resize(cols, rows int) error {
	if cols < 2 || rows < 1 {
		return nil
	}
	t.mu.Lock()
	f := t.ptmx
	t.scr.Resize(cols, rows)
	t.mu.Unlock()
	if f == nil {
		return nil
	}
	return pty.Setsize(f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// Snapshot returns the visible grid, including scrollback when the view is not pinned.
func (t *Term) Snapshot() ScreenSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	live := t.scr.Snapshot()
	if t.follow || len(t.scr.hist) == 0 {
		return live
	}
	return t.viewLocked(live)
}

func (t *Term) viewLocked(live ScreenSnapshot) ScreenSnapshot {
	hist := t.scr.hist
	rows := live.Rows
	cols := live.Cols
	if t.top < 0 {
		t.top = 0
	}
	if t.top > len(hist) {
		t.top = len(hist)
	}
	cells := make([]Cell, cols*rows)
	for y := range rows {
		src := t.rowLocked(hist, live, t.top+y)
		if src == nil {
			continue
		}
		n := min(len(src), cols)
		copy(cells[y*cols:y*cols+n], src[:n])
	}
	live.Cells = cells
	live.CursorOn = false
	live.History = len(hist)
	return live
}

func (t *Term) rowLocked(hist [][]Cell, live ScreenSnapshot, idx int) []Cell {
	if idx < len(hist) {
		return hist[idx]
	}
	y := idx - len(hist)
	if y < 0 || y >= live.Rows {
		return nil
	}
	return live.Cells[y*live.Cols : (y+1)*live.Cols]
}

// Scroll moves the view. A positive delta goes toward older lines.
func (t *Term) Scroll(delta int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if delta == 0 {
		return
	}
	if t.follow {
		t.top = len(t.scr.hist)
		t.follow = false
	}
	t.top -= delta
	if t.top < 0 {
		t.top = 0
	}
	if t.top >= len(t.scr.hist) {
		t.follow = true
		t.top = len(t.scr.hist)
	}
}

// Title is the OSC 0 or OSC 2 title.
func (t *Term) Title() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scr.title
}

// Bracketed reports DECSET 2004.
func (t *Term) Bracketed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scr.bracketed
}

// AppCursor reports DECSET 1.
func (t *Term) AppCursor() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scr.appCursor
}

// SetSelection records a cell range on the visible grid. x2,y2 is exclusive in the usual min/max sense after normalize.
func (t *Term) SetSelection(x0, y0, x1, y1 int) {
	t.mu.Lock()
	t.sel = [4]int{x0, y0, x1, y1}
	t.selOn = true
	t.mu.Unlock()
}

// ClearSelection drops the selection.
func (t *Term) ClearSelection() {
	t.mu.Lock()
	t.selOn = false
	t.mu.Unlock()
}

// Selection returns the visible cell range, inclusive.
func (t *Term) Selection() (x0, y0, x1, y1 int, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.selOn {
		return 0, 0, 0, 0, false
	}
	x0, y0, x1, y1 = t.sel[0], t.sel[1], t.sel[2], t.sel[3]
	if y0 > y1 || (y0 == y1 && x0 > x1) {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	return x0, y0, x1, y1, true
}

// SelectedText returns the current selection, one row per line.
func (t *Term) SelectedText() string {
	snap := t.Snapshot()
	t.mu.Lock()
	on := t.selOn
	sel := t.sel
	t.mu.Unlock()
	if !on {
		return ""
	}
	return selectionText(snap, sel)
}

// Close ends the PTY and the shell process.
// It returns after the read and wait goroutines have finished.
func (t *Term) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	f := t.ptmx
	cmd := t.cmd
	t.ptmx = nil
	t.mu.Unlock()
	var err error
	if f != nil {
		err = f.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	t.wg.Wait()
	return err
}

// ID is the bus session id.
func (t *Term) ID() uint64 { return t.id }

func shellEnv(base []string) []string {
	out := make([]string, 0, len(base)+2)
	for _, e := range base {
		if strings.HasPrefix(e, "TERM=") || strings.HasPrefix(e, "COLORTERM=") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "TERM=xterm-256color", "COLORTERM=truecolor")
}

func selectionText(snap ScreenSnapshot, sel [4]int) string {
	x0, y0, x1, y1 := sel[0], sel[1], sel[2], sel[3]
	if y0 > y1 || (y0 == y1 && x0 > x1) {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	var b strings.Builder
	for y := y0; y <= y1; y++ {
		if y < 0 || y >= snap.Rows {
			continue
		}
		if y > y0 {
			b.WriteByte('\n')
		}
		from, to := 0, snap.Cols
		if y == y0 {
			from = x0
		}
		if y == y1 {
			to = x1 + 1
		}
		if from < 0 {
			from = 0
		}
		if to > snap.Cols {
			to = snap.Cols
		}
		for x := from; x < to; x++ {
			c := snap.Cells[y*snap.Cols+x]
			if c.W < 0 {
				continue
			}
			if c.R == 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteRune(c.R)
		}
	}
	return strings.TrimRight(b.String(), " \n")
}

// Resize changes the grid, keeping the overlapping cells and resetting the scroll region.
func (sc *Screen) Resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if cols == sc.cols && rows == sc.rows {
		return
	}
	sc.primary = resizeGrid(sc.primary, sc.cols, sc.rows, cols, rows)
	sc.alt = resizeGrid(sc.alt, sc.cols, sc.rows, cols, rows)
	sc.cols, sc.rows = cols, rows
	sc.scrollTop = 0
	sc.scrollBot = rows - 1
	if sc.curY >= rows {
		sc.curY = rows - 1
	}
	if sc.curX >= cols {
		sc.curX = cols
	}
}

func resizeGrid(old []Cell, oc, or, nc, nr int) []Cell {
	next := make([]Cell, nc*nr)
	y := min(or, nr)
	x := min(oc, nc)
	for row := range y {
		copy(next[row*nc:row*nc+x], old[row*oc:row*oc+x])
	}
	return next
}

// DebugUnknown is the count of ignored sequences.
func (t *Term) DebugUnknown() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scr.unknown
}

// String is used in error wrapping.
func (t *Term) String() string { return fmt.Sprintf("terminal %d", t.id) }
