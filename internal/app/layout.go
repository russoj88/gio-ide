package app

import (
	"fmt"
	"image/color"
	"path/filepath"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/search"
	"github.com/russoj88/gio-ide/internal/ui"
)

func (s *shell) layout(gtx layout.Context) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := s.layoutFrame(gtx)
	call := macro.Stop()
	if !s.visualDirty {
		call.Add(gtx.Ops)
		return dims
	}
	s.visualDirty = false
	return s.layoutFrame(gtx)
}

func (s *shell) layoutFrame(gtx layout.Context) layout.Dimensions {
	th := s.theme
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions { return ui.Fill(gtx, th.Color("ui.background")) },
						s.layoutBody,
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.HBar(gtx, th.Color("ui.border"))
				}),
				layout.Rigid(s.layoutStatus),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if s.popup == "" {
				return layout.Dimensions{}
			}
			return s.layoutPopup(gtx)
		}),
	)
}

func (s *shell) layoutBody(gtx layout.Context) layout.Dimensions {
	th := s.theme
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(s.layoutToolStrip),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.leftOpen {
				return layout.Dimensions{}
			}
			w := gtx.Dp(unit.Dp(260))
			gtx.Constraints.Min.X = w
			gtx.Constraints.Max.X = w
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, s.layoutLeft),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.VBar(gtx, th.Color("ui.border"))
				}),
			)
		}),
		layout.Flexed(1, s.layoutCenter),
	)
}

func (s *shell) layoutToolStrip(gtx layout.Context) layout.Dimensions {
	th := s.theme
	w := gtx.Dp(unit.Dp(40))
	gtx.Constraints.Min.X = w
	gtx.Constraints.Max.X = w
	tools := []struct{ id, label string }{
		{"project", "P"},
		{"git", "G"},
		{"problems", "!"},
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return ui.Fill(gtx, th.Color("ui.panel")) },
				func(gtx layout.Context) layout.Dimensions {
					var kids []layout.FlexChild
					for _, tool := range tools {
						kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							bg := th.Color("ui.panel")
							if s.leftOpen && s.leftTool == tool.id {
								bg = th.Color("ui.selection")
							}
							dims, hit := ui.Click(gtx, s.tagFor("tool:"+tool.id), bg, func(gtx layout.Context) layout.Dimensions {
								return ui.Inset(gtx, unit.Dp(10), unit.Dp(8), unit.Dp(10), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
									return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), tool.label)
								})
							})
							if hit {
								if tool.id == "problems" {
									s.runCommand("view.problemsTool")
								} else {
									s.showTool(tool.id)
								}
							}
							return dims
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
				},
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.VBar(gtx, th.Color("ui.border"))
		}),
	)
}

func (s *shell) layoutLeft(gtx layout.Context) layout.Dimensions {
	th := s.theme
	title := "Project"
	body := "Open a folder from the command line to see its files."
	switch s.leftTool {
	case "git":
		title = "Git"
		body = "This folder is not a Git repository."
		if s.gitRepoNow() != nil {
			if len(s.gitFiles) == 0 {
				body = s.branch + "  clean"
			} else {
				return s.layoutGitFiles(gtx, title)
			}
		}
	case "problems":
		title = "Problems"
		body = "No problems."
	}
	if s.leftTool == "problems" && len(s.problems) > 0 {
		return s.layoutProblemList(gtx, title)
	}
	if s.leftTool == "project" {
		// The tree stays up with no workspace so Scratches is reachable.
		// The old empty-state sentence remains a dim hint above it.
		hint := ""
		if s.root == "" || len(s.files) == 0 {
			hint = body
			if s.root != "" {
				hint = s.root
			}
		}
		return s.layoutProjectTree(gtx, title, hint)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(10), unit.Dp(12), unit.Dp(8), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), title)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.HBar(gtx, th.Color("ui.border"))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(10), unit.Dp(12), unit.Dp(10), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(12), th.Color("ui.dim"), body)
			})
		}),
	)
}

func (s *shell) layoutProblemList(gtx layout.Context, title string) layout.Dimensions {
	th := s.theme
	n := min(len(s.problems), 80)
	rows := s.problems[:n]
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(10), unit.Dp(12), unit.Dp(8), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), title)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.HBar(gtx, th.Color("ui.border"))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			dims, _ := ui.List(gtx, s.shaper, unit.Sp(12), rows, -1,
				th.Color("ui.foreground"), th.Color("ui.selection"), th.Color("ui.panel"), nil, nil)
			return dims
		}),
	)
}

func (s *shell) layoutGitFiles(gtx layout.Context, title string) layout.Dimensions {
	th := s.theme
	rows := make([]string, 0, len(s.gitFiles))
	active := -1
	for i, f := range s.gitFiles {
		rows = append(rows, string(f.Staging)+string(f.Work)+"  "+f.Path)
		if f.Path == s.gitDiffPath {
			active = i
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(10), unit.Dp(12), unit.Dp(8), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), title+"  "+s.branch)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.HBar(gtx, th.Color("ui.border"))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			dims, picked := ui.List(gtx, s.shaper, unit.Sp(12), rows, active,
				th.Color("ui.foreground"), th.Color("ui.selection"), th.Color("ui.panel"), &s.gitScroll, s.gitRowTag)
			if picked >= 0 && picked < len(s.gitFiles) {
				s.showGitDiff(s.gitFiles[picked].Path)
			}
			return dims
		}),
	)
}

func (s *shell) layoutCenter(gtx layout.Context) layout.Dimensions {
	s.termCenterH = gtx.Constraints.Max.Y
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := s.layoutFindBar(gtx)
			s.termFindH = dims.Size.Y
			return dims
		}),
		layout.Flexed(1, s.layoutEditor),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.bottomOpen {
				return layout.Dimensions{}
			}
			return s.layoutBottomColumn(gtx)
		}),
	)
}

func (s *shell) layoutBottomColumn(gtx layout.Context) layout.Dimensions {
	th := s.theme
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.bottomTool == "terminal" {
				return s.layoutTermSash(gtx)
			}
			return ui.HBar(gtx, th.Color("ui.border"))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(unit.Dp(180))
			if s.bottomTool == "terminal" {
				h = s.termPanelHeight(gtx)
			}
			gtx.Constraints.Min.Y = h
			gtx.Constraints.Max.Y = h
			dims := s.layoutBottom(gtx)
			if s.bottomTool == "terminal" {
				s.termSashY = s.termCenterH - s.termSashH - h
			}
			return dims
		}),
	)
}

func (s *shell) layoutEditor(gtx layout.Context) layout.Dimensions {
	return s.layoutPane(gtx, s.rootPane)
}

func (s *shell) layoutPane(gtx layout.Context, p *pane) layout.Dimensions {
	if p == nil || p.isLeaf() {
		return s.layoutLeaf(gtx, p)
	}
	ratio := p.ratio
	if ratio <= 0 {
		ratio = 0.5
	}
	return ui.Split(gtx, p.down, ratio, gtx.Dp(unit.Dp(1)),
		func(gtx layout.Context) layout.Dimensions { return s.layoutPane(gtx, p.first) },
		func(gtx layout.Context) layout.Dimensions { return s.layoutPane(gtx, p.second) },
	)
}

func (s *shell) layoutLeaf(gtx layout.Context, p *pane) layout.Dimensions {
	if s.showsGitDiff(p) {
		return s.layoutGitDiff(gtx)
	}
	if p == nil || len(p.tabs) == 0 {
		return s.layoutWelcome(gtx)
	}
	if p.active < 0 || p.active >= len(p.tabs) {
		p.active = 0
	}
	for _, t := range p.tabs {
		pane := p
		t.view.OnFocus = func() {
			s.focus = focusEditor
			s.focusPane = pane
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			tab := p.tabs[p.active]
			if isMarkdownPath(tab.view.Doc.Path()) {
				return s.layoutMarkdown(gtx, p, tab)
			}
			focused := s.focus == focusEditor && s.popup == "" && s.focusPane == p
			s.requestSyntax(tab)
			tab.view.SetMarks(s.marks[tab.view.Doc.ID()])
			return tab.view.Layout(gtx, s.shaper, s.editorStyle(focused), s.spansFor(tab.view))
		}),
	)
}

func (s *shell) editorStyle(focused bool) editor.Style {
	cur := s.store.Current()
	th := s.theme
	fs := cur.Editor.FontSize
	if fs <= 0 {
		fs = 14
	}
	lh := cur.Editor.LineHeight
	if lh <= 0 {
		lh = 1.3
	}
	return editor.Style{
		FontSize:   unit.Sp(fs),
		LineHeight: float32(lh),
		TabWidth:   cur.Editor.TabWidth,
		ShowSpace:  cur.Editor.ShowWhitespace,
		Blink:      time.Duration(cur.Editor.CursorBlinkMS) * time.Millisecond,
		ThemeGen:   s.themeGen,
		Focused:    focused,
		Colors: editor.Colors{
			Background:    th.Color("editor.background"),
			Foreground:    th.Color("editor.foreground"),
			Selection:     th.Color("editor.selection"),
			Caret:         th.Color("editor.caret"),
			LineHighlight: th.Color("editor.lineHighlight"),
			GutterBG:      th.Color("gutter.background"),
			GutterFG:      th.Color("gutter.foreground"),
			Bracket:       th.Color("editor.bracketMatch"),
			Whitespace:    th.Color("editor.whitespace"),
			MarkNote:      markColor(th.Color("editor.findMatch")),
			MarkError:     markColor(th.Color("diag.error")),
			MarkSmell:     markColor(th.Color("diag.warning")),
		},
	}
}

func markColor(c color.NRGBA) color.NRGBA { c.A = 80; return c }

func (s *shell) layoutWelcome(gtx layout.Context) layout.Dimensions {
	th := s.theme
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return ui.Fill(gtx, th.Color("editor.background")) },
		func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(28), unit.Dp(28), unit.Dp(28), unit.Dp(28), func(gtx layout.Context) layout.Dimensions {
				var kids []layout.FlexChild
				lines := []struct {
					text string
					col  string
					size unit.Sp
				}{
					{"gio-ide", "editor.foreground", unit.Sp(22)},
					{"A Go editor with completion, navigation, a terminal, and Git built in.", "ui.dim", unit.Sp(14)},
					{"Find Action  Ctrl+Shift+A", "editor.foreground", unit.Sp(13)},
					{"Go to File  Ctrl+Shift+N", "editor.foreground", unit.Sp(13)},
					{"Terminal  Alt+F12", "editor.foreground", unit.Sp(13)},
				}
				if s.root != "" {
					lines = append([]struct {
						text string
						col  string
						size unit.Sp
					}{
						{filepath.Base(s.root), "editor.foreground", unit.Sp(22)},
						{s.root, "ui.dim", unit.Sp(13)},
					}, lines[1:]...)
				}
				for _, ln := range lines {
					kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.Inset(gtx, unit.Dp(2), 0, unit.Dp(2), 0, func(gtx layout.Context) layout.Dimensions {
							return ui.Text(gtx, s.shaper, ui.Font, ln.size, th.Color(ln.col), ln.text)
						})
					}))
				}
				if s.notice != "" {
					kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.Inset(gtx, unit.Dp(16), 0, 0, 0, func(gtx layout.Context) layout.Dimensions {
							return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("diag.warning"), s.notice)
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
			})
		},
	)
}

func (s *shell) layoutBottom(gtx layout.Context) layout.Dimensions {
	th := s.theme
	title := "Terminal"
	body := "Alt+F12 closes this panel."
	switch s.bottomTool {
	case "problems":
		title = "Problems"
		if len(s.problems) == 0 {
			body = "No problems."
		} else {
			body = joinLines(s.problems)
		}
	case "search":
		title = "Search"
		if s.searchErr != "" {
			body = s.searchErr
		} else if len(s.searchHits) == 0 {
			body = "No matches yet. Ctrl+Shift+F searches the project."
		} else {
			body = formatSearchHits(s.searchHits)
		}
	}
	if s.bottomTool == "terminal" {
		return s.layoutTerminal(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(6), unit.Dp(10), unit.Dp(4), unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(12), th.Color("ui.dim"), title)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(4), unit.Dp(10), unit.Dp(8), unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.MonoFont, unit.Sp(13), th.Color("terminal.foreground"), body)
			})
		}),
	)
}

func (s *shell) layoutStatus(gtx layout.Context) layout.Dimensions {
	th := s.theme
	pos := fmt.Sprintf("%d:%d", s.line, s.col)
	diags := fmt.Sprintf("%d errors  %d warnings", s.diagErr, s.diagWarn)
	heap := fmt.Sprintf("heap %s", formatBytes(s.mem.Heap))
	if s.mem.RSS > 0 {
		heap = fmt.Sprintf("heap %s  rss %s", formatBytes(s.mem.Heap), formatBytes(s.mem.RSS))
	}
	intel := "intel " + s.intel
	items := []ui.StatusItem{
		{Text: s.branch},
		{Text: pos},
		{Text: diags, Color: th.Color("diag.error")},
		{Text: intel},
		{Text: heap},
		{Text: "gio-ide " + s.version},
	}
	return ui.StatusBar(gtx, s.shaper, unit.Sp(12), th.Color("ui.statusBar"), th.Color("ui.statusForeground"), items)
}

func (s *shell) layoutPopup(gtx layout.Context) layout.Dimensions {
	th := s.theme
	title := "Find Action"
	switch s.popup {
	case "goto":
		title = "Go to File"
	case "symbol":
		title = "Go to Symbol"
	case "structure":
		title = "File Structure"
	case "line":
		title = "Go to Line"
	case "recent":
		title = "Recent Files"
	case "search", "replaceFiles":
		title = "Search in Files"
	case "find":
		title = "Find"
	case "replace":
		title = "Replace"
	case "commit":
		title = "Commit"
	case "renameScratch":
		title = "Rename Scratch"
	case "moveScratch":
		title = "Move Scratch to Project"
	case "branches":
		title = "Branches"
	case "about":
		title = "About gio-ide"
	case "unsaved":
		title = "Save changes?"
	case "completion":
		title = "Completion"
	case "usages":
		title = "Find Usages"
	case "dialog":
		title = "Code intelligence"
	case "settings":
		title = "Settings"
	}
	cardH := unit.Dp(420)
	if s.popup == "settings" {
		cardH = unit.Dp(520)
	}
	return ui.Popup(gtx, th.Color("ui.popup"), th.Color("ui.border"), unit.Dp(560), cardH,
		title, s.shaper, th.Color("ui.foreground"), func(gtx layout.Context) layout.Dimensions {
			return s.layoutPopupBody(gtx)
		})
}

func (s *shell) layoutPopupBody(gtx layout.Context) layout.Dimensions {
	th := s.theme
	if s.popup == "commit" || s.popup == "renameScratch" || s.popup == "moveScratch" {
		msg := "Commit message, then Enter. Changed files are staged."
		switch s.popup {
		case "renameScratch":
			msg = "Scratch name, then Enter."
		case "moveScratch":
			msg = "Path in the project, then Enter."
		}
		if s.popup == "commit" && s.gitNotice != "" {
			msg = s.gitNotice
		}
		return ui.Inset(gtx, unit.Dp(8), unit.Dp(8), 0, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("diag.warning"), msg)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(10), 0, 0, 0, func(gtx layout.Context) layout.Dimensions {
						return ui.Text(gtx, s.shaper, ui.MonoFont, unit.Sp(14), th.Color("editor.foreground"), showCaret(s.field, s.fieldCaret))
					})
				}),
			)
		})
	}
	if s.popup == "about" {
		lines := []string{
			fmt.Sprintf("gio-ide %s", s.version),
			"Go code editor for Linux.",
			"In-process gopls, tree-sitter, go-git.",
			"No plugins.",
		}
		kids := make([]layout.FlexChild, 0, len(lines))
		for _, line := range lines {
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.Inset(gtx, unit.Dp(2), 0, unit.Dp(2), 0, func(gtx layout.Context) layout.Dimensions {
					return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(14), th.Color("ui.foreground"), line)
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	}
	if s.popup == "unsaved" {
		return s.layoutUnsaved(gtx)
	}
	if s.popup == "settings" {
		return s.layoutSettings(gtx)
	}
	if s.popup == "dialog" {
		kids := make([]layout.FlexChild, 0, 2)
		if msg := firstLine(s.dialogMsg); msg != "" {
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.Inset(gtx, 0, 0, unit.Dp(8), 0, func(gtx layout.Context) layout.Dimensions {
					return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), msg)
				})
			}))
		}
		kids = append(kids, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutPopupRows(gtx)
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(32))
					gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(32))
					return ui.Fill(gtx, th.Color("editor.background"))
				},
				func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(6), unit.Dp(8), unit.Dp(6), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
						caret := s.field
						if s.popup != "" {
							caret = showCaret(s.field, s.fieldCaret)
						}
						return ui.Text(gtx, s.shaper, ui.MonoFont, unit.Sp(14), th.Color("editor.foreground"), caret)
					})
				},
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(8), 0, 0, 0, func(gtx layout.Context) layout.Dimensions {
				return ui.HBar(gtx, th.Color("ui.border"))
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if s.popup == "goto" || s.popup == "recent" || s.popup == "search" || s.popup == "replaceFiles" ||
				s.popup == "completion" || s.popup == "usages" || s.popup == "symbol" || s.popup == "structure" ||
				s.popup == "branches" {
				return s.layoutPopupRows(gtx)
			}
			if s.popup != "action" {
				return ui.Inset(gtx, unit.Dp(10), 0, 0, 0, func(gtx layout.Context) layout.Dimensions {
					return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.dim"), "This action is not in this version.")
				})
			}
			items := s.filteredCommands()
			if s.listIndex >= len(items) && len(items) > 0 {
				s.listIndex = len(items) - 1
			}
			rows := make([]string, 0, len(items))
			for _, c := range items {
				label := c.Title
				if c.Phase != "" {
					label += "  (later)"
				}
				rows = append(rows, label+"    "+c.ID)
			}
			rowH := max(gtx.Dp(unit.Dp(24)), 1)
			s.listVisible = max(gtx.Constraints.Max.Y/rowH, 1)
			dims, _ := ui.List(gtx, s.shaper, unit.Sp(13), rows, s.listIndex,
				th.Color("ui.foreground"), th.Color("ui.selection"), th.Color("ui.popup"), &s.listScroll, nil)
			return dims
		}),
	)
}

func (s *shell) layoutProjectTree(gtx layout.Context, title, hint string) layout.Dimensions {
	th := s.theme
	kids := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(10), unit.Dp(12), unit.Dp(8), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), title)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.HBar(gtx, th.Color("ui.border"))
		}),
	}
	if hint != "" {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.Inset(gtx, unit.Dp(10), unit.Dp(12), unit.Dp(10), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(12), th.Color("ui.dim"), hint)
			})
		}))
	}
	kids = append(kids, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return s.layoutTreeRows(gtx)
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func (s *shell) layoutPopupRows(gtx layout.Context) layout.Dimensions {
	th := s.theme
	var rows []string
	switch s.popup {
	case "goto":
		rows = s.gotoHits
		if len(rows) == 0 {
			rows = []string{"No matching files"}
		}
	case "recent":
		rows = s.recent
		if len(rows) == 0 {
			rows = []string{"No recent files"}
		}
	case "search":
		rows = []string{"Enter runs the search. Results open in the bottom panel."}
	case "replaceFiles":
		rows = []string{"Pattern, then Tab is not required: type pattern<tab>replacement, then Enter."}
	case "completion":
		for _, it := range s.compItems {
			line := it.Label
			if it.Detail != "" {
				line += "  " + it.Detail
			}
			rows = append(rows, line)
		}
		if len(rows) == 0 {
			rows = []string{"No completions"}
		}
	case "usages":
		for _, loc := range s.locHits {
			rows = append(rows, fmt.Sprintf("%s:%d:%d", loc.Path, loc.Line, loc.Col))
		}
		if len(rows) == 0 {
			rows = []string{"No usages"}
		}
	case "symbol", "structure":
		for _, sy := range s.symHits {
			line := sy.Name
			if sy.Container != "" {
				line = sy.Container + "." + line
			}
			if sy.Path != "" {
				line += "  " + sy.Path
			}
			rows = append(rows, line)
		}
		if len(rows) == 0 {
			rows = []string{"No symbols"}
		}
	case "dialog":
		rows = append(rows, s.dialogActions...)
		if len(rows) == 0 {
			rows = []string{"OK"}
		}
	case "branches":
		for _, b := range s.gitBranches {
			line := b.Name
			if b.Current {
				line += "  *"
			}
			rows = append(rows, line)
		}
		if len(rows) == 0 {
			rows = []string{"No branches yet. Type a name and press Enter."}
		}
	}
	if s.listIndex >= len(rows) {
		s.listIndex = 0
	}
	var kids []layout.FlexChild
	n := min(len(rows), 12)
	start := 0
	if s.listIndex >= n {
		start = s.listIndex - n + 1
	}
	for i := start; i < start+n && i < len(rows); i++ {
		i := i
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := th.Color("ui.popup")
			selectable := s.popup == "goto" || s.popup == "recent" || s.popup == "completion" ||
				s.popup == "usages" || s.popup == "symbol" || s.popup == "structure" || s.popup == "dialog" ||
				s.popup == "branches"
			if i == s.listIndex && selectable {
				bg = th.Color("ui.selection")
			}
			// Fill reports Constraints.Max. Leave Max.Y unset and the first
			// rigid row takes the rest of the list, so later rows get height 0.
			rowH := min(gtx.Dp(unit.Dp(22)), gtx.Constraints.Max.Y)
			gtx.Constraints.Min.Y = rowH
			gtx.Constraints.Max.Y = rowH
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = rowH
					gtx.Constraints.Max.Y = rowH
					return ui.Fill(gtx, bg)
				},
				func(gtx layout.Context) layout.Dimensions {
					return ui.Inset(gtx, unit.Dp(2), unit.Dp(8), unit.Dp(2), unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
						return ui.Text(gtx, s.shaper, ui.MonoFont, unit.Sp(13), th.Color("ui.foreground"), rows[i])
					})
				},
			)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func formatSearchHits(hits []search.Result) string {
	if len(hits) == 0 {
		return ""
	}
	h := hits[0]
	return fmt.Sprintf("%d matches  %s:%d  %s", len(hits), h.Path, h.Line, h.LineText)
}

func (s *shell) layoutUnsaved(gtx layout.Context) layout.Dimensions {
	th := s.theme
	name := "this file"
	if s.closePane != nil && s.closePane.isLeaf() && s.closeIndex >= 0 && s.closeIndex < len(s.closePane.tabs) {
		name = s.closePane.tabs[s.closeIndex].view.Doc.Path()
	}
	actions := []string{"Save", "Don't Save", "Cancel"}
	if s.listIndex >= len(actions) {
		s.listIndex = 0
	}
	var kids []layout.FlexChild
	kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return ui.Inset(gtx, unit.Dp(4), 0, unit.Dp(10), 0, func(gtx layout.Context) layout.Dimensions {
			return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(13), th.Color("ui.foreground"), name+" has unsaved changes.")
		})
	}))
	for i, label := range actions {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := th.Color("ui.popup")
			if i == s.listIndex {
				bg = th.Color("ui.selection")
			}
			dims, hit := ui.Click(gtx, s.tagFor("unsaved:"+label), bg, func(gtx layout.Context) layout.Dimensions {
				return ui.Inset(gtx, unit.Dp(8), unit.Dp(10), unit.Dp(8), unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
					return ui.Text(gtx, s.shaper, ui.Font, unit.Sp(14), th.Color("ui.foreground"), label)
				})
			})
			if hit {
				s.listIndex = i
				s.acceptPopup()
			}
			return dims
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func showCaret(s string, caret int) string {
	r := []rune(s)
	if caret < 0 {
		caret = 0
	}
	if caret > len(r) {
		caret = len(r)
	}
	return string(r[:caret]) + "|" + string(r[caret:])
}

func formatBytes(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	v := float64(n)
	u := -1
	for v >= 1024 && u < len(units)-1 {
		v /= 1024
		u++
	}
	if u < 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", v, units[u])
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
