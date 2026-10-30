//go:build debug

package app

import "time"

func noteSlowFrame(d time.Duration) { dumpSlowFrame(d) }
