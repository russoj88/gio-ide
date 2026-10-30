package intel

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/gopls/protocol"
)

func TestOffsetPositionRoundTrip(t *testing.T) {
	samples := []string{
		"ascii",
		"é",
		"你",
		"🙂",
		"aéb你🙂\nnext",
	}
	for _, s := range samples {
		m := protocol.NewMapper("file:///t.go", []byte(s))
		for off := 0; off <= len(s); {
			pos, err := m.OffsetPosition(off)
			if err != nil {
				t.Fatalf("%q offset %d: %v", s, off, err)
			}
			back, err := m.PositionOffset(pos)
			if err != nil || back != off {
				t.Fatalf("%q offset %d position %+v back %d err %v", s, off, pos, back, err)
			}
			if off == len(s) {
				break
			}
			_, size := utf8.DecodeRuneInString(s[off:])
			if size < 1 {
				t.Fatalf("bad rune at %d", off)
			}
			// A 4-byte rune is one code point and a UTF-16 surrogate pair.
			if r, _ := utf8.DecodeRuneInString(s[off:]); r >= 0x10000 {
				next, err := m.OffsetPosition(off + size)
				if err != nil {
					t.Fatal(err)
				}
				if next.Character != pos.Character+2 && next.Line == pos.Line {
					t.Fatalf("surrogate pair: %+v then %+v", pos, next)
				}
			}
			off += size
		}
	}
}

func TestLocationFromRangeSurrogate(t *testing.T) {
	text := []byte("a🙂b\n")
	uri := protocol.DocumentURI("file:///t.go")
	rng := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 3},
		End:   protocol.Position{Line: 0, Character: 4},
	}
	loc := locationFromRange(uri, text, rng)
	if loc.Offset != 5 || loc.Col != 6 || loc.EndOffset != 6 || loc.EndCol != 7 || loc.Line != 1 {
		t.Fatalf("%+v", loc)
	}
	raw := locationFromRange(uri, nil, rng)
	if raw.Col != 4 || raw.Offset != 0 {
		t.Fatalf("nil content %+v", raw)
	}
}

func TestPublishDiagnosticsByteColumn(t *testing.T) {
	text := []byte("a🙂b\n")
	uri := protocol.URIFromPath("/tmp/gio-ide-diag.go")
	var got bus.DiagnosticsUpdated
	h := &host{
		docs: map[uint64]*openDoc{
			1: {id: 1, path: uri.Path(), uri: uri, text: append([]byte(nil), text...)},
		},
		post: func(e bus.Event) {
			if d, ok := e.(bus.DiagnosticsUpdated); ok {
				got = d
			}
		},
	}
	err := (&client{h: h}).PublishDiagnostics(context.Background(), &protocol.PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []protocol.Diagnostic{{
			Range: protocol.Range{
				Start: protocol.Position{Character: 3},
				End:   protocol.Position{Character: 4},
			},
			Message:  "here",
			Severity: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("%+v", got)
	}
	it := got.Items[0]
	if it.Start != 5 || it.End != 6 || it.Col != 6 || it.Line != 1 {
		t.Fatalf("%+v", it)
	}
}
