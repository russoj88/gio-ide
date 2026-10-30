package keymap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultAndSample(t *testing.T) {
	m := Default()
	ctx := Context{EditorFocus: true}
	cmd, prefix := m.Lookup(mustSeq(t, "ctrl+shift+n"), ctx)
	if cmd != "nav.goToFile" || prefix {
		t.Fatalf("go to file: %q %v", cmd, prefix)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+y"), ctx)
	if cmd != "edit.deleteLine" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+t"), ctx)
	if cmd != "terminal.new" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+alt+n"), ctx)
	if cmd != "file.newScratch" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+t"), Context{TerminalFocus: true})
	if cmd != "terminal.new" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+d"), ctx)
	if cmd != "edit.duplicateLine" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+shift+`"), ctx)
	if cmd != "git.branches" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+-"), ctx)
	if cmd != "edit.fold" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+="), ctx)
	if cmd != "edit.unfold" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+shift+-"), ctx)
	if cmd != "edit.foldAll" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+shift+="), ctx)
	if cmd != "edit.unfoldAll" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+alt+h"), ctx)
	if cmd != "edit.highlight" {
		t.Fatal(cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+alt+shift+h"), ctx)
	if cmd != "edit.highlightLines" {
		t.Fatal(cmd)
	}
	// Unbound.
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+alt+l"), ctx)
	if cmd != "" {
		t.Fatal(cmd)
	}

	sample := `
[[bind]]
keys = "ctrl+shift+n"
command = "nav.goToFile"

[[bind]]
keys = "alt+g b"
command = "git.branches"
when = "!terminalFocus"

[[unbind]]
keys = "ctrl+y"
`
	loaded, err := Parse(sample, m)
	if err != nil {
		t.Fatal(err)
	}
	cmd, _ = loaded.Lookup(mustSeq(t, "ctrl+y"), ctx)
	if cmd != "" {
		t.Fatalf("unbind failed: %q", cmd)
	}
	cmd, prefix = loaded.Lookup(mustSeq(t, "alt+g"), ctx)
	if cmd != "" || !prefix {
		t.Fatalf("chord prefix: %q %v", cmd, prefix)
	}
	cmd, prefix = loaded.Lookup(mustSeq(t, "alt+g b"), ctx)
	if cmd != "git.branches" || prefix {
		t.Fatalf("chord: %q %v", cmd, prefix)
	}
	term := Context{TerminalFocus: true}
	cmd, _ = loaded.Lookup(mustSeq(t, "alt+g b"), term)
	if cmd != "" {
		t.Fatalf("when should reject terminal focus, got %q", cmd)
	}

	ok, err := Eval("editorFocus && !popupOpen", Context{EditorFocus: true})
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	ok, err = Eval("!hasSelection", Context{HasSelection: true})
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	if _, err := Eval("nope", Context{}); err == nil {
		t.Fatal("expected error")
	}

	r := Resolver{Map: loaded}
	now := time.Unix(0, 0)
	c, more := r.Push(mustSeq(t, "alt+g")[0], ctx, now)
	if c != "" || !more {
		t.Fatalf("resolver prefix %q %v", c, more)
	}
	c, more = r.Push(mustSeq(t, "b")[0], ctx, now.Add(time.Second))
	if c != "git.branches" || more {
		t.Fatalf("resolver chord %q %v", c, more)
	}
	// Timeout drops the pending stroke.
	c, more = r.Push(mustSeq(t, "alt+g")[0], ctx, now)
	if !more {
		t.Fatal("expected more")
	}
	c, more = r.Push(mustSeq(t, "b")[0], ctx, now.Add(2*time.Second))
	if c != "" || more {
		t.Fatalf("timeout should drop chord, got %q %v", c, more)
	}
}

func TestParseStroke(t *testing.T) {
	s, err := ParseStroke("ctrl+alt+shift+f12")
	if err != nil {
		t.Fatal(err)
	}
	if s.Key != "f12" || s.Mod != ModCtrl|ModAlt|ModShift {
		t.Fatal(s)
	}
	if _, err := ParseStroke("ctrl+foo"); err == nil {
		t.Fatal("expected error")
	}
	if !TerminalPassthrough(Stroke{Mod: ModAlt, Key: "f12"}) {
		t.Fatal("passthrough")
	}
	if TerminalPassthrough(Stroke{Mod: ModCtrl, Key: "c"}) {
		t.Fatal("ctrl+c goes to the pty")
	}
	if !TerminalPassthrough(Stroke{Mod: ModCtrl, Key: "t"}) {
		t.Fatal("ctrl+t")
	}
	if TerminalPassthrough(Stroke{Mod: ModCtrl, Key: "d"}) {
		t.Fatal("ctrl+d goes to the pty")
	}
}

func mustSeq(t *testing.T, s string) []Stroke {
	t.Helper()
	seq, err := ParseSequence(s)
	if err != nil {
		t.Fatal(err)
	}
	return seq
}

func TestSetBinding(t *testing.T) {
	ctx := Context{}
	m := Default()
	displaced := SetBinding(m, mustSeq(t, "ctrl+s"), "file.saveAll")
	if displaced != "file.save" {
		t.Fatalf("displaced %q", displaced)
	}
	cmd, _ := m.Lookup(mustSeq(t, "ctrl+s"), ctx)
	if cmd != "file.saveAll" {
		t.Fatalf("lookup %q", cmd)
	}
	for _, b := range m.Bindings() {
		if b.Command == "file.save" {
			t.Fatalf("file.save still bound to %s", seqText(b.Keys))
		}
	}

	m = Default()
	seq := mustSeq(t, "ctrl+alt+l")
	if displaced := SetBinding(m, seq, "file.save"); displaced != "" {
		t.Fatalf("displaced %q", displaced)
	}
	seq[0].Key = "nope"
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+alt+l"), ctx)
	if cmd != "file.save" {
		t.Fatalf("unbound stroke %q", cmd)
	}
	cmd, _ = m.Lookup(mustSeq(t, "ctrl+s"), ctx)
	if cmd != "" {
		t.Fatalf("file.save kept ctrl+s as %q", cmd)
	}

	before := Overrides(m)
	n := len(m.Bindings())
	if SetBinding(m, nil, "file.save") != "" || SetBinding(m, mustSeq(t, "ctrl+z"), "") != "" {
		t.Fatal("empty binding returned a command")
	}
	if Overrides(m) != before || len(m.Bindings()) != n {
		t.Fatal("empty SetBinding mutated the map")
	}
	if SetBinding(nil, mustSeq(t, "ctrl+s"), "file.save") != "" {
		t.Fatal("nil map")
	}
}

func TestResetCommand(t *testing.T) {
	ctx := Context{}
	m := Default()
	SetBinding(m, mustSeq(t, "ctrl+s"), "file.saveAll")
	ResetCommand(m, "file.save")
	cmd, _ := m.Lookup(mustSeq(t, "ctrl+s"), ctx)
	if cmd != "file.save" {
		t.Fatalf("restored %q", cmd)
	}
	for _, b := range m.Bindings() {
		if sameSeq(b.Keys, mustSeq(t, "ctrl+s")) && b.Command != "file.save" {
			t.Fatalf("previous owner %s kept ctrl+s", b.Command)
		}
	}

	loaded, err := Parse(`
[[bind]]
keys = "ctrl+s"
command = "file.saveAll"

[[bind]]
keys = "ctrl+alt+l"
command = "file.saveAll"
`, Default())
	if err != nil {
		t.Fatal(err)
	}
	ResetCommand(loaded, "file.save")
	cmd, _ = loaded.Lookup(mustSeq(t, "ctrl+s"), ctx)
	if cmd != "file.save" {
		t.Fatalf("taken back %q", cmd)
	}
	cmd, _ = loaded.Lookup(mustSeq(t, "ctrl+alt+l"), ctx)
	if cmd != "file.saveAll" {
		t.Fatalf("other sequence %q", cmd)
	}
	ResetCommand(nil, "file.save")
}

func TestOverrides(t *testing.T) {
	if Overrides(Default()) != "" || Overrides(nil) != "" {
		t.Fatal("unchanged map should have an empty diff")
	}
	m := Default()
	SetBinding(m, mustSeq(t, "ctrl+s"), "file.saveAll")
	diff := Overrides(m)
	unbind := strings.Index(diff, "[[unbind]]")
	bind := strings.Index(diff, "[[bind]]")
	if unbind < 0 || bind < 0 || unbind > bind {
		t.Fatalf("unbind then bind:\n%s", diff)
	}
	if !strings.Contains(diff, "keys = \"ctrl+s\"") || !strings.Contains(diff, "command = \"file.saveAll\"") {
		t.Fatalf("diff:\n%s", diff)
	}
	if strings.Contains(diff, "when =") {
		t.Fatalf("empty when should be omitted:\n%s", diff)
	}
	again, err := Parse(diff, Default())
	if err != nil {
		t.Fatal(err)
	}
	assertSameLookup(t, m, again)
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "keymap.toml")
	m := Default()
	SetBinding(m, mustSeq(t, "ctrl+s"), "file.saveAll")
	if err := Save(path, m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != Overrides(m) {
		t.Fatalf("file %q", body)
	}
	loaded, err := Load(path, Default())
	if err != nil {
		t.Fatal(err)
	}
	assertSameLookup(t, m, loaded)
	cmd, _ := loaded.Lookup(mustSeq(t, "ctrl+s"), Context{})
	if cmd != "file.saveAll" {
		t.Fatalf("loaded %q", cmd)
	}

	ResetCommand(m, "file.save")
	if Overrides(m) != "" {
		t.Fatalf("reset diff:\n%s", Overrides(m))
	}
	if err := Save(path, m); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Fatalf("empty diff wrote %q", body)
	}
	loaded, err = Load(path, Default())
	if err != nil {
		t.Fatal(err)
	}
	cmd, _ = loaded.Lookup(mustSeq(t, "ctrl+s"), Context{})
	if cmd != "file.save" {
		t.Fatalf("restored load %q", cmd)
	}
	assertSameLookup(t, m, loaded)
	assertSameLookup(t, Default(), loaded)

	nilPath := filepath.Join(dir, "nil.toml")
	if err := Save(nilPath, nil); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(nilPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Fatalf("nil map wrote %q", body)
	}
}

func TestOverridesWhen(t *testing.T) {
	m, err := Parse(`
[[bind]]
keys = "alt+g b"
command = "git.branches"
when = "!terminalFocus"
`, Default())
	if err != nil {
		t.Fatal(err)
	}
	diff := Overrides(m)
	if !strings.Contains(diff, "[[bind]]") || !strings.Contains(diff, "keys = \"alt+g b\"") || !strings.Contains(diff, "when = \"!terminalFocus\"") {
		t.Fatalf("diff:\n%s", diff)
	}
	again, err := Parse(diff, Default())
	if err != nil {
		t.Fatal(err)
	}
	assertSameLookup(t, m, again)
	cmd, _ := again.Lookup(mustSeq(t, "alt+g b"), Context{})
	if cmd != "git.branches" {
		t.Fatalf("chord %q", cmd)
	}
	cmd, _ = again.Lookup(mustSeq(t, "alt+g b"), Context{TerminalFocus: true})
	if cmd != "" {
		t.Fatalf("when rejected %q", cmd)
	}
	cmd, _ = again.Lookup(mustSeq(t, "ctrl+shift+`"), Context{})
	if cmd != "git.branches" {
		t.Fatalf("default chord owner %q", cmd)
	}

	moved, err := Parse(`
[[bind]]
keys = "ctrl+s"
command = "file.save"
when = "!terminalFocus"
`, Default())
	if err != nil {
		t.Fatal(err)
	}
	diff = Overrides(moved)
	unbind := strings.Index(diff, "[[unbind]]")
	bind := strings.Index(diff, "[[bind]]")
	if unbind < 0 || bind < 0 || unbind > bind || !strings.Contains(diff, "when = \"!terminalFocus\"") {
		t.Fatalf("unbind+bind when:\n%s", diff)
	}
	again, err = Parse(diff, Default())
	if err != nil {
		t.Fatal(err)
	}
	assertSameLookup(t, moved, again)
	cmd, _ = again.Lookup(mustSeq(t, "ctrl+s"), Context{})
	if cmd != "file.save" {
		t.Fatalf("when round trip %q", cmd)
	}
	cmd, _ = again.Lookup(mustSeq(t, "ctrl+s"), Context{TerminalFocus: true})
	if cmd != "" {
		t.Fatalf("when should hide ctrl+s, got %q", cmd)
	}
}

func assertSameLookup(t *testing.T, a, b *Map) {
	t.Helper()
	if len(a.Bindings()) != len(b.Bindings()) {
		t.Fatalf("binding count %d != %d", len(a.Bindings()), len(b.Bindings()))
	}
	seqs := map[string][]Stroke{}
	collect := func(m *Map) {
		for _, bind := range m.Bindings() {
			seqs[seqText(bind.Keys)] = bind.Keys
		}
	}
	collect(a)
	collect(b)
	contexts := []Context{{}, {TerminalFocus: true}, {EditorFocus: true}}
	for name, seq := range seqs {
		for _, ctx := range contexts {
			ca, pa := a.Lookup(seq, ctx)
			cb, pb := b.Lookup(seq, ctx)
			if ca != cb || pa != pb {
				t.Fatalf("%s ctx %+v: %q/%v != %q/%v", name, ctx, ca, pa, cb, pb)
			}
		}
	}
}
