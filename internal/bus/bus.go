// Package bus is the in-process event bus. Workers post events. The UI drains them.
package bus

import (
	"fmt"
	"sync"
)

// Bus delivers events to worker subscribers and to the UI drain.
// Post does not block for long. Mergeable events keep only the newest value.
type Bus interface {
	Post(e Event)
	Subscribe(fn func(Event)) (cancel func())
	Drain(fn func(Event))
	SetInvalidate(fn func())
}

type sub struct {
	id int
	fn func(Event)
}

type job struct {
	fns  []func(Event)
	e    Event
	wake bool
}

type impl struct {
	mu         sync.Mutex
	ui         []Event
	merged     map[string]int
	subs       []sub
	nextID     int
	subq       chan job
	pending    []Event
	invalidate func()
}

// New returns a bus. invalidate may be nil and can be set later.
// It is called after each post and must be safe for concurrent use
// (Gio's Window.Invalidate is).
func New(invalidate func()) Bus {
	b := &impl{
		merged:     map[string]int{},
		subq:       make(chan job, 1024),
		invalidate: invalidate,
	}
	go b.subLoop()
	return b
}

func (b *impl) SetInvalidate(fn func()) {
	b.mu.Lock()
	b.invalidate = fn
	b.mu.Unlock()
}

func (b *impl) Post(e Event) {
	b.mu.Lock()
	b.enqueueLocked(e)
	fns := make([]func(Event), 0, len(b.subs))
	for _, s := range b.subs {
		fns = append(fns, s.fn)
	}
	inv := b.invalidate
	b.mu.Unlock()
	select {
	case b.subq <- job{fns: fns, e: e}:
	default:
		b.mu.Lock()
		b.pending = append(b.pending, e)
		b.mu.Unlock()
		select {
		case b.subq <- job{wake: true}:
		default:
		}
	}
	if inv != nil {
		inv()
	}
}

func (b *impl) Subscribe(fn func(Event)) (cancel func()) {
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.subs = append(b.subs, sub{id: id, fn: fn})
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		out := b.subs[:0]
		for _, s := range b.subs {
			if s.id != id {
				out = append(out, s)
			}
		}
		b.subs = out
	}
}

func (b *impl) subLoop() {
	for j := range b.subq {
		if !j.wake {
			for _, fn := range j.fns {
				if fn != nil {
					fn(j.e)
				}
			}
		}
		b.flushPending()
	}
}

func (b *impl) flushPending() {
	for {
		b.mu.Lock()
		if len(b.pending) == 0 {
			b.mu.Unlock()
			return
		}
		e := b.pending[0]
		b.pending = b.pending[1:]
		fns := make([]func(Event), 0, len(b.subs))
		for _, s := range b.subs {
			fns = append(fns, s.fn)
		}
		b.mu.Unlock()
		for _, fn := range fns {
			if fn != nil {
				fn(e)
			}
		}
	}
}

func (b *impl) Drain(fn func(Event)) {
	b.mu.Lock()
	evs := b.ui
	b.ui = nil
	b.merged = map[string]int{}
	b.mu.Unlock()
	for _, e := range evs {
		fn(e)
	}
}

func (b *impl) enqueueLocked(e Event) {
	if k, ok := mergeKey(e); ok {
		if i, exists := b.merged[k]; exists && i < len(b.ui) {
			b.ui[i] = e
			return
		}
		b.merged[k] = len(b.ui)
	}
	b.ui = append(b.ui, e)
}

func mergeKey(e Event) (string, bool) {
	switch ev := e.(type) {
	case DiagnosticsUpdated:
		return "diag:" + ev.URI, true
	case SyntaxSpans:
		return fmt.Sprintf("syn:%d", ev.DocID), true
	case FoldRanges:
		return fmt.Sprintf("fold:%d", ev.DocID), true
	case GitStatusUpdated:
		return "gitstatus", true
	case TerminalDirty:
		return fmt.Sprintf("term:%d", ev.Session), true
	case MemorySample:
		return "mem", true
	default:
		return "", false
	}
}
