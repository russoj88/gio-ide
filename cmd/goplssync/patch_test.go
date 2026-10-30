package main

import (
	"strings"
	"testing"
)

func TestRewriteGoStmtsNested(t *testing.T) {
	src := []byte(`package p

import "fmt"

func f() {
	go fmt.Println(1)
	go func() {
		go fmt.Println(2)
	}()
}
`)
	out, n, err := rewriteGoStmts(src, "github.com/russoj88/gio-ide/internal/gopls/guard")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("rewrote %d, want 3", n)
	}
	s := string(out)
	if strings.Contains(s, "\tgo ") || strings.Contains(s, "\n\tgo ") {
		t.Fatalf("unguarded go remains:\n%s", s)
	}
	if strings.Count(s, "guard.Go(") != 3 {
		t.Fatalf("guard count:\n%s", s)
	}
	if !strings.Contains(s, `"github.com/russoj88/gio-ide/internal/gopls/guard"`) {
		t.Fatalf("import missing:\n%s", s)
	}
}

func TestNeutralizeFatals(t *testing.T) {
	src := []byte(`package p

import "log"

func f() {
	log.Fatalf("bad %d", 1)
}
`)
	out, changed, err := neutralizeFatals(src)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	s := string(out)
	if strings.Contains(s, "log.Fatal") {
		t.Fatalf("fatal remains:\n%s", s)
	}
	if strings.Contains(s, `"log"`) {
		t.Fatalf("unused log import:\n%s", s)
	}
	if !strings.Contains(s, "panic(fmt.Sprintf(") {
		t.Fatalf("missing panic:\n%s", s)
	}
}
