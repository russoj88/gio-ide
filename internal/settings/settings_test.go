package settings

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLayerOrderUnknownAndBad(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	project := filepath.Join(dir, "project.toml")
	if err := os.WriteFile(user, []byte(`
[editor]
font_size = 18
font_family = "big" # valid string

[ui]
theme = "dark"

[editor.not_a_key]
x = 1

not_a_section = true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// font_size bad in a second check; project overrides theme and a bad number.
	if err := os.WriteFile(project, []byte(`
[ui]
theme = "light"

[editor]
font_size = "nope"
tab_width = -3

[intel.gopls]
staticcheck = "yes"
directoryFilters = ["-vendor"]
mystery = 1

[search]
engine = "rg"
max_results = 10
`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, problems := Load(user, project)
	if s.Editor.FontSize != 18 {
		t.Fatalf("font size fell through, got %v", s.Editor.FontSize)
	}
	if s.UI.Theme != "light" {
		t.Fatalf("theme: %s", s.UI.Theme)
	}
	if s.Editor.TabWidth != 4 {
		t.Fatalf("tab width: %d", s.Editor.TabWidth)
	}
	if s.Search.Engine != "rg" || s.Search.MaxResults != 10 {
		t.Fatalf("search: %+v", s.Search)
	}
	if s.Intel.Gopls["staticcheck"] != false {
		t.Fatalf("staticcheck: %#v", s.Intel.Gopls["staticcheck"])
	}
	filters, _ := s.Intel.Gopls["directoryFilters"].([]string)
	if len(filters) != 1 || filters[0] != "-vendor" {
		t.Fatalf("filters: %#v", s.Intel.Gopls["directoryFilters"])
	}
	if len(problems) == 0 {
		t.Fatal("expected problems")
	}
	var unknown, bad bool
	for _, p := range problems {
		if p.Msg == "unknown key" {
			unknown = true
		}
		if len(p.Msg) >= 9 && p.Msg[:9] == "bad value" {
			bad = true
		}
	}
	if !unknown || !bad {
		t.Fatalf("problems: %#v", problems)
	}
	// User-only layer still beats defaults.
	only, _ := Load(user, filepath.Join(dir, "missing.toml"))
	if only.Editor.FontSize != 18 || only.UI.Theme != "dark" {
		t.Fatalf("user layer: %+v %+v", only.Editor, only.UI)
	}
	def, probs := Load("", "")
	if def.Editor.FontSize != 14 || def.UI.Theme != "dark" || def.Intel.Enabled != true {
		t.Fatalf("defaults: %+v", def)
	}
	if len(probs) != 0 {
		t.Fatal(probs)
	}
}

func TestMemoryLimitParse(t *testing.T) {
	n, err := ParseMemoryLimit("6GiB")
	if err != nil || n != 6*(1<<30) {
		t.Fatalf("%d %v", n, err)
	}
	if _, err := ParseMemoryLimit("auto"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMemoryLimit("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	s := Defaults()
	s.Editor.FontSize = 18
	s.Editor.CursorBlinkMS = 0
	if err := WriteFile(path, s); err != nil {
		t.Fatal(err)
	}
	got, problems := Load(path, "")
	if len(problems) != 0 {
		t.Fatalf("problems: %#v", problems)
	}
	if got.Editor.FontSize != 18 {
		t.Fatalf("font size %v", got.Editor.FontSize)
	}
	if got.Editor.CursorBlinkMS != 0 {
		t.Fatalf("cursor blink %d, want 0 to survive", got.Editor.CursorBlinkMS)
	}
	if got.Editor.TabWidth != 4 {
		t.Fatalf("tab width %d", got.Editor.TabWidth)
	}
	if got.UI.Theme != "dark" {
		t.Fatalf("theme %s", got.UI.Theme)
	}
}

func TestFormatOnSaveLoadsAndRejectsBadType(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	project := filepath.Join(dir, "project.toml")
	if err := os.WriteFile(user, []byte(`
[editor]
gofumpt_on_save = true
goimports_on_save = true
gofix_on_save = true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, problems := Load(user, "")
	if len(problems) != 0 {
		t.Fatalf("problems: %#v", problems)
	}
	if !got.Editor.GofumptOnSave || !got.Editor.GoimportsOnSave || !got.Editor.GofixOnSave {
		t.Fatalf("editor: %+v", got.Editor)
	}
	if err := os.WriteFile(project, []byte(`
[editor]
gofumpt_on_save = "yes"
goimports_on_save = "yes"
gofix_on_save = "yes"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, problems = Load(user, project)
	if !got.Editor.GofumptOnSave || !got.Editor.GoimportsOnSave || !got.Editor.GofixOnSave {
		t.Fatalf("bad type overwrote the user layer: %+v", got.Editor)
	}
	if !badValue(problems) {
		t.Fatalf("problems: %#v", problems)
	}
	bad := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(bad, []byte(`
[editor]
gofumpt_on_save = "yes"
goimports_on_save = "yes"
gofix_on_save = "yes"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, problems = Load(bad, "")
	if got.Editor.GofumptOnSave || got.Editor.GoimportsOnSave || got.Editor.GofixOnSave {
		t.Fatalf("bad type set the field: %+v", got.Editor)
	}
	if !badValue(problems) {
		t.Fatalf("problems: %#v", problems)
	}
}

func ignoredProjectKey(problems []Problem, key string) bool {
	for _, p := range problems {
		if p.Key == key && p.Msg == "ignored in the project file" {
			return true
		}
	}
	return false
}

func TestProjectFileCannotStartAProcess(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	project := filepath.Join(dir, "project.toml")
	if err := os.WriteFile(user, []byte("[terminal]\nshell = \"/bin/sh\"\n\n[editor]\ngoimports_on_save = false\ngofix_on_save = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("[terminal]\nshell = \"/tmp/evil\"\n\n[editor]\ngoimports_on_save = true\ngofix_on_save = true\nfont_size = 18\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, problems := Load(user, project)
	if got.Terminal.Shell != "/bin/sh" {
		t.Fatalf("shell %q", got.Terminal.Shell)
	}
	if got.Editor.GoimportsOnSave || got.Editor.GofixOnSave {
		t.Fatalf("project process flags applied: %+v", got.Editor)
	}
	if got.Editor.FontSize != 18 {
		t.Fatalf("font %v", got.Editor.FontSize)
	}
	for _, key := range []string{"terminal.shell", "editor.goimports_on_save", "editor.gofix_on_save"} {
		if !ignoredProjectKey(problems, key) {
			t.Fatalf("missing ignore for %s: %#v", key, problems)
		}
	}
}

func badValue(problems []Problem) bool {
	for _, p := range problems {
		if len(p.Msg) >= 9 && p.Msg[:9] == "bad value" {
			return true
		}
	}
	return false
}

func TestApplyGofumptOnSaveWritesOnlyThatKey(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	project := filepath.Join(dir, "project.toml")
	if err := os.WriteFile(user, []byte("[editor]\nfont_family = \"Go Mono\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("[editor]\nfont_size = 18\ngoimports_on_save = true\n\n[intel]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(user, project)
	cur := store.Current()
	if cur.Editor.FontSize != 18 || cur.Editor.GoimportsOnSave || cur.Editor.GofumptOnSave || cur.Editor.GofixOnSave || cur.Intel.Enabled {
		t.Fatalf("merged view %+v %+v", cur.Editor, cur.Intel)
	}
	if err := store.Apply(func(st *Settings) {
		st.Editor.GofumptOnSave = true
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.Current().Editor.GofumptOnSave; !got {
		t.Fatal("memory gofumpt on save not set")
	}
	b, err := os.ReadFile(user)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "gofumpt_on_save = true") {
		t.Fatalf("user file:\n%s", text)
	}
	if strings.Contains(text, "font_size") || strings.Contains(text, "enabled") || strings.Contains(text, "goimports_on_save") {
		t.Fatalf("project keys copied into the user file:\n%s", text)
	}
	if !strings.Contains(text, "font_family") {
		t.Fatalf("existing user key dropped:\n%s", text)
	}
	got, problems := Load(user, project)
	if !ignoredProjectKey(problems, "editor.goimports_on_save") {
		t.Fatalf("problems: %#v", problems)
	}
	if !got.Editor.GofumptOnSave {
		t.Fatal("reloaded gofumpt on save not set")
	}
	if got.Editor.GoimportsOnSave {
		t.Fatal("project goimports was applied")
	}
	if got.Editor.FontSize != 18 {
		t.Fatalf("project font size %v", got.Editor.FontSize)
	}
	if got.Intel.Enabled {
		t.Fatal("project intel override lost")
	}
}

func TestApplyWritesUserFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.toml")
	store := NewStore(path, "")
	if err := store.Apply(func(st *Settings) {
		st.Editor.FontSize = 18
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.Current().Editor.FontSize; got != 18 {
		t.Fatalf("memory font size %v", got)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "font_size = 18") {
		t.Fatalf("user file:\n%s", b)
	}
	got, problems := Load(path, "")
	if len(problems) != 0 {
		t.Fatalf("problems: %#v", problems)
	}
	if got.Editor.FontSize != 18 {
		t.Fatalf("reloaded font size %v", got.Editor.FontSize)
	}
}

func TestApplyKeepsProjectKeysOutOfTheUserFile(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	project := filepath.Join(dir, "project.toml")
	if err := os.WriteFile(user, []byte("[editor]\nfont_family = \"Go Mono\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("[editor]\nfont_size = 18\n\n[intel]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(user, project)
	if store.Current().Editor.FontSize != 18 || store.Current().Intel.Enabled {
		t.Fatalf("merged view %+v %+v", store.Current().Editor, store.Current().Intel)
	}
	called := false
	store.OnChange(func(Settings) { called = true })
	if err := store.Apply(func(st *Settings) {
		st.Editor.CursorBlinkMS = 0
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("OnChange did not run")
	}
	b, err := os.ReadFile(user)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, "font_size") || strings.Contains(text, "enabled") {
		t.Fatalf("project keys copied into the user file:\n%s", text)
	}
	if !strings.Contains(text, "cursor_blink_ms = 0") {
		t.Fatalf("user file:\n%s", text)
	}
	if !strings.Contains(text, "font_family") {
		t.Fatalf("existing user key dropped:\n%s", text)
	}
	cur := store.Current()
	if cur.Editor.FontSize != 18 {
		t.Fatalf("project font size %v", cur.Editor.FontSize)
	}
	if cur.Editor.CursorBlinkMS != 0 {
		t.Fatalf("blink %d", cur.Editor.CursorBlinkMS)
	}
	if cur.Intel.Enabled {
		t.Fatal("project intel override lost")
	}
}

func TestApplyGoplsKeyDoesNotCopyProjectKeys(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	project := filepath.Join(dir, "project.toml")
	if err := os.WriteFile(user, []byte("[intel.gopls]\nstaticcheck = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("[intel.gopls]\ndirectoryFilters = [\"-vendor\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(user, project)
	if err := store.Apply(func(st *Settings) {
		st.Intel.Gopls["staticcheck"] = true
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(user)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "staticcheck = true") {
		t.Fatalf("user file:\n%s", text)
	}
	if strings.Contains(text, "directoryFilters") || strings.Contains(text, "vendor") {
		t.Fatalf("project keys copied into the user file:\n%s", text)
	}
	cur := store.Current()
	if cur.Intel.Gopls["staticcheck"] != true {
		t.Fatalf("staticcheck: %#v", cur.Intel.Gopls["staticcheck"])
	}
	filters, ok := cur.Intel.Gopls["directoryFilters"].([]string)
	if !ok || len(filters) != 1 || filters[0] != "-vendor" {
		t.Fatalf("filters: %#v", cur.Intel.Gopls["directoryFilters"])
	}
}

func TestApplyLeavesStoreUnchangedWhenWriteFails(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "settings.toml")
	if err := os.Mkdir(user, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewStore(user, "")
	called := false
	store.OnChange(func(Settings) { called = true })
	err := store.Apply(func(st *Settings) {
		st.Editor.FontSize = 20
	})
	if err == nil {
		t.Fatal("expected write error")
	}
	if called {
		t.Fatal("OnChange ran after a failed write")
	}
	if got := store.Current().Editor.FontSize; got != 14 {
		t.Fatalf("font size %v", got)
	}
	if _, err := os.Stat(user + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
}

func TestWriteAtomicRemovesTempAfterWriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := path + ".tmp"
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("next")); err == nil {
		t.Fatal("expected write error")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "ok" {
		t.Fatalf("settings file %q: %v", b, err)
	}
}

func TestApplyAndReloadAgree(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.toml")
	if err := os.WriteFile(user, []byte("[editor]\nfont_size = 14\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(user, "")
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := store.Apply(func(st *Settings) {
				st.Editor.FontSize = 16
			}); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			store.Reload()
		}()
	}
	wg.Wait()
	if got := store.Current().Editor.FontSize; got != 16 {
		t.Fatalf("font size %v", got)
	}
	b, err := os.ReadFile(user)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "font_size = 16") {
		t.Fatalf("user file:\n%s", b)
	}
}
