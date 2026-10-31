// Package settings loads the layered TOML settings file.
package settings

import (
	"fmt"
	"maps"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pelletier/go-toml/v2"
)

// Problem is a warning shown in the Problems list.
type Problem struct {
	Key string
	Msg string
}

// Settings is the merged view of defaults, the user file, and the project file.
type Settings struct {
	Editor   Editor    `toml:"editor"`
	UI       UI        `toml:"ui"`
	Intel    Intel     `toml:"intel"`
	Runtime  Runtime   `toml:"runtime"`
	Search   Search    `toml:"search"`
	Files    Files     `toml:"files"`
	Terminal Terminal  `toml:"terminal"`
	Git      Git       `toml:"git"`
	Problems []Problem `toml:"-"`
}

type Editor struct {
	FontFamily     string  `toml:"font_family"`
	FontSize       float64 `toml:"font_size"`
	LineHeight     float64 `toml:"line_height"`
	TabWidth       int     `toml:"tab_width"`
	InsertSpaces   bool    `toml:"insert_spaces"`
	SoftWrap       bool    `toml:"soft_wrap"`
	ShowWhitespace bool    `toml:"show_whitespace"`
	// GofumptOnSave formats a .go file on save with the embedded gofumpt.
	// It defaults off. The reformat command does not read it.
	GofumptOnSave bool `toml:"gofumpt_on_save"`
	// GoimportsOnSave formats a .go file on save with the embedded goimports.
	// It defaults off. The organize-imports command does not read it.
	GoimportsOnSave bool `toml:"goimports_on_save"`
	// GofixOnSave runs go fix on a .go file before it is written.
	// It defaults off. When the other save tools are also on, go fix runs
	// first, then goimports, then gofumpt. Only the saved file is rewritten.
	GofixOnSave bool `toml:"gofix_on_save"`
	// CursorBlinkMS is one full caret on/off cycle. Zero keeps the caret lit.
	CursorBlinkMS int `toml:"cursor_blink_ms"`
}

type UI struct {
	Theme      string  `toml:"theme"`
	UIFontSize float64 `toml:"ui_font_size"`
}

type Intel struct {
	Enabled             bool           `toml:"enabled"`
	SemanticTokens      bool           `toml:"semantic_tokens"`
	CompletionTimeoutMS int            `toml:"completion_timeout_ms"`
	HoverTimeoutMS      int            `toml:"hover_timeout_ms"`
	DefinitionTimeoutMS int            `toml:"definition_timeout_ms"`
	ReferencesTimeoutMS int            `toml:"references_timeout_ms"`
	SymbolsTimeoutMS    int            `toml:"symbols_timeout_ms"`
	Gopls               map[string]any `toml:"gopls"`
}

type Runtime struct {
	MemoryLimit string `toml:"memory_limit"`
}

type Search struct {
	Engine     string   `toml:"engine"`
	Exclude    []string `toml:"exclude"`
	MaxResults int      `toml:"max_results"`
}

type Files struct {
	Exclude []string `toml:"exclude"`
	Watch   bool     `toml:"watch"`
}

type Terminal struct {
	Shell           string  `toml:"shell"`
	ScrollbackLines int     `toml:"scrollback_lines"`
	FontSize        float64 `toml:"font_size"`
}

type Git struct {
	Gutter           bool `toml:"gutter"`
	StatusDebounceMS int  `toml:"status_debounce_ms"`
}

// Defaults returns the design defaults from the build spec.
func Defaults() Settings {
	return Settings{
		Editor: Editor{
			FontFamily:      "Go Mono",
			FontSize:        14,
			LineHeight:      1.3,
			TabWidth:        4,
			InsertSpaces:    false,
			SoftWrap:        false,
			ShowWhitespace:  false,
			GofumptOnSave:   false,
			GoimportsOnSave: false,
			GofixOnSave:     false,
			CursorBlinkMS:   500,
		},
		UI: UI{Theme: "dark", UIFontSize: 13},
		Intel: Intel{
			Enabled:             true,
			SemanticTokens:      false,
			CompletionTimeoutMS: 1500,
			HoverTimeoutMS:      1500,
			DefinitionTimeoutMS: 3000,
			ReferencesTimeoutMS: 10000,
			SymbolsTimeoutMS:    5000,
			Gopls: map[string]any{
				"staticcheck":      false,
				"directoryFilters": []string{"-node_modules"},
			},
		},
		Runtime:  Runtime{MemoryLimit: "auto"},
		Search:   Search{Engine: "builtin", Exclude: []string{"vendor/"}, MaxResults: 20000},
		Files:    Files{Exclude: []string{"node_modules/"}, Watch: true},
		Terminal: Terminal{Shell: "", ScrollbackLines: 10000, FontSize: 13},
		Git:      Git{Gutter: true, StatusDebounceMS: 300},
	}
}

// Load merges defaults, then the user file, then the project file.
// A missing file is skipped. Unknown keys warn. Bad values fall back.
// The project file cannot set terminal.shell, editor.goimports_on_save,
// or editor.gofix_on_save. Those keys stay at the user value.
func Load(userPath, projectPath string) (Settings, []Problem) {
	s := Defaults()
	var problems []Problem
	if userPath != "" {
		applyFile(&s, userPath, "user", &problems)
	}
	if projectPath != "" {
		applyFile(&s, projectPath, "project", &problems)
	}
	s.Problems = problems
	return s, problems
}

func applyFile(s *Settings, path, layer string, problems *[]Problem) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		*problems = append(*problems, Problem{Key: layer, Msg: err.Error()})
		return
	}
	var raw map[string]any
	if err := toml.Unmarshal(b, &raw); err != nil {
		*problems = append(*problems, Problem{Key: layer, Msg: "toml: " + err.Error()})
		return
	}
	applyStruct(reflect.ValueOf(s), raw, "", problems, layer == "project")
}

// projectIgnores reports keys that must not come from a project file.
// Those keys start a process.
func projectIgnores(key string) bool {
	switch key {
	case "terminal.shell", "editor.goimports_on_save", "editor.gofix_on_save":
		return true
	default:
		return false
	}
}

func applyStruct(dst reflect.Value, table map[string]any, path string, problems *[]Problem, project bool) {
	for dst.Kind() == reflect.Pointer {
		dst = dst.Elem()
	}
	known := map[string]int{}
	t := dst.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("toml")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		known[name] = i
	}
	for k, v := range table {
		full := k
		if path != "" {
			full = path + "." + k
		}
		i, ok := known[k]
		if !ok {
			*problems = append(*problems, Problem{Key: full, Msg: "unknown key"})
			continue
		}
		field := dst.Field(i)
		if !field.CanSet() {
			continue
		}
		if project && projectIgnores(full) {
			*problems = append(*problems, Problem{Key: full, Msg: "ignored in the project file"})
			continue
		}
		if err := assign(field, v, full, problems, project); err != nil {
			*problems = append(*problems, Problem{Key: full, Msg: "bad value, using default: " + err.Error()})
		}
	}
}

func assign(field reflect.Value, v any, path string, problems *[]Problem, project bool) error {
	if v == nil {
		return fmt.Errorf("null")
	}
	switch field.Kind() {
	case reflect.Struct:
		m, ok := asMap(v)
		if !ok {
			return fmt.Errorf("want table, got %T", v)
		}
		applyStruct(field, m, path, problems, project)
		return nil
	case reflect.Map:
		m, ok := asMap(v)
		if !ok {
			return fmt.Errorf("want table, got %T", v)
		}
		return mergeGopls(field, m, path, problems)
	case reflect.Bool:
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("want bool, got %T", v)
		}
		field.SetBool(b)
		return nil
	case reflect.String:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("want string, got %T", v)
		}
		if path == "runtime.memory_limit" || strings.HasSuffix(path, "memory_limit") {
			if _, err := ParseMemoryLimit(s); err != nil && s != "auto" && s != "off" {
				return err
			}
		}
		if strings.HasSuffix(path, "engine") && s != "builtin" && s != "rg" {
			return fmt.Errorf("engine must be builtin or rg")
		}
		if strings.HasSuffix(path, "theme") && strings.TrimSpace(s) == "" {
			return fmt.Errorf("empty theme")
		}
		field.SetString(s)
		return nil
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, ok := asInt(v)
		if !ok {
			return fmt.Errorf("want int, got %T", v)
		}
		if n < 0 {
			return fmt.Errorf("negative")
		}
		field.SetInt(int64(n))
		return nil
	case reflect.Float64, reflect.Float32:
		f, ok := asFloat(v)
		if !ok {
			return fmt.Errorf("want number, got %T", v)
		}
		if f <= 0 {
			return fmt.Errorf("must be positive")
		}
		field.SetFloat(f)
		return nil
	case reflect.Slice:
		if field.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("unsupported slice")
		}
		ss, ok := asStringSlice(v)
		if !ok {
			return fmt.Errorf("want string array, got %T", v)
		}
		field.Set(reflect.ValueOf(ss))
		return nil
	default:
		return fmt.Errorf("unsupported kind %s", field.Kind())
	}
}

func mergeGopls(field reflect.Value, m map[string]any, path string, problems *[]Problem) error {
	cur, _ := field.Interface().(map[string]any)
	if cur == nil {
		cur = map[string]any{}
	}
	next := map[string]any{}
	maps.Copy(next, cur)
	known := map[string]string{
		"staticcheck":      "bool",
		"directoryFilters": "[]string",
	}
	for k, v := range m {
		full := path + "." + k
		want, ok := known[k]
		if !ok {
			*problems = append(*problems, Problem{Key: full, Msg: "unknown key"})
			continue
		}
		switch want {
		case "bool":
			b, ok := v.(bool)
			if !ok {
				*problems = append(*problems, Problem{Key: full, Msg: "bad value, using default: want bool"})
				continue
			}
			next[k] = b
		case "[]string":
			ss, ok := asStringSlice(v)
			if !ok {
				*problems = append(*problems, Problem{Key: full, Msg: "bad value, using default: want string array"})
				continue
			}
			next[k] = ss
		}
	}
	field.Set(reflect.ValueOf(next))
	return nil
}

func asMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	default:
		return nil, false
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case int:
		return n, true
	case uint64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func asStringSlice(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		out := append([]string(nil), s...)
		return out, true
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	default:
		return nil, false
	}
}

// ParseMemoryLimit parses "auto", "off", or a size such as "6GiB".
// "auto" and "off" return 0 with a nil error. The caller treats them specially.
func ParseMemoryLimit(s string) (int64, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "auto", "off":
		return 0, nil
	}
	units := []struct {
		suf string
		mul int64
	}{
		{"tib", 1 << 40},
		{"gib", 1 << 30},
		{"mib", 1 << 20},
		{"kib", 1 << 10},
		{"tb", 1000 * 1000 * 1000 * 1000},
		{"gb", 1000 * 1000 * 1000},
		{"mb", 1000 * 1000},
		{"kb", 1000},
		{"b", 1},
	}
	lower := strings.ToLower(s)
	for _, u := range units {
		if strings.HasSuffix(lower, u.suf) {
			num := strings.TrimSpace(lower[:len(lower)-len(u.suf)])
			f, err := strconv.ParseFloat(num, 64)
			if err != nil || f <= 0 {
				return 0, fmt.Errorf("bad memory size %q", s)
			}
			return int64(f * float64(u.mul)), nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("bad memory size %q", s)
	}
	return n, nil
}

// Store holds the live settings and reloads them when the files change.
type Store struct {
	mu       sync.RWMutex
	cur      Settings
	user     string
	project  string
	onChange func(Settings)
}

func NewStore(userPath, projectPath string) *Store {
	s, _ := Load(userPath, projectPath)
	return &Store{cur: s, user: userPath, project: projectPath}
}

func (s *Store) Current() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cur)
}

func (s *Store) Reload() Settings {
	// Load stays under the lock with the publish. Apply holds the same
	// lock across its write, so a reload that started from the pre-write
	// file cannot store that snapshot afterward.
	s.mu.Lock()
	next, _ := Load(s.user, s.project)
	s.cur = next
	cb := s.onChange
	out := clone(next)
	s.mu.Unlock()
	if cb != nil {
		cb(next)
	}
	return out
}

// OnChange registers a callback invoked on the watcher goroutine after a reload.
func (s *Store) OnChange(fn func(Settings)) {
	s.mu.Lock()
	s.onChange = fn
	s.mu.Unlock()
}

// Watch reloads when the user or project settings file changes.
func (s *Store) Watch(stop <-chan struct{}) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dirs := map[string]struct{}{}
	for _, p := range []string{s.user, s.project} {
		if p == "" {
			continue
		}
		dir := dirOf(p)
		if _, ok := dirs[dir]; ok {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		if err := w.Add(dir); err != nil {
			continue
		}
		dirs[dir] = struct{}{}
	}
	go func() {
		defer w.Close()
		var timer *time.Timer
		for {
			select {
			case <-stop:
				return
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(100*time.Millisecond, func() { s.Reload() })
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

func dirOf(p string) string {
	i := strings.LastIndex(p, string(os.PathSeparator))
	if i < 0 {
		return "."
	}
	return p[:i]
}

func clone(s Settings) Settings {
	s.Problems = append([]Problem(nil), s.Problems...)
	s.Search.Exclude = append([]string(nil), s.Search.Exclude...)
	s.Files.Exclude = append([]string(nil), s.Files.Exclude...)
	g := map[string]any{}
	for k, v := range s.Intel.Gopls {
		switch t := v.(type) {
		case []string:
			g[k] = append([]string(nil), t...)
		default:
			g[k] = v
		}
	}
	s.Intel.Gopls = g
	return s
}

// GoplsConfig returns a copy of the [intel.gopls] table for the Configuration callback.
func (s Settings) GoplsConfig() map[string]any {
	out := map[string]any{}
	for k, v := range s.Intel.Gopls {
		switch t := v.(type) {
		case []string:
			cp := append([]string(nil), t...)
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
}

// WriteFile replaces path with s. A missing parent directory is created.
func WriteFile(path string, s Settings) error {
	b, err := toml.Marshal(s)
	if err != nil {
		return err
	}
	return writeAtomic(path, b)
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Apply edits the merged settings. When a user file is configured, only the
// keys fn changed are written there, and the merged view is reloaded so a
// project file still wins. A failed write leaves the store unchanged and
// does not run OnChange. The lock is held from the edit through the write
// and the publish, so a concurrent Reload cannot store a snapshot it read
// before that write. fn must not call back into the Store (Current, Reload,
// and Apply would deadlock). OnChange runs after the lock is released.
func (s *Store) Apply(fn func(*Settings)) error {
	if fn == nil {
		return nil
	}
	s.mu.Lock()
	next, cb, err := s.applyLocked(fn)
	s.mu.Unlock()
	if err != nil || cb == nil {
		return err
	}
	cb(next)
	return nil
}

// applyLocked runs fn and, when a user file is set, writes the delta.
// The caller holds s.mu. cb is nil when nothing changed.
func (s *Store) applyLocked(fn func(*Settings)) (Settings, func(Settings), error) {
	prev := clone(s.cur)
	next := clone(s.cur)
	fn(&next)
	if s.user != "" {
		delta := diffSettings(prev, next)
		if len(delta) == 0 {
			return Settings{}, nil, nil
		}
		if err := patchUserFile(s.user, delta); err != nil {
			return Settings{}, nil, err
		}
		loaded, _ := Load(s.user, s.project)
		next = loaded
	}
	s.cur = next
	return next, s.onChange, nil
}

// patchUserFile merges delta into the existing user TOML. Keys the user did
// not change, including ones that exist only in the project file, stay out.
func patchUserFile(path string, delta map[string]any) error {
	raw := map[string]any{}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && strings.TrimSpace(string(b)) != "" {
		if err := toml.Unmarshal(b, &raw); err != nil {
			return err
		}
	}
	mergeMaps(raw, delta)
	out, err := toml.Marshal(raw)
	if err != nil {
		return err
	}
	return writeAtomic(path, out)
}

func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			delete(dst, k)
			continue
		}
		sub, ok := v.(map[string]any)
		if !ok {
			dst[k] = v
			continue
		}
		existing, _ := dst[k].(map[string]any)
		if existing == nil {
			dst[k] = sub
			continue
		}
		mergeMaps(existing, sub)
	}
}

func diffSettings(prev, next Settings) map[string]any {
	out := map[string]any{}
	diffStruct(reflect.ValueOf(prev), reflect.ValueOf(next), out)
	return out
}

func diffStruct(prev, next reflect.Value, into map[string]any) {
	t := prev.Type()
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("toml"), ",")
		if name == "" || name == "-" {
			continue
		}
		pv, nv := prev.Field(i), next.Field(i)
		if pv.Kind() == reflect.Struct {
			if reflect.DeepEqual(pv.Interface(), nv.Interface()) {
				continue
			}
			sub := map[string]any{}
			diffStruct(pv, nv, sub)
			if len(sub) > 0 {
				into[name] = sub
			}
			continue
		}
		if pv.Kind() == reflect.Map {
			sub := diffMap(pv, nv)
			if len(sub) > 0 {
				into[name] = sub
			}
			continue
		}
		if !reflect.DeepEqual(pv.Interface(), nv.Interface()) {
			into[name] = nv.Interface()
		}
	}
}

// diffMap returns keys that differ. A nil map is empty. Equal values are
// omitted, additions and edits keep the next value, and removals are nil.
// Slice values are kept whole.
func diffMap(prev, next reflect.Value) map[string]any {
	out := map[string]any{}
	prevVals := mapEntries(prev)
	nextVals := mapEntries(next)
	for k, nv := range nextVals {
		pv, ok := prevVals[k]
		if ok && reflect.DeepEqual(pv, nv) {
			continue
		}
		out[k] = nv
	}
	for k := range prevVals {
		if _, ok := nextVals[k]; !ok {
			out[k] = nil
		}
	}
	return out
}

func mapEntries(v reflect.Value) map[string]any {
	out := map[string]any{}
	if !v.IsValid() || v.Kind() != reflect.Map || v.IsNil() {
		return out
	}
	iter := v.MapRange()
	for iter.Next() {
		key, ok := iter.Key().Interface().(string)
		if !ok {
			continue
		}
		out[key] = iter.Value().Interface()
	}
	return out
}

// WriteDefault writes the default settings file if path does not exist.
func WriteDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultTOML), 0o644)
}

const defaultTOML = `# gio-ide settings. Unknown keys warn. Bad values fall back to defaults.
[editor]
font_family = "Go Mono"
font_size = 14
line_height = 1.3
tab_width = 4
insert_spaces = false
soft_wrap = false
show_whitespace = false
# Format .go files on save. When several are on, the order is go fix, goimports, gofumpt. Default off.
gofix_on_save = false
gofumpt_on_save = false
goimports_on_save = false
cursor_blink_ms = 500

[ui]
theme = "dark" # "dark" | "light" | a file name in themes/
ui_font_size = 13

[intel]
enabled = true
semantic_tokens = false
completion_timeout_ms = 1500
hover_timeout_ms = 1500
definition_timeout_ms = 3000
references_timeout_ms = 10000
symbols_timeout_ms = 5000

[intel.gopls]
staticcheck = false
directoryFilters = ["-node_modules"]

[runtime]
memory_limit = "auto" # "auto" | "off" | "6GiB"

[search]
engine = "builtin" # "builtin" | "rg"
exclude = ["vendor/"]
max_results = 20000

[files]
exclude = ["node_modules/"]
watch = true

[terminal]
shell = ""
scrollback_lines = 10000
font_size = 13

[git]
gutter = true
status_debounce_ms = 300
`
