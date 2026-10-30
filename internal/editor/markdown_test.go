package editor

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseMarkdown(t *testing.T) {
	src := "" +
		"# Title\n" +
		"\n" +
		"Hello `code` and *em* and **strong** and [a](https://example.com).\n" +
		"\n" +
		"```go\n" +
		"func f() {}\n" +
		"```\n" +
		"\n" +
		"- one\n" +
		"* two\n" +
		"1. three\n" +
		"\n" +
		"---\n" +
		"\n" +
		"<div>literal</div>\n" +
		"| a | b |\n" +
		"![alt](pic.png)\n" +
		"\n" +
		"tab\tstop\n"
	blocks := ParseMarkdown([]byte(src))
	want := []BlockKind{
		BlockHeading,
		BlockBlank,
		BlockParagraph,
		BlockBlank,
		BlockCode,
		BlockBlank,
		BlockList,
		BlockList,
		BlockList,
		BlockBlank,
		BlockRule,
		BlockBlank,
		BlockParagraph,
		BlockBlank,
		BlockParagraph,
	}
	if len(blocks) != len(want) {
		kinds := make([]BlockKind, len(blocks))
		for i, b := range blocks {
			kinds[i] = b.Kind
		}
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	for i, k := range want {
		if blocks[i].Kind != k {
			t.Fatalf("block %d kind %d, want %d", i, blocks[i].Kind, k)
		}
	}
	if blocks[0].Level != 1 || textOf(blocks[0]) != "Title" {
		t.Fatalf("heading %+v", blocks[0])
	}
	p := blocks[2]
	if !hasInline(p, InlineCode, "code") || !hasInline(p, InlineEm, "em") || !hasInline(p, InlineStrong, "strong") {
		t.Fatalf("paragraph inlines %+v", p.Inlines)
	}
	if !hasInline(p, InlineLink, "a") || p.Inlines[len(p.Inlines)-1].Href != "https://example.com" && !linkHref(p, "https://example.com") {
		t.Fatalf("link %+v", p.Inlines)
	}
	code := blocks[4]
	if code.Lang != "go" || len(code.Lines) != 1 || code.Lines[0] != "func f() {}" {
		t.Fatalf("fence %+v", code)
	}
	if blocks[6].Marker != "-" || textOf(blocks[6]) != "one" {
		t.Fatalf("list %+v", blocks[6])
	}
	if blocks[8].Marker != "1." || textOf(blocks[8]) != "three" {
		t.Fatalf("ordered %+v", blocks[8])
	}
	literal := textOf(blocks[12])
	if !strings.Contains(literal, "<div>literal</div>") || !strings.Contains(literal, "| a | b |") || !strings.Contains(literal, "alt") {
		t.Fatalf("literal %q", literal)
	}
	if strings.Contains(literal, "pic.png") && strings.Contains(literal, "![") {
		t.Fatalf("image markup kept %q", literal)
	}
	tabbed := textOf(blocks[14])
	if strings.Contains(tabbed, "\t") || !strings.Contains(tabbed, "tab") {
		t.Fatalf("tab text %q", tabbed)
	}
	for _, b := range blocks {
		for _, s := range shapedOf(b) {
			for _, r := range s {
				if r < 0x20 || r == 0x7f {
					t.Fatalf("control %U in %q", r, s)
				}
			}
		}
	}
}

func TestMarkdownPlainDropsControls(t *testing.T) {
	got := markdownPlain("a\tb\x00c\r", 4)
	if strings.ContainsAny(got, "\t\r") || strings.IndexByte(got, 0) >= 0 {
		t.Fatalf("plain %q", got)
	}
	if got != "a   bc" {
		t.Fatalf("plain %q", got)
	}
	for i := 0; i < len(got); {
		r, n := utf8.DecodeRuneInString(got[i:])
		if r < 0x20 {
			t.Fatalf("rune %U", r)
		}
		i += n
	}
}

func textOf(b Block) string {
	var s string
	for _, in := range b.Inlines {
		s += in.Text
	}
	return s
}

func hasInline(b Block, kind InlineKind, text string) bool {
	for _, in := range b.Inlines {
		if in.Kind == kind && in.Text == text {
			return true
		}
	}
	return false
}

func linkHref(b Block, href string) bool {
	for _, in := range b.Inlines {
		if in.Kind == InlineLink && in.Href == href {
			return true
		}
	}
	return false
}

func shapedOf(b Block) []string {
	out := []string{b.Marker, b.Lang}
	for _, in := range b.Inlines {
		out = append(out, in.Text)
	}
	out = append(out, b.Lines...)
	return out
}
