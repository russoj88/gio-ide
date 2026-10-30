package runtimemem

import (
	"runtime/debug"
	"testing"
	"time"
)

func TestApplyRespectsEnvAndOff(t *testing.T) {
	prev := CurrentLimit()
	t.Cleanup(func() { debug.SetMemoryLimit(prev) })

	t.Setenv("GOMEMLIMIT", "1GiB")
	limit, applied, err := Apply("auto")
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("GOMEMLIMIT should win")
	}
	if limit == 0 {
		t.Fatal("limit")
	}

	t.Setenv("GOMEMLIMIT", "")
	limit, applied, err = Apply("off")
	if err != nil || applied {
		t.Fatalf("off: %d %v %v", limit, applied, err)
	}

	limit, applied, err = Apply("64MiB")
	if err != nil || !applied {
		t.Fatal(err, applied)
	}
	if limit != 64*(1<<20) {
		t.Fatalf("limit %d", limit)
	}
	if CurrentLimit() != limit {
		t.Fatal(CurrentLimit())
	}
}

func TestReadAndSampler(t *testing.T) {
	s := Read()
	if s.Heap == 0 || s.RSS == 0 {
		t.Fatalf("%+v", s)
	}
	if PhysicalRAM() < 1<<20 {
		t.Fatal(PhysicalRAM())
	}
	stop := make(chan struct{})
	got := make(chan Sample, 1)
	Start(10*time.Millisecond, stop, func(s Sample) {
		select {
		case got <- s:
		default:
		}
	})
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("sampler")
	}
	close(stop)
}
