package app

import (
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/ui"
)

func (s *shell) openFind(replace bool) {
	s.findOn = true
	s.findReplace = replace
	s.findField = 0
	s.focus = focusFind
	if v := s.activeView(); v != nil {
		start, _ := v.SelectionBounds()
		s.findOrigin = start
		if v.HasSelection() && !stringsContainNL(v.SelectedText()) {
			s.findQuery = v.SelectedText()
			s.findCaret = utf8.RuneCountInString(s.findQuery)
		}
	}
	s.applyFind(1, true)
}

func stringsContainNL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return true
		}
	}
	return false
}

func (s *shell) findMove(dir int) {
	if !s.findOn {
		s.openFind(false)
		return
	}
	s.applyFind(dir, false)
}

// applyFind selects a match. fromOrigin restarts at the caret from openFind.
// Find next and find previous keep the current hit and move from there.
func (s *shell) applyFind(dir int, fromOrigin bool) {
	v := s.activeView()
	if v == nil || s.findQuery == "" {
		return
	}
	if fromOrigin {
		origin := buffer.Offset(s.findOrigin)
		v.SetSelections([]editor.Sel{{Anchor: origin, Head: origin}})
	}
	if err := v.FindNext(s.findQuery, s.findRegex, dir); err != nil {
		s.notify(err.Error())
	}
}

func (s *shell) findInput(st keymap.Stroke) {
	if st.Key == "escape" && st.Mod == 0 {
		s.findOn = false
		s.focus = focusEditor
		return
	}
	if st.Key == "enter" && st.Mod == 0 {
		if s.findReplace && s.findField == 1 {
			s.replaceOne()
			return
		}
		s.applyFind(1, false)
		return
	}
	if st.Key == "tab" && st.Mod == 0 && s.findReplace {
		s.findField = 1 - s.findField
		s.findCaret = utf8.RuneCountInString(s.activeFindText())
		return
	}
	text, caret := s.activeFindText(), s.findCaret
	runes := []rune(text)
	if caret > len(runes) {
		caret = len(runes)
	}
	switch st.Key {
	case "backspace":
		if st.Mod == 0 && caret > 0 {
			runes = append(runes[:caret-1], runes[caret:]...)
			caret--
		}
	case "delete":
		if st.Mod == 0 && caret < len(runes) {
			runes = append(runes[:caret], runes[caret+1:]...)
		}
	case "left":
		if caret > 0 {
			caret--
		}
	case "right":
		if caret < len(runes) {
			caret++
		}
	case "home":
		caret = 0
	case "end":
		caret = len(runes)
	default:
		if in := insertText(st); in != "" {
			runes = append(runes[:caret], append([]rune(in), runes[caret:]...)...)
			caret += len([]rune(in))
		} else {
			return
		}
	}
	s.findCaret = caret
	if s.findField == 0 {
		s.findQuery = string(runes)
		s.applyFind(1, true)
	} else {
		s.findRepl = string(runes)
	}
}

func (s *shell) activeFindText() string {
	if s.findField == 1 {
		return s.findRepl
	}
	return s.findQuery
}

func (s *shell) replaceOne() {
	v := s.activeView()
	if v == nil {
		return
	}
	if err := v.ReplaceCurrent(s.findQuery, s.findRepl, s.findRegex); err != nil {
		s.notify(err.Error())
	}
}

func (s *shell) replaceAll() {
	v := s.activeView()
	if v == nil {
		return
	}
	n, err := v.ReplaceAll(s.findQuery, s.findRepl, s.findRegex)
	if err != nil {
		s.notify(err.Error())
		return
	}
	s.notify(itoaSmall(n) + " replaced")
}

func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func (s *shell) layoutFindBar(gtx layout.Context) layout.Dimensions {
	if !s.findOn {
		return layout.Dimensions{}
	}
	th := s.theme
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.HBar(gtx, th.Color("ui.border"))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(32))
					return ui.Fill(gtx, th.Color("ui.panel"))
				},
				func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(4), unit.Dp(8), unit.Dp(4), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								label := "Find  " + showCaret(s.findQuery, s.findCaretIf(0))
								return ui.Text(gtx, s.shaper, ui.MonoFont, unit.Sp(13), th.Color("editor.foreground"), label)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								bg := th.Color("ui.panel")
								if s.findRegex {
									bg = th.Color("ui.selection")
								}
								dims, hit := ui.Click(gtx, s.tagFor("find:regex"), bg, func(gtx layout.Context) layout.Dimensions {
									return ui.Inset(gtx, unit.Dp(4), unit.Dp(8), unit.Dp(4), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
										return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(12), th.Color("ui.foreground"), ".*")
									})
								})
								if hit {
									s.findRegex = !s.findRegex
									s.applyFind(1, true)
								}
								return dims
							}),
						)
					})
				},
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.findReplace {
				return layout.Dimensions{}
			}
			return ui.Inset(gtx, unit.Dp(4), unit.Dp(8), unit.Dp(4), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						label := "Replace  " + showCaret(s.findRepl, s.findCaretIf(1))
						return ui.Text(gtx, s.shaper, ui.MonoFont, unit.Sp(13), th.Color("editor.foreground"), label)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims, hit := ui.Click(gtx, s.tagFor("find:all"), th.Color("ui.selection"), func(gtx layout.Context) layout.Dimensions {
							return ui.Inset(gtx, unit.Dp(4), unit.Dp(8), unit.Dp(4), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
								return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(12), th.Color("ui.foreground"), "All")
							})
						})
						if hit {
							s.replaceAll()
						}
						return dims
					}),
				)
			})
		}),
	)
}

func (s *shell) findCaretIf(field int) int {
	if s.focus == focusFind && s.findField == field {
		return s.findCaret
	}
	if field == 0 {
		return len([]rune(s.findQuery))
	}
	return len([]rune(s.findRepl))
}
