package app

import (
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/ui"
)

// isMarkdownPath reports paths the editor opens as a markdown preview.
func isMarkdownPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}

func (s *shell) markdownPreview() bool {
	t := s.activeTab()
	return t != nil && t.preview && isMarkdownPath(t.view.Doc.Path())
}

// previewKey scrolls the preview and swallows keys that would edit the buffer.
// Commands are resolved before this runs, so save, undo, and redo still work.
func (s *shell) previewKey(st keymap.Stroke) {
	t := s.activeTab()
	if t == nil {
		return
	}
	const line = 40
	const page = 240
	switch st.Key {
	case "up":
		t.previewY -= line
	case "down":
		t.previewY += line
	case "pageup":
		t.previewY -= page
	case "pagedown":
		t.previewY += page
	case "home":
		t.previewY = 0
	case "end":
		t.previewY = 1 << 30
	default:
		return
	}
	if t.previewY < 0 {
		t.previewY = 0
	}
}

// layoutMarkdown paints the preview or the raw editor, with a control at the
// top right that switches. The control names the mode it switches to.
func (s *shell) layoutMarkdown(gtx layout.Context, p *pane, tab *edTab) layout.Dimensions {
	th := s.theme
	focused := s.focus == focusEditor && s.popup == "" && s.focusPane == p
	return layout.Stack{Alignment: layout.NE}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if tab.preview {
				st := s.editorStyle(false)
				dims, y := editor.LayoutMarkdown(gtx, s.shaper, st, tab.view.Doc.Snapshot().Bytes(), tab.previewY,
					th.Color("ui.accent"), th.Color("ui.dim"), th.Color("ui.panel"), &tab.previewTag)
				tab.previewY = y
				return dims
			}
			s.requestSyntax(tab)
			tab.view.SetMarks(s.marks[tab.view.Doc.ID()])
			return tab.view.Layout(gtx, s.shaper, s.editorStyle(focused), s.spansFor(tab.view))
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return s.layoutMarkdownToggle(gtx, tab)
		}),
	)
}

func (s *shell) layoutMarkdownToggle(gtx layout.Context, tab *edTab) layout.Dimensions {
	th := s.theme
	label := "Text"
	if !tab.preview {
		label = "Preview"
	}
	return ui.Inset(gtx, unit.Dp(8), unit.Dp(8), 0, 0, func(gtx layout.Context) layout.Dimensions {
		bg := th.Color("ui.border")
		fg := th.Color("ui.foreground")
		dims, hit := ui.Click(gtx, &tab.previewBtn, bg, func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(4), unit.Dp(10), unit.Dp(4), unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), fg, label)
			})
		})
		if hit {
			tab.preview = !tab.preview
		}
		return dims
	})
}
