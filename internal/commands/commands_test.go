package commands

import "testing"

func TestCatalogUniqueAndExecute(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Catalog {
		if c.ID == "" || c.Title == "" {
			t.Fatalf("empty catalog entry: %+v", c)
		}
		if seen[c.ID] {
			t.Fatal("duplicate", c.ID)
		}
		seen[c.ID] = true
	}
	for _, id := range []string{
		"file.save", "nav.goToFile", "edit.undo", "view.terminal",
		"git.commit", "intel.restart", "app.findAction", "view.toggleTheme",
		"debug.continue", "refactor.rename", "run.run", "test.run",
	} {
		if !seen[id] {
			t.Fatal("missing", id)
		}
	}
	r := New()
	var ran string
	for _, c := range Catalog {
		c.Run = func(id string) func() { return func() { ran = id } }(c.ID)
		r.Register(c)
	}
	if !r.Execute("file.save") || ran != "file.save" {
		t.Fatal(ran)
	}
	if r.Execute("no.such") {
		t.Fatal("unknown command ran")
	}
	if !IsStub("debug.launch") || IsStub("file.save") {
		t.Fatal("stub classification")
	}
}
