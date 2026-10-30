package ui

import "testing"

func TestIconKind(t *testing.T) {
	cases := []struct {
		name string
		dir  bool
		want treeIcon
	}{
		{name: "src", dir: true, want: iconFolder},
		{name: "main.go", want: iconGo},
		{name: "license.go", want: iconGo},
		{name: "foo_test.go", want: iconTest},
		{name: "Foo_Test.GO", want: iconTest},
		{name: "README.md", want: iconMarkdown},
		{name: "README", want: iconMarkdown},
		{name: "notes.markdown", want: iconMarkdown},
		{name: "LICENSE", want: iconLicense},
		{name: "LICENSE.txt", want: iconLicense},
		{name: "LICENCE.md", want: iconLicense},
		{name: "COPYING", want: iconLicense},
		{name: "index.html", want: iconHTML},
		{name: "page.htm", want: iconHTML},
		{name: "app.js", want: iconJS},
		{name: "app.jsx", want: iconJS},
		{name: "app.ts", want: iconTS},
		{name: "app.tsx", want: iconTS},
		{name: "go.mod", want: iconModule},
		{name: "go.sum", want: iconModule},
		{name: "go.work", want: iconModule},
		{name: "note.toml", want: iconTOML},
		{name: "pic.png", want: iconImage},
		{name: "pic.SVG", want: iconImage},
		{name: ".gitignore", want: iconGit},
		{name: "Makefile", want: iconPlain},
		{name: "Dockerfile", want: iconPlain},
		{name: "notes.txt", want: iconPlain},
	}
	for _, tc := range cases {
		if got := iconKind(tc.name, tc.dir); got != tc.want {
			t.Errorf("iconKind(%q, %v) = %d, want %d", tc.name, tc.dir, got, tc.want)
		}
	}
}
