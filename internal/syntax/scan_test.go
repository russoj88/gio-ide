package syntax

import (
	"strings"
	"testing"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
)

func TestLang(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"a.go", "go"},
		{"/tmp/Main.GO", "go"},
		{"go.mod", "gomod"},
		{"pkg/go.mod", "gomod"},
		{"go.sum", "gosum"},
		{"app.yaml", "yaml"},
		{"app.YML", "yaml"},
		{"app.json", "json"},
		{"readme.md", ""},
		{"notes.markdown", ""},
		{"Makefile", ""},
		{"Dockerfile", "docker"},
		{"pkg/Dockerfile.dev", "docker"},
		{"image.dockerfile", "docker"},
		{"docker-compose.yml", "yaml"},
		{".env", "env"},
		{".env.local", "env"},
		{"config/production.env", "env"},
		{".envrc", ""},
		{"app.js", "javascript"},
		{"app.mjs", "javascript"},
		{"app.cjs", "javascript"},
		{"App.JS", "javascript"},
		{"page.html", "html"},
		{"page.HTM", "html"},
		{"app.jsx", ""},
		{"query.sql", "sql"},
		{"Query.SQL", "sql"},
		{"fn.pgsql", "sql"},
		{"fn.psql", "sql"},
		{"notes.sqlite", ""},
		{"archive.sql.bak", ""},
	}
	for _, tc := range cases {
		if got := Lang(tc.path); got != tc.want {
			t.Errorf("Lang(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestScan(t *testing.T) {
	mod := "module example.com/a\n\ngo 1.22.0\n\nrequire example.com/b v1.2.3\nreplace example.com/b => example.com/b v1.2.4\n// note\n"
	sum := "example.com/b v1.2.3 h1:abcdef=\nexample.com/b v1.2.3/go.mod h1:zzzzzz=\n"
	js := "{\"name\": \"ada\", \"n\": 1, \"ok\": true}\n"
	yml := "# comment\nname: app\ncount: 3\nok: true\nquote: \"hi\"\n---\na: |\n  hello\nb: 1\n"
	multi := "a: \"hello\nworld\"\n"

	if got := Scan("go", []byte("package p\n"), 0, 1); got != nil {
		t.Fatalf("go scan = %#v, want nil", got)
	}
	if got := Scan("", []byte("package p\n"), 0, 1); got != nil {
		t.Fatalf("empty lang scan = %#v, want nil", got)
	}

	modSpans := Scan("gomod", []byte(mod), 0, lineCount([]byte(mod)))
	assertSpan(t, "gomod", mod, modSpans, "module", "syntax.keyword")
	assertSpan(t, "gomod", mod, modSpans, "1.22.0", "syntax.number")
	assertSpan(t, "gomod", mod, modSpans, "v1.2.3", "syntax.number")
	assertSpan(t, "gomod", mod, modSpans, "=>", "syntax.operator")
	assertSpan(t, "gomod", mod, modSpans, "// note", "syntax.comment")

	sumSpans := Scan("gosum", []byte(sum), 0, lineCount([]byte(sum)))
	assertSpan(t, "gosum", sum, sumSpans, "v1.2.3", "syntax.number")
	assertSpan(t, "gosum", sum, sumSpans, "h1:abcdef=", "syntax.string")
	assertSpan(t, "gosum", sum, sumSpans, "h1:zzzzzz=", "syntax.string")

	jsSpans := Scan("json", []byte(js), 0, lineCount([]byte(js)))
	assertSpan(t, "json", js, jsSpans, "\"name\"", "syntax.property")
	assertSpan(t, "json", js, jsSpans, "\"ada\"", "syntax.string")
	assertSpan(t, "json", js, jsSpans, "1", "syntax.number")
	assertSpan(t, "json", js, jsSpans, "true", "syntax.constant")

	ymlSpans := Scan("yaml", []byte(yml), 0, lineCount([]byte(yml)))
	assertSpan(t, "yaml", yml, ymlSpans, "# comment", "syntax.comment")
	assertSpan(t, "yaml", yml, ymlSpans, "name", "syntax.property")
	assertSpan(t, "yaml", yml, ymlSpans, "3", "syntax.number")
	assertSpan(t, "yaml", yml, ymlSpans, "true", "syntax.constant")
	assertSpan(t, "yaml", yml, ymlSpans, "\"hi\"", "syntax.string")
	assertSpan(t, "yaml", yml, ymlSpans, "---", "syntax.keyword")
	assertSpan(t, "yaml", yml, ymlSpans, "hello", "syntax.string")
	assertSpan(t, "yaml", yml, ymlSpans, "b", "syntax.property")

	multiSpans := Scan("yaml", []byte(multi), 0, lineCount([]byte(multi)))
	assertSpan(t, "yaml", multi, multiSpans, "\"hello", "syntax.string")
	assertSpan(t, "yaml", multi, multiSpans, "world\"", "syntax.string")

	// A window that starts on the continuation still knows the quote is open.
	tail := Scan("yaml", []byte(multi), 1, 2)
	if len(tail) != 1 {
		t.Fatalf("tail lines = %d, want 1", len(tail))
	}
	assertSpan(t, "yaml-tail", "world\"\n", tail, "world\"", "syntax.string")
}

func TestScanDockerEnvJSAndHTML(t *testing.T) {
	docker := "# comment\nFROM golang:1.22 AS build\nRUN go build \\\nFROM scratch\nCOPY --from=build /src /out\nENV PORT=8080\nEXPOSE 8080\nCMD [\"app\"]\n"
	env := "# comment\nexport PORT=8080\nNAME=\"ada\"\nURL=https://example.com/${HOST}\nKEY=\"hello\nworld\"\n"
	js := "// note\nconst n = 1;\nfunction add(a, b) {\n  return a + b;\n}\nconst s = \"hi\";\n/* block\n   comment */\nconst ok = true;\nconst t = `hello\n${name}`;\n"
	html := "<!-- note -->\n<!DOCTYPE html>\n<div class=\"box\">\n  <p>Hello &amp;</p>\n</div>\n<script>\nconst n = 1;\nconst s = \"</script>\";\nconst m = 2;\n</script>\n"

	docSpans := Scan("docker", []byte(docker), 0, lineCount([]byte(docker)))
	assertSpan(t, "docker", docker, docSpans, "# comment", "syntax.comment")
	assertSpan(t, "docker", docker, docSpans, "FROM", "syntax.keyword")
	assertSpan(t, "docker", docker, docSpans, "1.22", "syntax.number")
	assertSpan(t, "docker", docker, docSpans, "AS", "syntax.keyword")
	assertSpan(t, "docker", docker, docSpans, "RUN", "syntax.keyword")
	assertSpan(t, "docker", docker, docSpans, "--from", "syntax.property")
	assertSpan(t, "docker", docker, docSpans, "8080", "syntax.number")
	assertSpan(t, "docker", docker, docSpans, "\"app\"", "syntax.string")
	assertNoSpan(t, "docker", docker, docSpans, "FROM scratch", "syntax.keyword")

	envSpans := Scan("env", []byte(env), 0, lineCount([]byte(env)))
	assertSpan(t, "env", env, envSpans, "# comment", "syntax.comment")
	assertSpan(t, "env", env, envSpans, "export", "syntax.keyword")
	assertSpan(t, "env", env, envSpans, "PORT", "syntax.property")
	assertSpan(t, "env", env, envSpans, "=", "syntax.operator")
	assertSpan(t, "env", env, envSpans, "\"ada\"", "syntax.string")
	assertSpan(t, "env", env, envSpans, "${HOST}", "syntax.constant")
	assertSpan(t, "env", env, envSpans, "\"hello", "syntax.string")
	assertSpan(t, "env", env, envSpans, "world\"", "syntax.string")

	jsSpans := Scan("javascript", []byte(js), 0, lineCount([]byte(js)))
	assertSpan(t, "js", js, jsSpans, "// note", "syntax.comment")
	assertSpan(t, "js", js, jsSpans, "const", "syntax.keyword")
	assertSpan(t, "js", js, jsSpans, "1", "syntax.number")
	assertSpan(t, "js", js, jsSpans, "function", "syntax.keyword")
	assertSpan(t, "js", js, jsSpans, "add", "syntax.function")
	assertSpan(t, "js", js, jsSpans, "return", "syntax.keyword")
	assertSpan(t, "js", js, jsSpans, "\"hi\"", "syntax.string")
	assertSpan(t, "js", js, jsSpans, "block", "syntax.comment")
	assertSpan(t, "js", js, jsSpans, "comment", "syntax.comment")
	assertSpan(t, "js", js, jsSpans, "true", "syntax.constant")
	assertSpan(t, "js", js, jsSpans, "hello", "syntax.string")
	assertSpan(t, "js", js, jsSpans, "${", "syntax.operator")

	// A window that starts inside the block comment still knows it is open.
	// "   comment */" is the 8th line (index 7) of js.
	tail := Scan("javascript", []byte(js), 7, 8)
	assertSpan(t, "js-tail", "   comment */\n", tail, "comment", "syntax.comment")

	htmlSpans := Scan("html", []byte(html), 0, lineCount([]byte(html)))
	assertSpan(t, "html", html, htmlSpans, "<!-- note -->", "syntax.comment")
	assertSpan(t, "html", html, htmlSpans, "<!DOCTYPE html>", "syntax.keyword")
	assertSpan(t, "html", html, htmlSpans, "div", "syntax.keyword")
	assertSpan(t, "html", html, htmlSpans, "class", "syntax.property")
	assertSpan(t, "html", html, htmlSpans, "\"box\"", "syntax.string")
	assertSpan(t, "html", html, htmlSpans, "&amp;", "syntax.constant")
	assertSpan(t, "html", html, htmlSpans, "const", "syntax.keyword")
	assertSpan(t, "html", html, htmlSpans, "1", "syntax.number")
	assertSpan(t, "html", html, htmlSpans, "2", "syntax.number")
	assertSpan(t, "html", html, htmlSpans, "\"</script>\"", "syntax.string")
}

func TestSQLWord(t *testing.T) {
	cases := []struct {
		word  string
		style string
	}{
		{"select", "syntax.keyword"},
		{"SELECT", "syntax.keyword"},
		{"From", "syntax.keyword"},
		{"perform", "syntax.keyword"},
		{"foreach", "syntax.keyword"},
		{"while", "syntax.keyword"},
		{"plpgsql", "syntax.keyword"},
		{"true", "syntax.constant"},
		{"FALSE", "syntax.constant"},
		{"null", "syntax.constant"},
		{"int", "syntax.type"},
		{"timestamptz", "syntax.type"},
		{"jsonb", "syntax.type"},
		{"varchar", "syntax.type"},
		{"date", "syntax.type"},
		{"count", ""},
		{"selecting", ""},
		{"performance", ""},
		{"found", ""},
	}
	for _, tc := range cases {
		if got := sqlWord(tc.word); got != tc.style {
			t.Errorf("sqlWord(%q) = %q, want %q", tc.word, got, tc.style)
		}
	}
}

func TestScanSQL(t *testing.T) {
	src := `-- select stays a comment
SELECT id FROM users WHERE active = TRUE;
selecting := 1;
SELECT x::int, y::date, 1_000, 0xFF, .5, 1e-3, $1, 1..10;
SELECT a <> b AND c -> 'k' OR d || e;
SELECT 'it''s', E'a\'b', B'0101', U&'snow', "a""b";
SELECT 'hello
world';
/* outer /* inner */ still */
/* block
   still */
CREATE FUNCTION f() RETURNS int LANGUAGE plpgsql AS $$
BEGIN
  PERFORM x;
  RETURN 1;
END;
$$;
SELECT count(1), performance;
+-- note
$Ab$ inside $ab$
`
	got := Scan("sql", []byte(src), 0, lineCount([]byte(src)))
	parts := strings.Split(src, "\n")
	if len(got) != len(parts) {
		t.Fatalf("sql lines = %d, text lines = %d", len(got), len(parts))
	}
	for i, line := range parts {
		prev := 0
		for _, sp := range got[i] {
			if sp.Start < prev || sp.End > len(line) || sp.Start >= sp.End || sp.Style == "" {
				t.Fatalf("line %d %q span %+v", i, line, sp)
			}
			prev = sp.End
		}
	}

	assertSpan(t, "sql", src, got, "stays", "syntax.comment")
	assertNoSpan(t, "sql", src, got, "stays", "syntax.keyword")
	assertSpan(t, "sql", src, got, "SELECT", "syntax.keyword")
	assertSpan(t, "sql", src, got, "FROM", "syntax.keyword")
	assertSpan(t, "sql", src, got, "WHERE", "syntax.keyword")
	assertSpan(t, "sql", src, got, "TRUE", "syntax.constant")
	assertNoSpan(t, "sql", src, got, "users", "syntax.keyword")
	assertNoSpan(t, "sql", src, got, "active", "syntax.keyword")
	assertNoSpan(t, "sql", src, got, "selecting", "syntax.keyword")
	assertSpan(t, "sql", src, got, ":=", "syntax.operator")
	assertSpan(t, "sql", src, got, "::", "syntax.operator")
	assertSpan(t, "sql", src, got, "int", "syntax.type")
	assertSpan(t, "sql", src, got, "date", "syntax.type")
	assertSpan(t, "sql", src, got, "1_000", "syntax.number")
	assertSpan(t, "sql", src, got, "0xFF", "syntax.number")
	assertSpan(t, "sql", src, got, ".5", "syntax.number")
	assertSpan(t, "sql", src, got, "1e-3", "syntax.number")
	assertSpan(t, "sql", src, got, "$1", "syntax.number")
	assertSpan(t, "sql", src, got, "..", "syntax.operator")
	assertSpan(t, "sql", src, got, "<>", "syntax.operator")
	assertSpan(t, "sql", src, got, "->", "syntax.operator")
	assertSpan(t, "sql", src, got, "||", "syntax.operator")
	assertSpan(t, "sql", src, got, "AND", "syntax.keyword")
	assertSpan(t, "sql", src, got, "OR", "syntax.keyword")
	assertSpan(t, "sql", src, got, "'it''s'", "syntax.string")
	assertSpan(t, "sql", src, got, `E'a\'b'`, "syntax.string")
	assertSpan(t, "sql", src, got, "B'0101'", "syntax.string")
	assertSpan(t, "sql", src, got, "U&'snow'", "syntax.string")
	assertSpan(t, "sql", src, got, `"a""b"`, "syntax.property")
	assertSpan(t, "sql", src, got, "hello", "syntax.string")
	assertSpan(t, "sql", src, got, "world'", "syntax.string")
	assertSpan(t, "sql", src, got, "still", "syntax.comment")
	assertSpan(t, "sql", src, got, "FUNCTION", "syntax.keyword")
	assertSpan(t, "sql", src, got, "plpgsql", "syntax.keyword")
	assertSpan(t, "sql", src, got, "$$", "syntax.string")
	assertSpan(t, "sql", src, got, "BEGIN", "syntax.keyword")
	assertSpan(t, "sql", src, got, "PERFORM", "syntax.keyword")
	assertSpan(t, "sql", src, got, "RETURN", "syntax.keyword")
	assertNoSpan(t, "sql", src, got, "count", "syntax.keyword")
	assertNoSpan(t, "sql", src, got, "performance", "syntax.keyword")
	assertSpan(t, "sql", src, got, "-- note", "syntax.comment")
	assertSpan(t, "sql", src, got, "+", "syntax.operator")
	assertSpan(t, "sql", src, got, "inside", "syntax.string")
	assertNoSpan(t, "sql", src, got, "inside", "syntax.keyword")

	var strLine int
	var strText string
	var comLine int
	var comText string
	for i, line := range parts {
		if line == "world';" {
			strLine = i
			strText = line
		}
		if line == "   still */" {
			comLine = i
			comText = line
		}
	}
	if strText == "" || comText == "" {
		t.Fatal("missing continuation lines")
	}
	strTail := Scan("sql", []byte(src), strLine, strLine+1)
	assertSpan(t, "sql-str", strText, strTail, "world'", "syntax.string")
	comTail := Scan("sql", []byte(src), comLine, comLine+1)
	assertSpan(t, "sql-comment", comText, comTail, "still", "syntax.comment")

	nest := "$fn$ SELECT $body$ hello $body$ $fn$\n"
	nestSpans := Scan("sql", []byte(nest), 0, lineCount([]byte(nest)))
	assertSpan(t, "sql-nest", nest, nestSpans, "SELECT", "syntax.keyword")
	assertSpan(t, "sql-nest", nest, nestSpans, "$body$", "syntax.string")
	assertNoSpan(t, "sql-nest", nest, nestSpans, "hello", "syntax.string")

	ops := "a *- b\na @- b\n"
	opSpans := Scan("sql", []byte(ops), 0, lineCount([]byte(ops)))
	assertSpan(t, "sql-op", ops, opSpans, "*", "syntax.operator")
	assertSpan(t, "sql-op", ops, opSpans, "-", "syntax.operator")
	assertSpan(t, "sql-op", ops, opSpans, "@-", "syntax.operator")
	if len(opSpans) < 2 || len(opSpans[0]) != 2 || len(opSpans[1]) != 1 {
		t.Fatalf("operator spans = %+v", opSpans)
	}
}

func assertNoSpan(t *testing.T, lang, text string, lines [][]StyleSpan, sub, style string) {
	t.Helper()
	for li, line := range strings.Split(text, "\n") {
		if li >= len(lines) {
			break
		}
		at := strings.Index(line, sub)
		if at < 0 {
			continue
		}
		end := at + len(sub)
		for _, sp := range lines[li] {
			if sp.Style == style && sp.Start < end && sp.End > at {
				t.Errorf("%s: %q on line %d covered by %s in %+v", lang, sub, li, style, lines[li])
				return
			}
		}
	}
}

func TestEngineScanPostsSpans(t *testing.T) {
	text := []byte("{\"name\": \"ada\"}\n")
	var got bus.SyntaxSpans
	eng := New(func(ev bus.Event) {
		if sp, ok := ev.(bus.SyntaxSpans); ok {
			got = sp
		}
	})
	doc := buffer.FromBytes("a.json", text)
	eng.Open(doc.ID(), Lang(doc.Path()), doc.Snapshot())
	eng.RequestSpans(doc.ID(), doc.Version(), 0, doc.Snapshot().LineCount())
	eng.Flush()
	eng.Close(doc.ID())
	eng.Shutdown()
	if Live() != 0 {
		t.Fatalf("live objects = %d after shutdown", Live())
	}
	formatted := formatSpans(got)
	if !strings.Contains(formatted, "syntax.property") || !strings.Contains(formatted, "syntax.string") {
		t.Fatalf("spans:\n%s", formatted)
	}
}

func assertSpan(t *testing.T, lang, text string, lines [][]StyleSpan, sub, style string) {
	t.Helper()
	off := 0
	for li, line := range strings.Split(text, "\n") {
		if li >= len(lines) {
			break
		}
		at := strings.Index(line, sub)
		if at >= 0 {
			end := at + len(sub)
			for _, sp := range lines[li] {
				if sp.Style == style && sp.Start <= at && sp.End >= end && sp.Start >= 0 && sp.End <= len(line) {
					return
				}
			}
			t.Errorf("%s: %q on line %d not covered by %s in %+v", lang, sub, li, style, lines[li])
			return
		}
		off += len(line) + 1
	}
	_ = off
	t.Errorf("%s: %q not found", lang, sub)
}
