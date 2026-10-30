// The Unlicense covers this file. See LICENSE at the repository root.

// Package guard starts goroutines that recover a panic, log the stack,
// and call the hook the intel host registers.
package guard

import (
	"log"
	"runtime/debug"
	"sync"
)

// Hook, when set, is called after a recovered panic is logged.
// The intel host sets it so a gopls panic marks the session degraded.
var Hook func(recovered any, stack []byte)

var hookMu sync.Mutex

// Go runs fn in a new goroutine. A panic is recovered, logged, and reported.
func Go(fn func()) {
	go func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			stack := debug.Stack()
			log.Printf("gopls guard: panic: %v\n%s", r, stack)
			hookMu.Lock()
			h := Hook
			hookMu.Unlock()
			if h == nil {
				return
			}
			defer func() { _ = recover() }()
			h(r, stack)
		}()
		fn()
	}()
}
