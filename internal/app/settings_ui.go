package app

import (
	"fmt"
	"image"
	"math"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/theme"
	"github.com/russoj88/gio-ide/internal/ui"
)

const (
	settingsRowDp = 36
	settingsBtnDp = 36
)

type settingRow struct {
	label  string
	value  string
	minus  string
	plus   string
	toggle string
}

func (s *shell) layoutSettings(gtx layout.Context) layout.Dimensions {
	if s.store == nil {
		return layout.Dimensions{}
	}
	cur := s.store.Current()
	rowH := gtx.Dp(unit.Dp(settingsRowDp))
	rows := []settingRow{
		{label: "Font size", value: fmt.Sprintf("%.0f", cur.Editor.FontSize), minus: "set:font-", plus: "set:font+"},
		{label: "Cursor blink", value: blinkLabel(cur.Editor.CursorBlinkMS), minus: "set:blink-", plus: "set:blink+"},
		{label: "Line height", value: fmt.Sprintf("%.1f", cur.Editor.LineHeight), minus: "set:lh-", plus: "set:lh+"},
		{label: "Tab width", value: fmt.Sprintf("%d", max(cur.Editor.TabWidth, 0)), minus: "set:tab-", plus: "set:tab+"},
		{label: "Terminal font", value: fmt.Sprintf("%.0f", cur.Terminal.FontSize), minus: "set:term-", plus: "set:term+"},
		{label: "Indent", value: indentLabel(cur.Editor.InsertSpaces), toggle: "set:indent"},
		{label: "Whitespace", value: whitespaceLabel(cur.Editor.ShowWhitespace), toggle: "set:space"},
		{label: "gofumpt", value: onOffLabel(cur.Editor.GofumptOnSave), toggle: "set:gofumpt"},
		{label: "goimports", value: onOffLabel(cur.Editor.GoimportsOnSave), toggle: "set:goimports"},
		{label: "go fix", value: onOffLabel(cur.Editor.GofixOnSave), toggle: "set:gofix"},
		{label: "Theme", value: themeLabel(cur.UI.Theme), toggle: "set:theme"},
	}
	s.shownFont = cur.Editor.FontSize
	var kids []layout.FlexChild
	for _, row := range rows {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = rowH
			gtx.Constraints.Max.Y = rowH
			return s.layoutSettingRow(gtx, row)
		}))
	}
	kids = append(kids,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.HBar(gtx, s.theme.Color("ui.border"))
		}),
		layout.Rigid(s.layoutKeymapHeader),
		layout.Flexed(1, s.layoutKeymap),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func (s *shell) layoutSettingRow(gtx layout.Context, row settingRow) layout.Dimensions {
	th := s.theme
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, 0, unit.Dp(8), 0, 0, func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), row.label)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if row.toggle != "" {
				return s.settingButton(gtx, row.toggle, row.value, unit.Dp(96))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := max(gtx.Dp(unit.Dp(80)), 1)
					gtx.Constraints.Min.X = width
					gtx.Constraints.Max.X = width
					return ui.Inset(gtx, 0, unit.Dp(8), 0, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
						return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), row.value)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.settingButton(gtx, row.minus, "-", unit.Dp(settingsBtnDp))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.settingButton(gtx, row.plus, "+", unit.Dp(settingsBtnDp))
				}),
			)
		}),
	)
}

func (s *shell) settingButton(gtx layout.Context, tag, label string, w unit.Dp) layout.Dimensions {
	th := s.theme
	width := max(gtx.Dp(w), 1)
	height := gtx.Constraints.Max.Y
	if height < 1 {
		height = gtx.Dp(unit.Dp(settingsRowDp))
	}
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = image.Pt(width, height)
	dims, hit := ui.Click(gtx, s.tagFor(tag), th.Color("ui.selection"), func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), label)
			}),
		)
	})
	if hit {
		s.adjustSetting(tag)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func (s *shell) adjustSetting(tag string) {
	if after, ok := strings.CutPrefix(tag, "km:"); ok {
		s.keyCapture = after
		s.visualDirty = true
		return
	}
	if after, ok := strings.CutPrefix(tag, "km-reset:"); ok {
		s.resetKeyCommand(after)
		return
	}
	if tag == "km-reset-all" {
		s.resetKeymap()
		return
	}
	if s.store == nil {
		return
	}
	err := s.store.Apply(func(st *settings.Settings) {
		switch tag {
		case "set:font+":
			st.Editor.FontSize = clampFloat(st.Editor.FontSize+1, 8, 72)
		case "set:font-":
			st.Editor.FontSize = clampFloat(st.Editor.FontSize-1, 8, 72)
		case "set:blink+":
			st.Editor.CursorBlinkMS = clampInt(st.Editor.CursorBlinkMS+50, 0, 2000)
		case "set:blink-":
			st.Editor.CursorBlinkMS = clampInt(st.Editor.CursorBlinkMS-50, 0, 2000)
		case "set:lh+":
			st.Editor.LineHeight = clampFloat(roundTenth(st.Editor.LineHeight)+0.1, 1, 2.5)
		case "set:lh-":
			st.Editor.LineHeight = clampFloat(roundTenth(st.Editor.LineHeight)-0.1, 1, 2.5)
		case "set:tab+":
			st.Editor.TabWidth = clampInt(st.Editor.TabWidth+1, 1, 16)
		case "set:tab-":
			st.Editor.TabWidth = clampInt(max(st.Editor.TabWidth, 1)-1, 1, 16)
		case "set:term+":
			st.Terminal.FontSize = clampFloat(st.Terminal.FontSize+1, 8, 32)
		case "set:term-":
			st.Terminal.FontSize = clampFloat(st.Terminal.FontSize-1, 8, 32)
		case "set:indent":
			st.Editor.InsertSpaces = !st.Editor.InsertSpaces
		case "set:space":
			st.Editor.ShowWhitespace = !st.Editor.ShowWhitespace
		case "set:gofumpt":
			st.Editor.GofumptOnSave = !st.Editor.GofumptOnSave
		case "set:goimports":
			st.Editor.GoimportsOnSave = !st.Editor.GoimportsOnSave
		case "set:gofix":
			st.Editor.GofixOnSave = !st.Editor.GofixOnSave
		case "set:theme":
			switch st.UI.Theme {
			case "dark":
				st.UI.Theme = "light"
			case "light":
				st.UI.Theme = "dark"
			}
		}
	})
	if err != nil {
		s.notify("Could not save settings: " + err.Error())
		return
	}
	if tag == "set:theme" {
		s.applyTheme(s.store.Current().UI.Theme)
	}
	// The click is handled while the settings digits are already recorded.
	// layout paints this frame again so the new size is what the user sees.
	s.visualDirty = true
}

func roundTenth(v float64) float64 {
	return math.Round(v*10) / 10
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func blinkLabel(ms int) string {
	if ms <= 0 {
		return "steady"
	}
	return fmt.Sprintf("%d ms", ms)
}

func indentLabel(spaces bool) string {
	if spaces {
		return "Spaces"
	}
	return "Tabs"
}

func whitespaceLabel(on bool) string {
	if on {
		return "Shown"
	}
	return "Hidden"
}

func onOffLabel(on bool) string {
	if on {
		return "On"
	}
	return "Off"
}

func themeLabel(name string) string {
	switch name {
	case "light":
		return "Light"
	case "dark":
		return "Dark"
	default:
		return name
	}
}

// layoutTreeRows draws the project tree. A directory toggle is applied
// before the rows are committed, so the open folder is visible in the
// same frame that handled the click.
func (s *shell) layoutTreeRows(gtx layout.Context) layout.Dimensions {
	th := s.theme
	paint := func() (layout.Dimensions, string) {
		nodes := s.treeNodes()
		return ui.Tree(gtx, s.shaper, unit.Sp(13), nodes,
			th.Color("ui.foreground"), th.Color("ui.dim"), th.Color("ui.selection"),
			iconColors(th), &s.treeScroll)
	}
	macro := op.Record(gtx.Ops)
	dims, clicked := paint()
	call := macro.Stop()
	if clicked == "" || !s.clickTree(clicked) {
		call.Add(gtx.Ops)
		return dims
	}
	gtx.Execute(op.InvalidateCmd{})
	dims, _ = paint()
	return dims
}

// iconColors maps project-tree marks onto theme keys that already exist.
// Module, git, and plain files share ui.dim. Shell shares the folder gold.
// TOML uses gutter.deleted because gutter.modified sits too close to ui.selection.
// The shapes are what separate those pairs.
func iconColors(th *theme.Theme) ui.IconColors {
	return ui.IconColors{
		Folder:   th.Color("diag.warning"),
		Go:       th.Color("syntax.function"),
		Test:     th.Color("gutter.added"),
		Markdown: th.Color("syntax.constant"),
		License:  th.Color("diag.info"),
		HTML:     th.Color("diag.error"),
		JS:       th.Color("ui.accent"),
		TS:       th.Color("syntax.number"),
		JSON:     th.Color("syntax.string"),
		YAML:     th.Color("syntax.keyword"),
		TOML:     th.Color("gutter.deleted"),
		Module:   th.Color("ui.dim"),
		CSS:      th.Color("editor.caret"),
		Image:    th.Color("ui.foreground"),
		Shell:    th.Color("diag.warning"),
		Git:      th.Color("ui.dim"),
		Plain:    th.Color("ui.dim"),
	}
}
