// Package runtimemem sets the process memory limit and samples heap and RSS.
package runtimemem

import (
	"bufio"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"

	"github.com/russoj88/gio-ide/internal/settings"
)

const floor int64 = 2 << 30 // 2 GiB

// Apply sets the soft memory limit from a [runtime] memory_limit value.
// If GOMEMLIMIT is already set, the environment wins and Apply does not
// call SetMemoryLimit. "off" leaves the limit unchanged.
// The returned limit is the active limit after the call.
func Apply(spec string) (limit int64, applied bool, err error) {
	if os.Getenv("GOMEMLIMIT") != "" {
		return debug.SetMemoryLimit(-1), false, nil
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "auto"
	}
	if strings.EqualFold(spec, "off") {
		return debug.SetMemoryLimit(-1), false, nil
	}
	var n int64
	if strings.EqualFold(spec, "auto") {
		ram := PhysicalRAM()
		n = max(ram/2, floor)
	} else {
		n, err = settings.ParseMemoryLimit(spec)
		if err != nil {
			return debug.SetMemoryLimit(-1), false, err
		}
		if n <= 0 {
			return debug.SetMemoryLimit(-1), false, nil
		}
	}
	debug.SetMemoryLimit(n)
	return n, true, nil
}

// CurrentLimit returns the active soft limit.
func CurrentLimit() int64 { return debug.SetMemoryLimit(-1) }

// Sample reads the Go heap and the process resident set.
type Sample struct {
	Heap  uint64
	RSS   uint64
	Limit int64
	Ratio float64
}

func Read() Sample {
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(samples)
	heap := samples[0].Value.Uint64()
	rss := RSS()
	limit := CurrentLimit()
	var ratio float64
	if limit > 0 && limit < int64(^uint64(0)>>1) {
		ratio = float64(heap) / float64(limit)
	}
	return Sample{Heap: heap, RSS: rss, Limit: limit, Ratio: ratio}
}

// RSS returns the resident set size in bytes from /proc/self/statm.
func RSS() uint64 {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.Sys
	}
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(os.Getpagesize())
}

// PhysicalRAM returns MemTotal from /proc/meminfo, in bytes.
func PhysicalRAM() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return floor * 2
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			break
		}
		return kb * 1024
	}
	return floor * 2
}

// Start samples every interval until stop is closed. onSample runs on the sampler goroutine.
func Start(interval time.Duration, stop <-chan struct{}, onSample func(Sample)) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if onSample != nil {
					onSample(Read())
				}
			}
		}
	}()
}
