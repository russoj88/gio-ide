package intel

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
)

func testHost(t *testing.T) (*host, bus.Bus) {
	t.Helper()
	b := bus.New(nil)
	to := DefaultTimeouts()
	to.Other = 45 * time.Second
	to.Completion = 20 * time.Second
	to.Definition = 20 * time.Second
	to.References = 20 * time.Second
	h := New(Config{
		Post:     b.Post,
		Timeouts: to,
		Gopls:    map[string]any{"staticcheck": false},
		Logf:     func(s string) { t.Log(s) },
	}).(*host)
	t.Cleanup(func() {
		if h.State() == Stopped {
			return
		}
		done := make(chan struct{})
		h.enqueue(job{kind: "shutdown", run: func(ctx context.Context) error {
			err := h.shutdown(ctx)
			close(done)
			return err
		}})
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
	})
	return h, b
}

func waitState(t *testing.T, h *host, b bus.Bus, want State, d time.Duration) {
	t.Helper()
	if h.State() == want {
		return
	}
	ch := make(chan struct{}, 8)
	cancel := b.Subscribe(func(e bus.Event) {
		if _, ok := e.(bus.IntelStateChanged); !ok {
			return
		}
		select {
		case ch <- struct{}{}:
		default:
		}
	})
	defer cancel()
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		if h.State() == want {
			return
		}
		select {
		case <-ch:
		case <-timer.C:
			if h.State() == want {
				return
			}
			t.Fatalf("state = %s, want %s (%s)", h.State(), want, h.detail)
		}
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func waitEvent(t *testing.T, b bus.Bus, d time.Duration, match func(bus.Event) bool) bus.Event {
	t.Helper()
	got := make(chan bus.Event, 8)
	cancel := b.Subscribe(func(e bus.Event) {
		if match(e) {
			select {
			case got <- e:
			default:
			}
		}
	})
	defer cancel()
	select {
	case e := <-got:
		return e
	case <-time.After(d):
		t.Fatal("timed out waiting for event")
		return nil
	}
}

func TestJobPanicDegradesAndRestart(t *testing.T) {
	h, b := testHost(t)
	h.Start(fixtureRoot(t))
	waitState(t, h, b, Ready, 40*time.Second)

	ran := make(chan struct{})
	h.enqueue(job{kind: "boom", run: func(context.Context) error { panic("boom") }})
	h.enqueue(job{kind: "after", run: func(context.Context) error { close(ran); return nil }})
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("sequencer did not continue after a panic")
	}
	if h.State() != Degraded {
		t.Fatalf("state = %s, want Degraded", h.State())
	}
	h.Restart()
	waitState(t, h, b, Ready, 40*time.Second)
}

func TestJobTimeoutThenNext(t *testing.T) {
	h, b := testHost(t)
	got := make(chan struct{}, 1)
	cancel := b.Subscribe(func(e bus.Event) {
		n, ok := e.(bus.Notification)
		if ok && strings.Contains(n.Text, "deadline") {
			select {
			case got <- struct{}{}:
			default:
			}
		}
	})
	defer cancel()
	h.enqueue(job{
		kind:    "slow",
		timeout: 40 * time.Millisecond,
		run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	next := make(chan struct{})
	h.enqueue(job{kind: "next", run: func(context.Context) error { close(next); return nil }})
	select {
	case <-next:
	case <-time.After(3 * time.Second):
		t.Fatal("next job did not run")
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("missing timeout notification")
	}
}

func TestDidChangeVersionOrder(t *testing.T) {
	h, b := testHost(t)
	root := fixtureRoot(t)
	h.Start(root)
	waitState(t, h, b, Ready, 40*time.Second)
	path := filepath.Join(root, "fixture.go")
	doc, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h.DidOpen(doc)
	h.enqueue(job{kind: "barrier", run: func(context.Context) error { return nil }})

	var mu sync.Mutex
	var got []int32
	h.mu.Lock()
	h.observe = func(kind string, version int32) {
		if kind != "change" {
			return
		}
		mu.Lock()
		got = append(got, version)
		mu.Unlock()
	}
	h.mu.Unlock()
	text := doc.Snapshot().Bytes()
	for i := 1; i <= 1000; i++ {
		h.DidChange(doc.ID(), int32(i), []buffer.Edit{{
			Start: 0, End: buffer.Offset(len(text)), Text: string(text),
		}}, doc.Snapshot())
	}
	done := make(chan struct{})
	h.enqueue(job{kind: "barrier", run: func(context.Context) error { close(done); return nil }})
	select {
	case <-done:
	case <-time.After(45 * time.Second):
		t.Fatal("changes did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1000 {
		t.Fatalf("ran %d change jobs", len(got))
	}
	for i, v := range got {
		if v != int32(i+1) {
			t.Fatalf("job %d has version %d", i, v)
		}
	}
}

func TestFixtureDiagnosticsCompletionDefinition(t *testing.T) {
	h, b := testHost(t)
	root := fixtureRoot(t)
	h.Start(root)
	waitState(t, h, b, Ready, 40*time.Second)
	path := filepath.Join(root, "fixture.go")
	doc, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h.DidOpen(doc)
	opened := make(chan struct{})
	h.enqueue(job{kind: "barrier", run: func(context.Context) error {
		close(opened)
		return nil
	}})
	select {
	case <-opened:
	case <-time.After(30 * time.Second):
		t.Fatal("open did not finish")
	}

	// Completion on the valid file, at the method name after `s.`.
	cur := string(doc.Snapshot().Bytes())
	dot := strings.LastIndex(cur, "s.")
	if dot < 0 {
		t.Fatal("missing selector")
	}
	h.Completion(doc.ID(), doc.Version(), buffer.Offset(dot+len("s.")))
	raw := waitEvent(t, b, 30*time.Second, func(e bus.Event) bool {
		_, ok := e.(bus.CompletionReady)
		return ok
	}).(bus.CompletionReady)
	if raw.Err != "" {
		t.Fatalf("completion error: %s", raw.Err)
	}
	comp := raw
	found := false
	for _, it := range comp.Items {
		if it.Label == "Listen" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Listen missing from %#v", comp.Items)
	}

	// Definition of the call's method name.
	call := strings.LastIndex(cur, "Listen")
	h.Definition(doc.ID(), doc.Version(), buffer.Offset(call))
	def := waitEvent(t, b, 20*time.Second, func(e bus.Event) bool {
		l, ok := e.(bus.LocationsReady)
		return ok && l.Kind == "definition" && l.Err == "" && len(l.Locations) > 0
	}).(bus.LocationsReady)
	if !strings.HasSuffix(def.Locations[0].Path, "fixture.go") {
		t.Fatalf("definition path %s", def.Locations[0].Path)
	}
	if def.Locations[0].Line < 1 {
		t.Fatalf("definition line %d", def.Locations[0].Line)
	}

	h.References(doc.ID(), doc.Version(), buffer.Offset(call))
	refs := waitEvent(t, b, 20*time.Second, func(e bus.Event) bool {
		l, ok := e.(bus.LocationsReady)
		return ok && l.Kind == "references" && l.Err == ""
	}).(bus.LocationsReady)
	if len(refs.Locations) < 2 {
		t.Fatalf("references = %d, want at least 2", len(refs.Locations))
	}

	old := `"localhost"`
	at := strings.Index(cur, old)
	if at < 0 {
		t.Fatal("fixture missing localhost")
	}
	snap, err := doc.Apply([]buffer.Edit{{
		Start: buffer.Offset(at), End: buffer.Offset(at + len(old)), Text: "1",
	}}, buffer.OriginCommand)
	if err != nil {
		t.Fatal(err)
	}
	h.DidChange(doc.ID(), doc.Version(), []buffer.Edit{{
		Start: buffer.Offset(at), End: buffer.Offset(at + len(old)), Text: "1",
	}}, snap)
	diag := waitEvent(t, b, 40*time.Second, func(e bus.Event) bool {
		d, ok := e.(bus.DiagnosticsUpdated)
		if !ok {
			return false
		}
		for _, it := range d.Items {
			if it.Severity == 1 && (strings.Contains(it.Message, "cannot") || strings.Contains(strings.ToLower(it.Message), "int")) {
				return true
			}
		}
		return false
	})
	if diag == nil {
		t.Fatal("no type-error diagnostic")
	}
}

func TestRestartReleasesGoroutines(t *testing.T) {
	h, b := testHost(t)
	root := fixtureRoot(t)
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	g0 := runtime.NumGoroutine()
	h.Start(root)
	waitState(t, h, b, Ready, 40*time.Second)
	path := filepath.Join(root, "fixture.go")
	doc, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h.DidOpen(doc)
	opened := make(chan struct{})
	h.enqueue(job{kind: "barrier", run: func(context.Context) error {
		close(opened)
		return nil
	}})
	select {
	case <-opened:
	case <-time.After(30 * time.Second):
		t.Fatal("open did not finish")
	}
	h.Restart()
	waitState(t, h, b, Ready, 40*time.Second)
	h.enqueue(job{kind: "shutdown", run: func(ctx context.Context) error { return h.shutdown(ctx) }})
	waitState(t, h, b, Stopped, 15*time.Second)
	select {
	case <-h.exited:
	case <-time.After(15 * time.Second):
		t.Fatal("sequencer did not exit")
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	g1 := runtime.NumGoroutine()
	t.Logf("goroutines before=%d after=%d delta=%d heap before=%d after=%d",
		g0, g1, g1-g0, before.HeapAlloc, after.HeapAlloc)
	if g1-g0 > 40 {
		t.Fatalf("goroutines grew by %d (before %d after %d); restart did not release them", g1-g0, g0, g1)
	}
}
