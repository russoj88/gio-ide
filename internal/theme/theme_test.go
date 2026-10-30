package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinsDefineEveryKey(t *testing.T) {
	for _, th := range []*Theme{Dark(), Light()} {
		for _, k := range Keys {
			c := th.Color(k)
			if c.A == 0 && k != "editor.whitespace" {
				t.Fatalf("%s %s alpha 0", th.Name, k)
			}
		}
		if th.Color("editor.background") == th.Color("syntax.keyword") && th.Name == "dark" {
			t.Fatal("dark keyword should differ from background")
		}
	}
}

func TestLoadInheritsBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-dark.toml")
	body := `
name = "my-dark"
base = "dark"
[colors]
"editor.background" = "#000000"
"syntax.keyword" = "#ff00ffaa"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	th, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "my-dark" {
		t.Fatal(th.Name)
	}
	bg := th.Color("editor.background")
	if bg.R != 0 || bg.G != 0 || bg.B != 0 || bg.A != 255 {
		t.Fatalf("bg %#v", bg)
	}
	kw := th.Color("syntax.keyword")
	if kw.R != 255 || kw.A != 0xaa {
		t.Fatalf("kw %#v", kw)
	}
	// Inherited from dark.
	if th.Color("syntax.string") != Dark().Color("syntax.string") {
		t.Fatal("inherit")
	}
}

func TestParseColorRejects(t *testing.T) {
	if _, err := ParseColor("red"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseColor("#abc"); err == nil {
		t.Fatal("expected error")
	}
}
