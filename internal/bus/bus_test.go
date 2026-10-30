package bus

import (
	"testing"
	"time"
)

func TestMergeKeepsNewest(t *testing.T) {
	b := New(nil)
	b.Post(DiagnosticsUpdated{URI: "file:///a.go", Version: 1, Items: []Diagnostic{{Message: "old"}}})
	b.Post(DiagnosticsUpdated{URI: "file:///a.go", Version: 2, Items: []Diagnostic{{Message: "new"}}})
	b.Post(DiagnosticsUpdated{URI: "file:///b.go", Version: 1, Items: []Diagnostic{{Message: "b"}}})
	b.Post(SyntaxSpans{DocID: 7, Version: 1})
	b.Post(SyntaxSpans{DocID: 7, Version: 4})
	b.Post(Notification{Text: "one"})
	b.Post(Notification{Text: "two"})
	b.Post(MemorySample{Heap: 1})
	b.Post(MemorySample{Heap: 9})
	b.Post(TerminalDirty{Session: 3})
	b.Post(TerminalDirty{Session: 3})
	b.Post(GitStatusUpdated{Branch: "main"})
	b.Post(GitStatusUpdated{Branch: "dev"})

	var got []Event
	b.Drain(func(e Event) { got = append(got, e) })
	if len(got) != 8 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	d := got[0].(DiagnosticsUpdated)
	if d.Version != 2 || d.Items[0].Message != "new" {
		t.Fatalf("diag a: %#v", d)
	}
	if got[1].(DiagnosticsUpdated).URI != "file:///b.go" {
		t.Fatal(got[1])
	}
	if got[2].(SyntaxSpans).Version != 4 {
		t.Fatal(got[2])
	}
	if got[3].(Notification).Text != "one" || got[4].(Notification).Text != "two" {
		t.Fatalf("notes: %#v %#v", got[3], got[4])
	}
	if got[5].(MemorySample).Heap != 9 {
		t.Fatal(got[5])
	}
	if got[6].(TerminalDirty).Session != 3 {
		t.Fatal(got[6])
	}
	if got[7].(GitStatusUpdated).Branch != "dev" {
		t.Fatal(got[7])
	}
}

func TestSubscribeAndCancel(t *testing.T) {
	b := New(nil)
	got := make(chan string, 4)
	cancel := b.Subscribe(func(e Event) {
		if n, ok := e.(Notification); ok {
			got <- n.Text
		}
	})
	b.Post(Notification{Text: "a"})
	select {
	case s := <-got:
		if s != "a" {
			t.Fatal(s)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	cancel()
	b.Post(Notification{Text: "b"})
	select {
	case s := <-got:
		t.Fatalf("cancelled subscriber received %s", s)
	case <-time.After(50 * time.Millisecond):
	}
}
