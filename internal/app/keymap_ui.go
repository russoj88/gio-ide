package app

import (
	"image"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/commands"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/ui"
	"github.com/russoj88/gio-ide/internal/xdg"
)

type keybindRow struct {
	id    string
	title string
	keys  string
	dirty bool
}

func (s *shell) layoutKeymapHeader(gtx layout.Context) layout.Dimensions {
	th := s.theme
	rowH := gtx.Dp(unit.Dp(settingsRowDp))
	gtx.Constraints.Min.Y = rowH
	gtx.Constraints.Max.Y = rowH
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, 0, unit.Dp(8), 0, 0, func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), "Keymap")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.settingButton(gtx, "km-reset-all", "Reset", unit.Dp(64))
		}),
	)
}

// layoutKeymap draws the command list under the settings rows.
// The shortcut button captures the next stroke. Reset restores that command.
func (s *shell) layoutKeymap(gtx layout.Context) layout.Dimensions {
	rows := s.keybindRows()
	rowH := max(gtx.Dp(unit.Dp(28)), 1)
	maxRows := max(gtx.Constraints.Max.Y/rowH, 1)
	maxOff := max(len(rows)-maxRows, 0)
	keyWheel(gtx, &s.keyScroll)
	if s.keyScroll < 0 {
		s.keyScroll = 0
	}
	if s.keyScroll > maxOff {
		s.keyScroll = maxOff
	}
	var kids []layout.FlexChild
	end := min(s.keyScroll+maxRows, len(rows))
	for i := s.keyScroll; i < end; i++ {
		row := rows[i]
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = rowH
			gtx.Constraints.Max.Y = rowH
			return s.layoutKeyRow(gtx, row)
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	dims.Size.X = gtx.Constraints.Max.X
	return dims
}

func (s *shell) layoutKeyRow(gtx layout.Context, row keybindRow) layout.Dimensions {
	th := s.theme
	label := row.keys
	if label == "" {
		label = "Unbound"
	}
	if s.keyCapture == row.id {
		label = "Press a key..."
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, 0, unit.Dp(8), 0, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), row.title)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.passClick(gtx, "km:"+row.id, label, unit.Dp(148))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !row.dirty {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(64)), gtx.Constraints.Max.Y)}
			}
			return s.passClick(gtx, "km-reset:"+row.id, "Reset", unit.Dp(64))
		}),
	)
}

func (s *shell) keybindRows() []keybindRow {
	cur := s.keys
	def := keymap.Default()
	rows := make([]keybindRow, 0, len(commands.Catalog))
	for _, c := range commands.Catalog {
		if c.Phase != "" {
			continue
		}
		now := shortcutsFor(cur, c.ID)
		rows = append(rows, keybindRow{
			id:    c.ID,
			title: c.Title,
			keys:  now,
			dirty: now != shortcutsFor(def, c.ID),
		})
	}
	return rows
}

func shortcutsFor(m *keymap.Map, id string) string {
	if m == nil {
		return ""
	}
	var parts []string
	for _, b := range m.Bindings() {
		if b.Command != id {
			continue
		}
		parts = append(parts, formatStrokes(b.Keys))
	}
	return strings.Join(parts, ", ")
}

func formatStrokes(keys []keymap.Stroke) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k.String()
	}
	return strings.Join(parts, " ")
}

func commandTitle(id string) string {
	for _, c := range commands.Catalog {
		if c.ID == id {
			return c.Title
		}
	}
	return id
}

func (s *shell) bindCaptured(st keymap.Stroke) {
	id := s.keyCapture
	s.keyCapture = ""
	if id == "" {
		return
	}
	if s.keys == nil {
		s.keys = keymap.Default()
	}
	displaced := keymap.SetBinding(s.keys, []keymap.Stroke{st}, id)
	s.chord.Map = s.keys
	s.chord.Clear()
	if err := keymap.Save(xdg.KeymapPath(), s.keys); err != nil {
		s.notify("Could not save keymap: " + err.Error())
	} else if displaced != "" {
		s.notify(st.String() + " was " + commandTitle(displaced))
	}
	s.visualDirty = true
}

func (s *shell) resetKeyCommand(id string) {
	if s.keys == nil {
		s.keys = keymap.Default()
	}
	keymap.ResetCommand(s.keys, id)
	s.chord.Map = s.keys
	s.chord.Clear()
	s.keyCapture = ""
	if err := keymap.Save(xdg.KeymapPath(), s.keys); err != nil {
		s.notify("Could not save keymap: " + err.Error())
	}
	s.visualDirty = true
}

func (s *shell) resetKeymap() {
	s.keys = keymap.Default()
	s.chord.Map = s.keys
	s.chord.Clear()
	s.keyCapture = ""
	if err := keymap.Save(xdg.KeymapPath(), s.keys); err != nil {
		s.notify("Could not save keymap: " + err.Error())
	}
	s.visualDirty = true
}

// passClick is a button that does not swallow the wheel, so the keymap list scrolls.
func (s *shell) passClick(gtx layout.Context, tag, label string, w unit.Dp) layout.Dimensions {
	th := s.theme
	width := max(gtx.Dp(w), 1)
	height := gtx.Constraints.Max.Y
	if height < 1 {
		height = gtx.Dp(unit.Dp(settingsRowDp))
	}
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = image.Pt(width, height)
	ht := s.tagFor(tag)
	hit := false
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: ht, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
			hit = true
		}
	}
	size := gtx.Constraints.Max
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, ht)
	pass.Pop()
	ui.FillSize(gtx.Ops, th.Color("ui.selection"), size)
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), label)
	})
	stack.Pop()
	if hit {
		s.adjustSetting(tag)
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.Dimensions{Size: size}
}

func keyWheel(gtx layout.Context, scroll *int) {
	size := gtx.Constraints.Max
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, scroll)
	stack.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  scroll,
			Kinds:   pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1e6, Max: 1e6},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || e.Kind != pointer.Scroll {
			continue
		}
		if e.Scroll.Y < 0 {
			*scroll -= 3
		} else if e.Scroll.Y > 0 {
			*scroll += 3
		}
	}
}
