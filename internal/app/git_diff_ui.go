package app

import (
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/ui"
)

// gitRowTag is the pointer target for one Git status row.
func (s *shell) gitRowTag(i int) event.Tag {
	if i < 0 || i >= len(s.gitFiles) {
		return nil
	}
	return s.tagFor("git:" + s.gitFiles[i].Path)
}

// showsGitDiff reports whether this leaf paints the git diff
// instead of its buffer. Other splits keep their buffers.
func (s *shell) showsGitDiff(p *pane) bool {
	if s.gitDiffPath == "" || p == nil {
		return false
	}
	if s.focusPane != nil {
		return p == s.focusPane
	}
	return p == s.rootPane
}

// layoutGitDiff paints the selected path and its side-by-side diff.
// Close returns the well to the buffer.
func (s *shell) layoutGitDiff(gtx layout.Context) layout.Dimensions {
	th := s.theme
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(8), unit.Dp(8), unit.Dp(8), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
						return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), s.gitDiffPath)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(6), unit.Dp(8), unit.Dp(6), 0, func(gtx layout.Context) layout.Dimensions {
						dims, hit := ui.Click(gtx, s.tagFor("gitdiff-close"), th.Color("ui.border"), func(gtx layout.Context) layout.Dimensions {
							return ui.Inset(gtx, unit.Dp(4), unit.Dp(10), unit.Dp(4), unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
								return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), "Close")
							})
						})
						if hit {
							s.clearGitDiff()
						}
						return dims
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.HBar(gtx, th.Color("ui.border"))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(s.gitDiffRows) == 0 {
				return layout.Dimensions{}
			}
			return layout.Flex{}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(4), unit.Dp(8), unit.Dp(4), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
						return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(12), th.Color("ui.dim"), "HEAD")
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.VBar(gtx, th.Color("ui.border"))
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(4), unit.Dp(8), unit.Dp(4), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
						return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(12), th.Color("ui.dim"), "Working tree")
					})
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(s.gitDiffRows) == 0 {
				msg := s.gitDiffNote
				if msg == "" {
					msg = "Diff…"
				}
				return ui.Inset(gtx, unit.Dp(12), unit.Dp(12), unit.Dp(12), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
					return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.dim"), msg)
				})
			}
			rows := make([]editor.DiffRow, len(s.gitDiffRows))
			for i, row := range s.gitDiffRows {
				rows[i] = editor.DiffRow{Left: editorDiffSide(row.Left), Right: editorDiffSide(row.Right)}
			}
			dims, x, y := editor.LayoutDiff(gtx, s.shaper, s.editorStyle(false), rows, s.gitDiffX, s.gitDiffY, th.Color("ui.dim"), s.tagFor("gitdiff"))
			s.gitDiffX, s.gitDiffY = x, y
			return dims
		}),
	)
}

func editorDiffSide(side bus.GitDiffSide) editor.DiffSide {
	var spans []editor.DiffSpan
	if len(side.Spans) > 0 {
		spans = make([]editor.DiffSpan, len(side.Spans))
		for i, sp := range side.Spans {
			spans[i] = editor.DiffSpan{Start: sp.Start, End: sp.End, Kind: sp.Kind}
		}
	}
	return editor.DiffSide{Line: side.Line, Text: side.Text, Kind: side.Kind, Spans: spans}
}

// diffKey scrolls the open diff. Other keys are left for the caller.
func (s *shell) diffKey(st keymap.Stroke) bool {
	if st.Mod != 0 {
		return false
	}
	const line = 40
	const page = 240
	switch st.Key {
	case "up":
		s.gitDiffY -= line
	case "down":
		s.gitDiffY += line
	case "left":
		s.gitDiffX -= line
	case "right":
		s.gitDiffX += line
	case "pageup":
		s.gitDiffY -= page
	case "pagedown":
		s.gitDiffY += page
	case "home":
		s.gitDiffX = 0
		s.gitDiffY = 0
	case "end":
		s.gitDiffY = 1 << 30
	default:
		return false
	}
	if s.gitDiffX < 0 {
		s.gitDiffX = 0
	}
	if s.gitDiffY < 0 {
		s.gitDiffY = 0
	}
	return true
}
