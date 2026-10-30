package app

import (
	"context"
	"image"
	"math"
	"os"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/terminal"
	"github.com/russoj88/gio-ide/internal/ui"
)

// termTag is a Gio pointer target. The byte keeps two tags from sharing
// an address, which a zero-size struct{} field can do.
type termTag struct{ _ byte }

type termTab struct {
	term  *terminal.Term
	grid  terminal.Grid
	hit   termTag
	close termTag
}

func (s *shell) bindTerminal() {
	s.onCommand["terminal.new"] = func() { s.newTerminal() }
	s.onCommand["terminal.close"] = func() { s.closeTerminal() }
	s.onCommand["terminal.copy"] = func() { s.copyTerminal() }
	s.onCommand["terminal.paste"] = func() { s.pasteTerminal() }
}

func (s *shell) ensureTerminal() {
	if len(s.terms) == 0 {
		s.newTerminal()
	}
}

func (s *shell) newTerminal() {
	id := s.nextTerm
	s.nextTerm++
	cur := s.store.Current()
	t := terminal.New(id, cur.Terminal.ScrollbackLines, s.bus.Post)
	dir := s.root
	if dir == "" {
		dir, _ = os.Getwd()
	}
	shell := cur.Terminal.Shell
	go func() {
		if err := t.Start(context.Background(), shell, dir, 80, 24); err != nil {
			s.bus.Post(bus.Notification{Level: "error", Text: "terminal: " + err.Error()})
		}
	}()
	tab := &termTab{term: t}
	tab.grid.Term = t
	tab.grid.Resize = func(cols, rows int) {
		go func() { _ = t.Resize(cols, rows) }()
	}
	s.terms = append(s.terms, tab)
	s.termActive = len(s.terms) - 1
	s.bottomOpen = true
	s.bottomTool = "terminal"
	s.focus = focusTerminal
}

func (s *shell) closeTerminal() {
	if len(s.terms) == 0 {
		return
	}
	s.closeTermAt(s.termActive)
}

// closeTermAt drops one session. The last session hides the panel.
func (s *shell) closeTermAt(i int) {
	if i < 0 || i >= len(s.terms) {
		return
	}
	tab := s.terms[i]
	go func() { _ = tab.term.Close() }()
	s.terms = append(s.terms[:i], s.terms[i+1:]...)
	if len(s.terms) == 0 {
		s.termActive = 0
		s.bottomOpen = false
		s.focus = focusEditor
		return
	}
	if s.termActive > i {
		s.termActive--
	}
	if s.termActive >= len(s.terms) {
		s.termActive = len(s.terms) - 1
	}
}

func (s *shell) shutdownTerminals() {
	for _, tab := range s.terms {
		go func(t *terminal.Term) { _ = t.Close() }(tab.term)
	}
}

func (s *shell) activeTerm() *termTab {
	if s.termActive < 0 || s.termActive >= len(s.terms) {
		return nil
	}
	return s.terms[s.termActive]
}

func (s *shell) writeTerminal(st keymap.Stroke) {
	tab := s.activeTerm()
	if tab == nil {
		return
	}
	if st.Mod == keymap.ModShift && st.Key == "pageup" {
		tab.term.Scroll(s.termPage())
		return
	}
	if st.Mod == keymap.ModShift && st.Key == "pagedown" {
		tab.term.Scroll(-s.termPage())
		return
	}
	text := insertText(st)
	ctrl := st.Mod&keymap.ModCtrl != 0
	alt := st.Mod&keymap.ModAlt != 0
	b := terminal.EncodeKey(st.Key, text, ctrl, alt, tab.term.AppCursor())
	if len(b) == 0 {
		return
	}
	go func() { _, _ = tab.term.Write(b) }()
}

func (s *shell) termPage() int {
	tab := s.activeTerm()
	if tab == nil || tab.grid.Rows() < 2 {
		return 8
	}
	return tab.grid.Rows() - 1
}

func (s *shell) copyTerminal() {
	tab := s.activeTerm()
	if tab == nil {
		return
	}
	text := tab.term.SelectedText()
	if text == "" {
		s.notify("Nothing selected in the terminal")
		return
	}
	s.clipWrite = text
}

func (s *shell) pasteTerminal() {
	if s.activeTerm() == nil {
		return
	}
	s.pasteArmed = true
	s.pasteToTerm = true
}

func (s *shell) deliverTermPaste(text string) {
	tab := s.activeTerm()
	if tab == nil || text == "" {
		return
	}
	b := terminal.BracketPaste(text, tab.term.Bracketed())
	go func() { _, _ = tab.term.Write(b) }()
}

func (s *shell) onTerminalExit(ev bus.TerminalExited) {
	if ev.Err != "" {
		s.notice = "terminal: " + ev.Err
	}
	for i, tab := range s.terms {
		if tab.term != nil && tab.term.ID() == ev.Session {
			s.closeTermAt(i)
			return
		}
	}
}

// openNewTerminal runs terminal.new. Ctrl+T runs that same command.
func (s *shell) openNewTerminal() {
	if fn := s.onCommand["terminal.new"]; fn != nil {
		fn()
		return
	}
	s.newTerminal()
}

func (s *shell) layoutTerminal(gtx layout.Context) layout.Dimensions {
	th := s.theme
	s.ensureTerminal()
	tab := s.activeTerm()
	if tab == nil {
		return layout.Dimensions{}
	}
	tab.grid.Style = terminal.Style{
		Background: th.Color("terminal.background"),
		Foreground: th.Color("terminal.foreground"),
		Cursor:     th.Color("terminal.cursor"),
		Selection:  th.Color("terminal.selection"),
		FontSize:   unit.Sp(s.store.Current().Terminal.FontSize),
	}
	if tab.grid.Style.FontSize <= 0 {
		tab.grid.Style.FontSize = 13
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutTermTabs(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return tab.grid.Layout(gtx)
		}),
	)
}

func (s *shell) layoutTermTabs(gtx layout.Context) layout.Dimensions {
	th := s.theme
	s.termSelect = -1
	s.termCloseAt = -1
	h := max(gtx.Dp(unit.Dp(30)), 1)
	w := max(gtx.Constraints.Max.X, 1)
	size := image.Pt(w, h)
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	ui.FillSize(gtx.Ops, th.Color("ui.tabInactive"), size)

	s.termTabLeft = s.termTabLeft[:0]
	x := 0
	activeLeft, activeW := -1, 0
	gtx.Constraints.Min = image.Point{}
	for i := range s.terms {
		s.termTabLeft = append(s.termTabLeft, x)
		macro := op.Record(gtx.Ops)
		dims := s.layoutTermTab(gtx, i)
		call := macro.Stop()
		off := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
		if i == s.termActive {
			activeLeft, activeW = x, dims.Size.X
		}
		x += dims.Size.X
	}
	s.termPlusLeft = x
	macro := op.Record(gtx.Ops)
	s.layoutTermNew(gtx)
	plusCall := macro.Stop()
	off := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
	plusCall.Add(gtx.Ops)
	off.Pop()

	borderH := max(gtx.Dp(unit.Dp(1)), 1)
	rule := op.Offset(image.Pt(0, h-borderH)).Push(gtx.Ops)
	ui.FillSize(gtx.Ops, th.Color("ui.border"), image.Pt(w, borderH))
	rule.Pop()
	if activeW > 0 {
		mark := op.Offset(image.Pt(activeLeft, h-borderH)).Push(gtx.Ops)
		ui.FillSize(gtx.Ops, th.Color("ui.accent"), image.Pt(activeW, borderH))
		mark.Pop()
	}
	stack.Pop()

	s.applyTermClicks(gtx)
	return layout.Dimensions{Size: size}
}

func (s *shell) applyTermClicks(gtx layout.Context) {
	if s.termCloseAt >= 0 {
		s.closeTermAt(s.termCloseAt)
		s.visualDirty = true
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if s.termSelect >= 0 && s.termSelect < len(s.terms) {
		if s.termActive != s.termSelect || s.focus != focusTerminal {
			s.visualDirty = true
		}
		s.termActive = s.termSelect
		s.focus = focusTerminal
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (s *shell) layoutTermTab(gtx layout.Context, i int) layout.Dimensions {
	if i < 0 || i >= len(s.terms) {
		return layout.Dimensions{}
	}
	th := s.theme
	tab := s.terms[i]
	active := i == s.termActive
	title := "terminal"
	if tab.term != nil {
		if t := tab.term.Title(); t != "" {
			title = t
		}
	}
	fg := th.Color("ui.dim")
	bg := th.Color("ui.tabInactive")
	if active {
		fg = th.Color("ui.foreground")
		bg = th.Color("ui.tabActive")
	}
	h := max(gtx.Dp(unit.Dp(30)), 1)
	labelGtx := gtx
	labelGtx.Constraints.Min = image.Point{}
	labelGtx.Constraints.Max.X = gtx.Dp(unit.Dp(160))
	labelGtx.Constraints.Max.Y = h
	macro := op.Record(gtx.Ops)
	label := ui.Text(labelGtx, s.shaper, ui.Font, unit.Sp(13), fg, title)
	labelCall := macro.Stop()
	xMacro := op.Record(gtx.Ops)
	xLabel := ui.Text(labelGtx, s.shaper, ui.Font, unit.Sp(13), fg, "x")
	xCall := xMacro.Stop()

	padL := gtx.Dp(unit.Dp(12))
	gap := gtx.Dp(unit.Dp(8))
	closeW := max(gtx.Dp(unit.Dp(22)), xLabel.Size.X+gap)
	w := padL + label.Size.X + gap + closeW
	size := image.Pt(w, h)
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &tab.hit)
	ui.FillSize(gtx.Ops, bg, size)
	if active {
		bar := max(gtx.Dp(unit.Dp(3)), 2)
		ui.FillSize(gtx.Ops, th.Color("ui.accent"), image.Pt(w, bar))
	}
	labelY := max((h-label.Size.Y)/2, 0)
	labelOff := op.Offset(image.Pt(padL, labelY)).Push(gtx.Ops)
	labelCall.Add(gtx.Ops)
	labelOff.Pop()

	closeAt := w - closeW
	closeOff := op.Offset(image.Pt(closeAt, 0)).Push(gtx.Ops)
	closeClip := clip.Rect{Max: image.Pt(closeW, h)}.Push(gtx.Ops)
	event.Op(gtx.Ops, &tab.close)
	xY := (h - xLabel.Size.Y) / 2
	xX := (closeW - xLabel.Size.X) / 2
	if xY < 0 {
		xY = 0
	}
	if xX < 0 {
		xX = 0
	}
	xOff := op.Offset(image.Pt(xX, xY)).Push(gtx.Ops)
	xCall.Add(gtx.Ops)
	xOff.Pop()
	closeClip.Pop()
	closeOff.Pop()

	borderW := max(gtx.Dp(unit.Dp(1)), 1)
	edge := op.Offset(image.Pt(w-borderW, 0)).Push(gtx.Ops)
	ui.FillSize(gtx.Ops, th.Color("ui.border"), image.Pt(borderW, h))
	edge.Pop()
	stack.Pop()

	if termPressed(gtx, &tab.close) {
		s.termCloseAt = i
	} else if termPressed(gtx, &tab.hit) {
		s.termSelect = i
	}
	return layout.Dimensions{Size: size}
}

func (s *shell) layoutTermNew(gtx layout.Context) layout.Dimensions {
	th := s.theme
	h := max(gtx.Dp(unit.Dp(30)), 1)
	side := max(gtx.Dp(unit.Dp(8)), 1)
	btn := max(gtx.Dp(unit.Dp(22)), 1)
	if btn > h-2*side {
		btn = h - side
	}
	if btn < 1 {
		btn = 1
	}
	w := side + btn + side
	size := image.Pt(w, h)
	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	btnOff := op.Offset(image.Pt(side, (h-btn)/2)).Push(gtx.Ops)
	btnClip := clip.Rect{Max: image.Pt(btn, btn)}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.termNew)
	border := max(gtx.Dp(unit.Dp(1)), 1)
	ui.FillSize(gtx.Ops, th.Color("ui.border"), image.Pt(btn, btn))
	if btn > 2*border {
		in := op.Offset(image.Pt(border, border)).Push(gtx.Ops)
		ui.FillSize(gtx.Ops, th.Color("ui.panel"), image.Pt(btn-2*border, btn-2*border))
		in.Pop()
	}
	macro := op.Record(gtx.Ops)
	label := ui.Text(gtx, s.shaper, ui.Font, unit.Sp(16), th.Color("ui.foreground"), "+")
	call := macro.Stop()
	lx := (btn - label.Size.X) / 2
	ly := (btn - label.Size.Y) / 2
	if lx < 0 {
		lx = 0
	}
	if ly < 0 {
		ly = 0
	}
	off := op.Offset(image.Pt(lx, ly)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	btnClip.Pop()
	btnOff.Pop()
	stack.Pop()
	if termPressed(gtx, &s.termNew) {
		s.openNewTerminal()
		s.visualDirty = true
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.Dimensions{Size: size}
}

func termPressed(gtx layout.Context, tag event.Tag) bool {
	pressed := false
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || e.Kind != pointer.Press || !e.Buttons.Contain(pointer.ButtonPrimary) {
			continue
		}
		pressed = true
	}
	return pressed
}

// layoutTermSash is the bar above the terminal. Dragging it up increases termH.
// Pointer Y is converted with termSashY, the bar's window position from the
// previous frame, so the delta stays stable as the bar moves.
func (s *shell) layoutTermSash(gtx layout.Context) layout.Dimensions {
	h := max(gtx.Dp(unit.Dp(6)), 4)
	s.termSashH = h
	w := max(gtx.Constraints.Max.X, 1)
	size := image.Pt(w, h)
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &s.termSash)
	pointer.CursorNorthSouthResize.Add(gtx.Ops)
	minH, maxH := s.termLimits(gtx)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &s.termSash,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Enter | pointer.Leave,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Enter:
			s.termSashHot = true
		case pointer.Leave:
			s.termSashHot = false
		case pointer.Press:
			if !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			s.termDrag = true
			s.termDragID = e.PointerID
			s.termDragY = s.termSashY + roundPx(e.Position.Y)
			s.termDragH = s.termPanelHeight(gtx)
			gtx.Execute(pointer.GrabCmd{Tag: &s.termSash, ID: e.PointerID})
		case pointer.Drag:
			if !s.termDrag || e.PointerID != s.termDragID {
				continue
			}
			y := s.termSashY + roundPx(e.Position.Y)
			s.termH = clampTermH(s.termDragH-(y-s.termDragY), minH, maxH)
		case pointer.Release, pointer.Cancel:
			if e.PointerID == s.termDragID {
				s.termDrag = false
			}
		}
	}
	bg := s.theme.Color("terminal.background")
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
	line := s.theme.Color("ui.border")
	lh := 1
	if s.termDrag || s.termSashHot {
		line = s.theme.Color("editor.caret")
		lh = max(gtx.Dp(unit.Dp(2)), 2)
	}
	paint.FillShape(gtx.Ops, line, clip.Rect{Max: image.Pt(size.X, lh)}.Op())
	return layout.Dimensions{Size: size}
}

func (s *shell) termLimits(gtx layout.Context) (minH, maxH int) {
	minH = gtx.Dp(unit.Dp(80))
	maxH = s.termCenterH - s.termFindH - s.termSashH - gtx.Dp(unit.Dp(120))
	return minH, maxH
}

func (s *shell) termPanelHeight(gtx layout.Context) int {
	h := s.termH
	if h <= 0 {
		h = gtx.Dp(unit.Dp(240))
	}
	minH, maxH := s.termLimits(gtx)
	return clampTermH(h, minH, maxH)
}

func clampTermH(h, minH, maxH int) int {
	if minH < 1 {
		minH = 1
	}
	if maxH < minH {
		maxH = minH
	}
	if h < minH {
		return minH
	}
	if h > maxH {
		return maxH
	}
	return h
}

func roundPx(v float32) int {
	return int(math.Round(float64(v)))
}
