// Package keymap loads the default GoLand-like keymap and user overrides.
package keymap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

// Mod is a set of modifier keys.
type Mod uint8

const (
	ModCtrl Mod = 1 << iota
	ModAlt
	ModShift
	ModSuper
)

func (m Mod) String() string {
	var parts []string
	if m&ModCtrl != 0 {
		parts = append(parts, "ctrl")
	}
	if m&ModAlt != 0 {
		parts = append(parts, "alt")
	}
	if m&ModShift != 0 {
		parts = append(parts, "shift")
	}
	if m&ModSuper != 0 {
		parts = append(parts, "super")
	}
	return strings.Join(parts, "+")
}

// Stroke is one key press.
type Stroke struct {
	Mod Mod
	Key string
}

func (s Stroke) String() string {
	if s.Mod == 0 {
		return s.Key
	}
	return s.Mod.String() + "+" + s.Key
}

// Context is the when-expression environment.
type Context struct {
	EditorFocus   bool
	TerminalFocus bool
	TreeFocus     bool
	PopupOpen     bool
	HasSelection  bool
}

// Binding is one keymap entry.
type Binding struct {
	Keys    []Stroke
	Command string
	When    string
}

// Map resolves key sequences to commands.
type Map struct {
	binds []Binding
}

// Default returns the built-in keymap. Unbound commands are absent.
func Default() *Map {
	m := &Map{}
	for _, row := range defaultRows {
		if row.keys == "" {
			continue
		}
		strokes, err := ParseSequence(row.keys)
		if err != nil {
			panic(err)
		}
		m.binds = append(m.binds, Binding{Keys: strokes, Command: row.command, When: row.when})
	}
	return m
}

type row struct {
	keys, command, when string
}

// defaultRows follows the build spec table. when is empty unless noted.
var defaultRows = []row{
	{"ctrl+s", "file.save", ""},
	{"ctrl+alt+n", "file.newScratch", ""},
	{"ctrl+f4", "file.close", ""},
	{"ctrl+shift+n", "nav.goToFile", ""},
	{"ctrl+alt+shift+n", "nav.goToSymbol", ""},
	{"ctrl+f12", "nav.fileStructure", ""},
	{"ctrl+b", "nav.goToDeclaration", ""},
	{"alt+f7", "nav.findUsages", ""},
	{"ctrl+alt+left", "nav.back", ""},
	{"ctrl+alt+right", "nav.forward", ""},
	{"ctrl+g", "nav.goToLine", ""},
	{"ctrl+e", "nav.recentFiles", ""},
	{"f2", "nav.nextError", ""},
	{"shift+f2", "nav.prevError", ""},
	{"alt+right", "nav.nextTab", ""},
	{"alt+left", "nav.prevTab", ""},
	{"ctrl+f", "edit.find", ""},
	{"ctrl+r", "edit.replace", ""},
	{"f3", "edit.findNext", ""},
	{"shift+f3", "edit.findPrev", ""},
	{"ctrl+shift+f", "search.inFiles", ""},
	{"ctrl+shift+r", "search.replaceInFiles", ""},
	{"ctrl+space", "edit.completion", ""},
	{"ctrl+q", "edit.quickDoc", ""},
	{"alt+j", "edit.addNextOccurrence", ""},
	{"alt+shift+j", "edit.unselectOccurrence", ""},
	{"ctrl+alt+shift+j", "edit.selectAllOccurrences", ""},
	{"ctrl+d", "edit.duplicateLine", ""},
	{"ctrl+y", "edit.deleteLine", ""},
	{"ctrl+/", "edit.toggleLineComment", ""},
	{"ctrl+-", "edit.fold", ""},
	{"ctrl+=", "edit.unfold", ""},
	{"ctrl+shift+-", "edit.foldAll", ""},
	{"ctrl+shift+=", "edit.unfoldAll", ""},
	{"ctrl+z", "edit.undo", ""},
	{"ctrl+shift+z", "edit.redo", ""},
	{"ctrl+c", "edit.copy", ""},
	{"ctrl+x", "edit.cut", ""},
	{"ctrl+v", "edit.paste", ""},
	{"ctrl+a", "edit.selectAll", ""},
	{"ctrl+alt+h", "edit.highlight", ""},
	{"ctrl+alt+shift+h", "edit.highlightLines", ""},
	{"alt+1", "view.projectTool", ""},
	{"alt+6", "view.problemsTool", ""},
	{"alt+9", "view.gitTool", ""},
	{"alt+f12", "view.terminal", ""},
	{"ctrl+k", "git.commit", ""},
	{"ctrl+shift+`", "git.branches", ""},
	{"ctrl+shift+a", "app.findAction", ""},
	{"ctrl+alt+s", "app.settings", ""},
	{"ctrl+shift+c", "terminal.copy", ""},
	{"ctrl+shift+v", "terminal.paste", ""},
	{"ctrl+t", "terminal.new", ""},
}

// file.saveAll, file.newScratchBuffer, file.newScratchFromSelection,
// file.renameScratch, file.moveScratch, view.splitRight, view.splitDown,
// git.refreshStatus, intel.restart, app.keymap, view.toggleTheme,
// edit.highlightError, edit.highlightSmell, and edit.clearHighlights are unbound.
// file.newScratch is ctrl+alt+n. Gio has no Insert key, so Ctrl+Alt+Shift+Insert is not bound.

type file struct {
	Bind   []bind   `toml:"bind"`
	Unbind []unbind `toml:"unbind"`
}

type bind struct {
	Keys    string `toml:"keys"`
	Command string `toml:"command"`
	When    string `toml:"when"`
}

type unbind struct {
	Keys string `toml:"keys"`
}

// Load parses a keymap file and applies it on top of base.
// A missing file returns base unchanged.
func Load(path string, base *Map) (*Map, error) {
	if base == nil {
		base = Default()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return base, nil
		}
		return nil, err
	}
	return Parse(string(b), base)
}

// Parse applies TOML keymap text on top of base.
func Parse(text string, base *Map) (*Map, error) {
	var f file
	if err := toml.Unmarshal([]byte(text), &f); err != nil {
		return nil, err
	}
	m := &Map{binds: append([]Binding(nil), base.binds...)}
	for _, u := range f.Unbind {
		seq, err := ParseSequence(u.Keys)
		if err != nil {
			return nil, fmt.Errorf("unbind %q: %w", u.Keys, err)
		}
		m.binds = removeSequence(m.binds, seq)
	}
	for _, b := range f.Bind {
		seq, err := ParseSequence(b.Keys)
		if err != nil {
			return nil, fmt.Errorf("bind %q: %w", b.Keys, err)
		}
		if _, err := compileWhen(b.When); err != nil {
			return nil, fmt.Errorf("when %q: %w", b.When, err)
		}
		m.binds = removeSequence(m.binds, seq)
		m.binds = append(m.binds, Binding{Keys: seq, Command: b.Command, When: b.When})
	}
	return m, nil
}

func removeSequence(binds []Binding, seq []Stroke) []Binding {
	out := binds[:0]
	for _, b := range binds {
		if sameSeq(b.Keys, seq) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func sameSeq(a, b []Stroke) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Lookup returns the command for an exact sequence whose when-expression
// matches, and whether any binding has seq as a proper prefix.
func (m *Map) Lookup(seq []Stroke, ctx Context) (command string, prefix bool) {
	for _, b := range m.binds {
		if len(b.Keys) > len(seq) && sameSeq(b.Keys[:len(seq)], seq) {
			ok, err := Eval(b.When, ctx)
			if err == nil && ok {
				prefix = true
			}
		}
		if sameSeq(b.Keys, seq) {
			ok, err := Eval(b.When, ctx)
			if err == nil && ok {
				command = b.Command
			}
		}
	}
	return command, prefix
}

// Bindings returns a copy of the bindings.
func (m *Map) Bindings() []Binding {
	return append([]Binding(nil), m.binds...)
}

// SetBinding replaces every sequence bound to command with seq.
// A different command that owned seq is removed and returned as displaced.
// seq is copied. An empty seq or command leaves the map unchanged and returns "".
func SetBinding(m *Map, seq []Stroke, command string) (displaced string) {
	if m == nil || len(seq) == 0 || command == "" {
		return ""
	}
	seq = cloneSeq(seq)
	old := append([]Binding(nil), m.binds...)
	next := make([]Binding, 0, len(old)+1)
	for _, b := range old {
		if b.Command == command {
			continue
		}
		if sameSeq(b.Keys, seq) {
			displaced = b.Command
			continue
		}
		next = append(next, b)
	}
	next = append(next, Binding{Keys: seq, Command: command})
	m.binds = next
	return displaced
}

// ResetCommand drops command's current sequences and puts Default()'s sequences
// for command back, including their when clauses. A default sequence owned by
// another command is taken back. Other commands' other sequences stay.
func ResetCommand(m *Map, command string) {
	if m == nil || command == "" {
		return
	}
	var restore []Binding
	for _, b := range Default().binds {
		if b.Command != command {
			continue
		}
		restore = append(restore, Binding{
			Keys:    cloneSeq(b.Keys),
			Command: b.Command,
			When:    b.When,
		})
	}
	old := append([]Binding(nil), m.binds...)
	next := make([]Binding, 0, len(old)+len(restore))
	for _, b := range old {
		if b.Command == command {
			continue
		}
		if _, ok := findBinding(restore, b.Keys); ok {
			continue
		}
		next = append(next, b)
	}
	m.binds = append(next, restore...)
}

// Overrides is the TOML diff of m against Default().
// Every default sequence that is missing or bound to a different command or when
// is an [[unbind]]. Every current sequence that is not an unchanged default
// (same strokes, command, and when) is a [[bind]]. Unbinds come first.
// An unchanged map returns "".
func Overrides(m *Map) string {
	if m == nil {
		return ""
	}
	def := Default()
	var b strings.Builder
	for _, d := range def.binds {
		cur, ok := findBinding(m.binds, d.Keys)
		if ok && cur.Command == d.Command && cur.When == d.When {
			continue
		}
		writeUnbind(&b, d.Keys)
	}
	for _, cur := range m.binds {
		d, ok := findBinding(def.binds, cur.Keys)
		if ok && d.Command == cur.Command && d.When == cur.When {
			continue
		}
		writeBind(&b, cur)
	}
	return b.String()
}

// Save writes Overrides(m) to path atomically (temp file in the same directory,
// then rename). It creates the parent directory. An empty diff writes an empty file
// so a later Load returns the defaults. A nil map writes an empty file.
func Save(path string, m *Map) error {
	var data []byte
	if m != nil {
		data = []byte(Overrides(m))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("save keymap %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save keymap %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save keymap %s: %w", path, err)
	}
	return nil
}

func cloneSeq(seq []Stroke) []Stroke {
	return append([]Stroke(nil), seq...)
}

func findBinding(binds []Binding, seq []Stroke) (Binding, bool) {
	for _, b := range binds {
		if sameSeq(b.Keys, seq) {
			return b, true
		}
	}
	return Binding{}, false
}

func seqText(seq []Stroke) string {
	parts := make([]string, len(seq))
	for i, s := range seq {
		parts[i] = s.String()
	}
	return strings.Join(parts, " ")
}

func writeUnbind(b *strings.Builder, keys []Stroke) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("[[unbind]]\nkeys = ")
	b.WriteString(strconv.Quote(seqText(keys)))
	b.WriteByte('\n')
}

func writeBind(b *strings.Builder, bind Binding) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("[[bind]]\nkeys = ")
	b.WriteString(strconv.Quote(seqText(bind.Keys)))
	b.WriteString("\ncommand = ")
	b.WriteString(strconv.Quote(bind.Command))
	b.WriteByte('\n')
	if bind.When != "" {
		b.WriteString("when = ")
		b.WriteString(strconv.Quote(bind.When))
		b.WriteByte('\n')
	}
}

// Resolver tracks chord state. The chord timeout is 1.5s.
type Resolver struct {
	Map     *Map
	pending []Stroke
	since   time.Time
	Timeout time.Duration
}

func (r *Resolver) timeout() time.Duration {
	if r.Timeout == 0 {
		return 1500 * time.Millisecond
	}
	return r.Timeout
}

// Push consumes one stroke. cmd is set when a binding matches.
// more is true while a chord is in progress.
func (r *Resolver) Push(s Stroke, ctx Context, now time.Time) (cmd string, more bool) {
	if len(r.pending) > 0 && now.Sub(r.since) > r.timeout() {
		r.pending = nil
	}
	seq := append(append([]Stroke{}, r.pending...), s)
	command, prefix := r.Map.Lookup(seq, ctx)
	if command != "" {
		r.pending = nil
		return command, false
	}
	if prefix {
		r.pending = seq
		r.since = now
		return "", true
	}
	if len(r.pending) > 0 {
		r.pending = nil
		return r.Push(s, ctx, now)
	}
	return "", false
}

func (r *Resolver) Clear() { r.pending = nil }

// ParseSequence parses "ctrl+shift+n" or "alt+g b".
func ParseSequence(s string) ([]Stroke, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty key sequence")
	}
	parts := strings.Fields(s)
	out := make([]Stroke, 0, len(parts))
	for _, p := range parts {
		st, err := ParseStroke(p)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

// ParseStroke parses one stroke such as "ctrl+shift+n" or "f2".
func ParseStroke(s string) (Stroke, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return Stroke{}, fmt.Errorf("empty stroke")
	}
	// A trailing '+' means the key itself is '+', as in "ctrl++".
	keyIsPlus := strings.HasSuffix(s, "++") || s == "+"
	parts := strings.Split(s, "+")
	if keyIsPlus {
		parts = parts[:len(parts)-1]
		if len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
	}
	var key string
	var mods []string
	if keyIsPlus {
		key = "+"
		mods = parts
	} else {
		if len(parts) == 0 {
			return Stroke{}, fmt.Errorf("bad stroke %q", s)
		}
		key = parts[len(parts)-1]
		mods = parts[:len(parts)-1]
	}
	if !validKey(key) {
		return Stroke{}, fmt.Errorf("bad key %q", key)
	}
	var m Mod
	for _, mod := range mods {
		switch mod {
		case "ctrl":
			m |= ModCtrl
		case "alt":
			m |= ModAlt
		case "shift":
			m |= ModShift
		case "super":
			m |= ModSuper
		case "":
			return Stroke{}, fmt.Errorf("bad stroke %q", s)
		default:
			return Stroke{}, fmt.Errorf("bad modifier %q", mod)
		}
	}
	return Stroke{Mod: m, Key: key}, nil
}

func validKey(k string) bool {
	if k == "" {
		return false
	}
	switch k {
	case "left", "right", "up", "down",
		"home", "end", "pageup", "pagedown",
		"enter", "tab", "space", "backspace", "delete", "escape",
		"`", "-", "=", "[", "]", "\\", ";", "'", ",", ".", "/", "+", "*":
		return true
	}
	if len(k) >= 2 && k[0] == 'f' {
		n := 0
		for _, r := range k[1:] {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		return n >= 1 && n <= 24
	}
	if len(k) == 1 {
		r := rune(k[0])
		return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	}
	return false
}

// Eval evaluates a when-expression. An empty expression is true.
// The grammar is identifiers, '!', and '&&'.
func Eval(expr string, ctx Context) (bool, error) {
	fn, err := compileWhen(expr)
	if err != nil {
		return false, err
	}
	return fn(ctx), nil
}

func compileWhen(expr string) (func(Context) bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return func(Context) bool { return true }, nil
	}
	p := &wparser{s: expr}
	fn, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i != len(p.s) {
		return nil, fmt.Errorf("trailing %q", p.s[p.i:])
	}
	return fn, nil
}

type wparser struct {
	s string
	i int
}

func (p *wparser) skip() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

func (p *wparser) parseAnd() (func(Context) bool, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		p.skip()
		if strings.HasPrefix(p.s[p.i:], "&&") {
			p.i += 2
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			l, r := left, right
			left = func(c Context) bool { return l(c) && r(c) }
			continue
		}
		return left, nil
	}
}

func (p *wparser) parseUnary() (func(Context) bool, error) {
	p.skip()
	if p.i >= len(p.s) {
		return nil, fmt.Errorf("unexpected end")
	}
	if p.s[p.i] == '!' {
		p.i++
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return func(c Context) bool { return !inner(c) }, nil
	}
	start := p.i
	for p.i < len(p.s) && (unicode.IsLetter(rune(p.s[p.i])) || p.s[p.i] == '_') {
		p.i++
	}
	if start == p.i {
		return nil, fmt.Errorf("expected identifier at %q", p.s[p.i:])
	}
	name := p.s[start:p.i]
	switch name {
	case "editorFocus":
		return func(c Context) bool { return c.EditorFocus }, nil
	case "terminalFocus":
		return func(c Context) bool { return c.TerminalFocus }, nil
	case "treeFocus":
		return func(c Context) bool { return c.TreeFocus }, nil
	case "popupOpen":
		return func(c Context) bool { return c.PopupOpen }, nil
	case "hasSelection":
		return func(c Context) bool { return c.HasSelection }, nil
	default:
		return nil, fmt.Errorf("unknown context %q", name)
	}
}

// TerminalPassthrough reports whether a stroke is delivered to gio-ide
// while the terminal has focus. Every other stroke goes to the PTY.
func TerminalPassthrough(s Stroke) bool {
	switch s {
	case Stroke{Mod: ModAlt, Key: "f12"},
		Stroke{Mod: ModAlt, Key: "1"},
		Stroke{Mod: ModAlt, Key: "6"},
		Stroke{Mod: ModAlt, Key: "9"},
		Stroke{Mod: ModCtrl | ModShift, Key: "a"},
		Stroke{Mod: ModCtrl | ModShift, Key: "n"},
		Stroke{Mod: ModCtrl | ModShift, Key: "c"},
		Stroke{Mod: ModCtrl | ModShift, Key: "v"},
		Stroke{Mod: ModCtrl, Key: "t"}:
		return true
	default:
		return false
	}
}
