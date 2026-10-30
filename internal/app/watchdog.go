package app

import (
	"log"
	"os"
	"runtime"
	"time"
)

func dumpSlowFrame(d time.Duration) {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	log.Printf("gio-ide: frame took %s (watchdog)\n%s", d, buf[:n])
}

func noteSlowFrameRelease(d time.Duration) {
	if os.Getenv("GIO_IDE_WATCHDOG") == "1" {
		dumpSlowFrame(d)
	}
}
