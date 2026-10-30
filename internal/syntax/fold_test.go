package syntax

import (
	"strconv"
	"strings"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
)

func TestGoFolds(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		present []bus.FoldRange
		// noStart lines must not be the Start of any fold.
		noStart []int
		// noBrace lines must not start a fold whose Suffix is "}".
		noBrace []int
	}{
		{
			name: "if-else",
			src: lines(
				"package p",
				"",
				"func f() {",
				"\tif x {",
				"\t\treturn",
				"\t} else {",
				"\t\treturn",
				"\t}",
				"}",
			),
			present: []bus.FoldRange{
				{Start: 2, End: 8, Suffix: "}"},
				{Start: 3, End: 5, Suffix: "}"},
				{Start: 5, End: 7, Suffix: "}"},
			},
		},
		{
			name: "import",
			src: lines(
				"package p",
				"",
				"import (",
				"\t\"fmt\"",
				"\t\"os\"",
				")",
			),
			present: []bus.FoldRange{
				{Start: 2, End: 5, Suffix: ")"},
			},
		},
		{
			name: "struct",
			src: lines(
				"package p",
				"",
				"type T struct {",
				"\tA int",
				"}",
			),
			present: []bus.FoldRange{
				{Start: 2, End: 4, Suffix: "}"},
			},
		},
		{
			name: "raw-string",
			src: lines(
				"package p",
				"",
				"func f() {",
				"\ts := `foo",
				"\t}`",
				"}",
			),
			// The func block still closes on the real brace. The } inside
			// the raw string (line 4) is not a fold header.
			present: []bus.FoldRange{
				{Start: 2, End: 5, Suffix: "}"},
			},
			noStart: []int{4},
		},
		{
			name: "line-comments",
			src: lines(
				"package p",
				"",
				"func f() {",
				"\t// one",
				"\t// two",
				"\treturn",
				"}",
			),
			present: []bus.FoldRange{
				{Start: 3, End: 4, Suffix: ""},
			},
		},
		{
			name: "block-comment",
			src: lines(
				"package p",
				"",
				"func f() {",
				"\t/*",
				"\t}",
				"\t*/",
				"\treturn",
				"}",
			),
			present: []bus.FoldRange{
				{Start: 2, End: 7, Suffix: "}"},
				{Start: 3, End: 5, Suffix: "*/"},
			},
			// Line 4 is only the brace inside the comment.
			noBrace: []int{4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goFoldRanges(t, tt.src)
			requireFolds(t, got, tt.present)
			forbidStart(t, got, tt.noStart...)
			for _, line := range tt.noBrace {
				if foldSuffixAt(got, line, "}") {
					t.Errorf("suffix %q starts on line %d\ngot:\n%s", "}", line, dumpFolds(got))
				}
			}
		})
	}
}

func TestHTMLFolds(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		want     []bus.FoldRange
		noSuffix []string
	}{
		{
			name: "nested",
			src: lines(
				"<div>",
				"<section>",
				"<p>hi</p>",
				"</section>",
				"</div>",
			),
			want: []bus.FoldRange{
				{Start: 0, End: 4, Suffix: "</div>"},
				{Start: 1, End: 3, Suffix: "</section>"},
			},
		},
		{
			name: "void",
			src: lines(
				"<div>",
				"<br>",
				"<img src=\"x\">",
				"</div>",
			),
			want: []bus.FoldRange{
				{Start: 0, End: 3, Suffix: "</div>"},
			},
			noSuffix: []string{"</br>", "</img>"},
		},
		{
			name: "unclosed-p",
			src: lines(
				"<div>",
				"<p>hi",
				"</div>",
			),
			want: []bus.FoldRange{
				{Start: 0, End: 2, Suffix: "</div>"},
			},
			noSuffix: []string{"</p>"},
		},
		{
			name: "mismatched",
			src: lines(
				"<div>",
				"<span>",
				"</div>",
			),
			want: []bus.FoldRange{
				{Start: 0, End: 2, Suffix: "</div>"},
			},
			noSuffix: []string{"</span>"},
		},
		{
			name: "script",
			src: lines(
				"<script>",
				"function f() {",
				"  return 1",
				"}",
				"</script>",
			),
			want: []bus.FoldRange{
				{Start: 0, End: 4, Suffix: "</script>"},
				{Start: 1, End: 3, Suffix: "}"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeFolds("html", []byte(tt.src), nil)
			exactFolds(t, got, tt.want)
			forbidSuffix(t, got, tt.noSuffix...)
		})
	}
}

func TestJavaScriptFolds(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		brace   bus.FoldRange
		noStart []int
	}{
		{
			name: "string-comment-template",
			src: lines(
				"function f() {",
				"  var s = \"{\"",
				"  // }",
				"  var t = `foo ${1} bar`",
				"  return 1",
				"}",
			),
			brace:   bus.FoldRange{Start: 0, End: 5, Suffix: "}"},
			noStart: []int{1, 2, 3},
		},
		{
			name: "multiline-template",
			src: lines(
				"function f() {",
				"  var t = `foo",
				"  ${1}",
				"  bar`",
				"  return 1",
				"}",
			),
			brace:   bus.FoldRange{Start: 0, End: 5, Suffix: "}"},
			noStart: []int{2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeFolds("javascript", []byte(tt.src), nil)
			requireFolds(t, got, []bus.FoldRange{tt.brace})
			forbidStart(t, got, tt.noStart...)
			for _, r := range got {
				if r.Suffix != "}" {
					continue
				}
				if r.Start != tt.brace.Start || r.End != tt.brace.End {
					t.Errorf("extra brace fold start=%d end=%d\ngot:\n%s", r.Start, r.End, dumpFolds(got))
				}
			}
		})
	}
}

func TestJSONFolds(t *testing.T) {
	src := lines(
		"{",
		"  \"a\": [",
		"    1",
		"  ]",
		"}",
	)
	got := computeFolds("json", []byte(src), nil)
	exactFolds(t, got, []bus.FoldRange{
		{Start: 0, End: 4, Suffix: "}"},
		{Start: 1, End: 3, Suffix: "]"},
	})
}

func TestOpenTextDoesNotParseGo(t *testing.T) {
	src := "func f() {\n\treturn\n}\n"
	var got []bus.FoldRange
	eng := New(func(ev bus.Event) {
		fr, ok := ev.(bus.FoldRanges)
		if !ok {
			return
		}
		got = append([]bus.FoldRange(nil), fr.Ranges...)
	})
	doc := buffer.FromBytes("note.txt", []byte(src))
	eng.Open(doc.ID(), "text", doc.Snapshot())
	eng.Flush()
	eng.Close(doc.ID())
	eng.Shutdown()
	if Live() != 0 {
		t.Fatalf("live objects = %d", Live())
	}
	if len(got) == 0 {
		t.Fatal("text open posted no folds")
	}
	for _, r := range got {
		if r.Suffix != "" {
			t.Fatalf("text fold suffix %q, want an indent fold", r.Suffix)
		}
	}
}

func TestIndentFolds(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []bus.FoldRange
	}{
		{
			name: "one-header",
			src: lines(
				"a:",
				"  b: 1",
				"  c: 2",
				"d:",
				"  e:",
				"    f: 3",
			),
			want: []bus.FoldRange{
				{Start: 0, End: 2},
				{Start: 3, End: 5},
				{Start: 4, End: 5},
			},
		},
		{
			// A tab is one level, and each space is one level, so the tab
			// on b is less indented than the eight spaces on c.
			name: "tab-one-level",
			src: lines(
				"a:",
				"\tb: 1",
				"        c: 2",
			),
			want: []bus.FoldRange{
				{Start: 0, End: 2},
				{Start: 1, End: 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeFolds("yaml", []byte(tt.src), nil)
			exactFolds(t, got, tt.want)
			for _, r := range got {
				if r.Suffix != "" {
					t.Errorf("indent fold suffix = %q, want empty\ngot:\n%s", r.Suffix, dumpFolds(got))
				}
			}
		})
	}
}

func goFoldRanges(t *testing.T, src string) []bus.FoldRange {
	t.Helper()
	var got []bus.FoldRange
	eng := New(func(ev bus.Event) {
		fr, ok := ev.(bus.FoldRanges)
		if !ok {
			return
		}
		got = append([]bus.FoldRange(nil), fr.Ranges...)
	})
	doc := buffer.FromBytes("fold.go", []byte(src))
	eng.Open(doc.ID(), "go", doc.Snapshot())
	eng.Flush()
	eng.Close(doc.ID())
	eng.Shutdown()
	if Live() != 0 {
		t.Fatalf("live objects = %d", Live())
	}
	return got
}

func lines(parts ...string) string {
	return strings.Join(parts, "\n")
}

func hasFold(ranges []bus.FoldRange, start, end int, suffix string) bool {
	for _, r := range ranges {
		if r.Start == start && r.End == end && r.Suffix == suffix {
			return true
		}
	}
	return false
}

func foldSuffixAt(ranges []bus.FoldRange, start int, suffix string) bool {
	for _, r := range ranges {
		if r.Start == start && r.Suffix == suffix {
			return true
		}
	}
	return false
}

func requireFolds(t *testing.T, got, want []bus.FoldRange) {
	t.Helper()
	for _, w := range want {
		if !hasFold(got, w.Start, w.End, w.Suffix) {
			t.Errorf("missing start=%d end=%d suffix=%q\ngot:\n%s", w.Start, w.End, w.Suffix, dumpFolds(got))
		}
	}
}

func exactFolds(t *testing.T, got, want []bus.FoldRange) {
	t.Helper()
	if len(got) == len(want) {
		match := true
		for _, w := range want {
			if !hasFold(got, w.Start, w.End, w.Suffix) {
				match = false
				break
			}
		}
		if match {
			return
		}
	}
	t.Errorf("folds mismatch\nwant:\n%s\ngot:\n%s", dumpFolds(want), dumpFolds(got))
}

func forbidStart(t *testing.T, got []bus.FoldRange, starts ...int) {
	t.Helper()
	for _, s := range starts {
		for _, r := range got {
			if r.Start == s {
				t.Errorf("fold starts on line %d (end=%d suffix=%q)\ngot:\n%s", s, r.End, r.Suffix, dumpFolds(got))
			}
		}
	}
}

func forbidSuffix(t *testing.T, got []bus.FoldRange, suffixes ...string) {
	t.Helper()
	for _, suf := range suffixes {
		for _, r := range got {
			if r.Suffix == suf {
				t.Errorf("unexpected suffix %q start=%d end=%d\ngot:\n%s", suf, r.Start, r.End, dumpFolds(got))
			}
		}
	}
}

func dumpFolds(rs []bus.FoldRange) string {
	if len(rs) == 0 {
		return "(none)\n"
	}
	var b strings.Builder
	for _, r := range rs {
		b.WriteString("{start:")
		b.WriteString(strconv.Itoa(r.Start))
		b.WriteString(" end:")
		b.WriteString(strconv.Itoa(r.End))
		b.WriteString(" suffix:")
		b.WriteString(strconv.Quote(r.Suffix))
		b.WriteString("}\n")
	}
	return b.String()
}
