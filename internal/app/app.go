// Package app is the Gio shell: window, frame loop, layout, and focus.
package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/russoj88/gio-ide/internal/buffer"
	"github.com/russoj88/gio-ide/internal/bus"
	"github.com/russoj88/gio-ide/internal/commands"
	"github.com/russoj88/gio-ide/internal/editor"
	"github.com/russoj88/gio-ide/internal/git"
	"github.com/russoj88/gio-ide/internal/intel"
	"github.com/russoj88/gio-ide/internal/keymap"
	"github.com/russoj88/gio-ide/internal/runtimemem"
	"github.com/russoj88/gio-ide/internal/search"
	"github.com/russoj88/gio-ide/internal/settings"
	"github.com/russoj88/gio-ide/internal/syntax"
	"github.com/russoj88/gio-ide/internal/theme"
	"github.com/russoj88/gio-ide/internal/workspace"
	"github.com/russoj88/gio-ide/internal/xdg"
)

// Run opens the window. path may be a directory, a file, or empty.
func Run(path, version string) error {
	project, file, err := splitLaunch(path)
	if err != nil {
		return err
	}
	sh, err := newShell(project, version)
	if err != nil {
		return err
	}
	if file != "" {
		sh.openPath(file)
	}
	go sh.loop()
	app.Main()
	return nil
}

func splitLaunch(path string) (project, file string, err error) {
	if path == "" {
		return "", "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", "", err
	}
	if st.IsDir() {
		return abs, "", nil
	}
	return filepath.Dir(abs), abs, nil
}

type focusArea int

const (
	focusEditor focusArea = iota
	focusTree
	focusTerminal
	focusPopup
	focusFind
)

type shell struct {
	version    string
	root       string
	scratchDir string // empty skips the Scratches tree row
	window     *app.Window
	bus        bus.Bus
	store      *settings.Store
	theme      *theme.Theme
	themeName  string
	keys       *keymap.Map
	chord      keymap.Resolver
	cmds       *commands.Registry
	shaper     *text.Shaper
	logf       io.WriteCloser
	stop       chan struct{}

	focus       focusArea
	line, col   int
	branch      string
	intel       string
	intelDetail string
	diagErr     int
	diagWarn    int
	mem         runtimemem.Sample
	problems    []string
	notice      string

	leftOpen   bool
	bottomOpen bool
	leftTool   string // project, git, problems
	bottomTool string // terminal, problems, search

	popup       string // "", "action", "goto", "line", "branches", "commit", "renameScratch", "moveScratch", "about", "settings"
	field       string
	fieldCaret  int
	listIndex   int
	listScroll  int  // first visible command-palette row
	listVisible int  // rows laid out last frame; 0 until layout
	visualDirty bool // settings changed during this frame; layout paints again

	keyCapture string  // command waiting for a new shortcut in Settings
	keyScroll  int     // first visible keymap row
	shownFont  float64 // editor font size painted into the settings card

	focused bool
	heldTag event.Tag
	tag     struct{}
	tags    map[string]*handlerTag

	onCommand map[string]func()

	docsMu    sync.Mutex
	docs      map[uint64]*buffer.Doc
	opening   map[string]bool
	rootPane  *pane
	focusPane *pane
	themeGen  uint64

	// closeAfter is a document id to close once its save lands.
	closeAfter uint64
	closePane  *pane
	closeIndex int
	// saveVersions is the document version exported for an in-flight save.
	saveVersions map[uint64]int32
	// formatToken is the newest format request for a document.
	// An older FormatResult is ignored.
	formatToken map[uint64]int64

	// Clipboard requests are executed on the UI goroutine.
	clipWrite  string
	pasteArmed bool
	pasteWait  int

	startupFile string

	syn    syntax.Engine
	colors map[uint64]*docPaint
	// marks are persistent highlights for one document. The shell map is the source of truth.
	marks map[uint64][]editor.Mark

	findOn      bool
	findReplace bool
	findRegex   bool
	findQuery   string
	findRepl    string
	findCaret   int
	findField   int // 0 query, 1 replacement
	findOrigin  int

	wsMu          sync.Mutex
	index         workspace.Index
	files         []string
	tree          *workspace.Node
	openDirs      map[string]bool
	treeScroll    int
	treeClickPath string
	treeClickAt   time.Time
	gotoHits      []string
	recent        []string
	searchHits    []search.Result
	searchErr     string
	watchLimited  bool

	ih            intel.Host
	diags         map[string][]bus.Diagnostic
	compItems     []bus.CompletionItem
	locHits       []bus.Location
	symHits       []bus.Symbol
	navBack       []bus.Location
	navFwd        []bus.Location
	pendingJump   bus.Location
	pendingJumpOn bool
	dialogReply   chan<- int
	dialogActions []string
	dialogMsg     string

	terms       []*termTab
	termActive  int
	nextTerm    uint64
	pasteToTerm bool
	termPaste   struct{}

	// termH is the terminal panel height in pixels. Zero uses the 240 dp default.
	// termSashY is the resize bar's window Y from the previous frame.
	termH       int
	termSashY   int
	termSashH   int
	termCenterH int
	termFindH   int
	termDrag    bool
	termDragID  pointer.ID
	termDragY   int
	termDragH   int
	termSashHot bool
	termSash    termTag
	termNew     termTag
	// termSelect and termCloseAt are tab-strip clicks from this frame.
	// -1 means none. They are applied after the strip lays out.
	termSelect   int
	termCloseAt  int
	termTabLeft  []int
	termPlusLeft int

	gitMu        sync.Mutex
	gitRepo      *git.Repo
	gitFiles     []bus.GitFile
	gitNotice    string
	gitBranches  []git.Branch
	gitTimer     *time.Timer
	gitScroll    int
	gutterTimers map[uint64]*time.Timer

	// gitDiffPath is the status path shown in the editor well.
	// Empty means the editor shows the buffer.
	gitDiffPath  string
	gitDiffSeq   uint64
	gitDiffRows  []bus.GitDiffRow
	gitDiffNote  string
	gitDiffX     int
	gitDiffY     int
	gitDiffTimer *time.Timer
}

func newShell(project, version string) (*shell, error) {
	if project != "" {
		abs, err := filepath.Abs(project)
		if err != nil {
			return nil, err
		}
		project = abs
	}
	lg, err := xdg.OpenLog()
	if err != nil {
		log.Printf("gio-ide: log: %v", err)
	} else {
		log.SetOutput(io.MultiWriter(os.Stderr, lg))
	}
	user := xdg.SettingsPath()
	var proj string
	if project != "" {
		proj = xdg.ProjectSettings(project)
	}
	store := settings.NewStore(user, proj)
	cur := store.Current()
	if _, _, err := runtimemem.Apply(cur.Runtime.MemoryLimit); err != nil {
		log.Printf("gio-ide: memory limit: %v", err)
	}
	themeName := cur.UI.Theme
	th, err := loadTheme(cur.UI.Theme)
	if err != nil {
		log.Printf("gio-ide: theme: %v", err)
		th = theme.Dark()
		themeName = "dark"
	}
	km, err := keymap.Load(xdg.KeymapPath(), keymap.Default())
	if err != nil {
		log.Printf("gio-ide: keymap: %v", err)
		km = keymap.Default()
	}
	s := &shell{
		version:      version,
		root:         project,
		scratchDir:   xdg.ScratchesDir(),
		window:       new(app.Window),
		bus:          bus.New(nil),
		store:        store,
		theme:        th,
		themeName:    themeName,
		keys:         km,
		cmds:         commands.New(),
		shaper:       text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection())),
		logf:         lg,
		stop:         make(chan struct{}),
		leftOpen:     true,
		leftTool:     "project",
		intel:        "stopped",
		line:         1,
		col:          1,
		branch:       "—",
		tags:         map[string]*handlerTag{},
		onCommand:    map[string]func(){},
		docs:         map[uint64]*buffer.Doc{},
		opening:      map[string]bool{},
		saveVersions: map[uint64]int32{},
		formatToken:  map[uint64]int64{},
		rootPane:     &pane{ratio: 0.5},
		colors:       map[uint64]*docPaint{},
		marks:        map[uint64][]editor.Mark{},
	}
	s.syn = syntax.New(s.bus.Post)
	s.focusPane = s.rootPane
	s.chord = keymap.Resolver{Map: s.keys}
	s.bus.SetInvalidate(s.window.Invalidate)
	s.window.Option(
		app.Title(s.title()),
		app.Size(unit.Dp(1280), unit.Dp(800)),
		app.MinSize(unit.Dp(720), unit.Dp(480)),
	)
	s.bindCommands()
	s.bindTerminal()
	s.startIntel(cur)
	if s.root != "" {
		go s.loadWorkspace()
	}
	for _, p := range cur.Problems {
		s.problems = append(s.problems, p.Key+": "+p.Msg)
	}
	store.OnChange(func(next settings.Settings) {
		s.bus.Post(bus.SettingsChanged{Theme: next.UI.Theme})
		if len(next.Problems) > 0 {
			s.bus.Post(bus.Notification{Level: "warn", Text: fmt.Sprintf("%d settings warning(s)", len(next.Problems))})
		}
	})
	_ = store.Watch(s.stop)
	runtimemem.Start(5*time.Second, s.stop, func(sample runtimemem.Sample) {
		s.bus.Post(bus.MemorySample{Heap: sample.Heap, RSS: sample.RSS, Limit: sample.Limit, Ratio: sample.Ratio})
		if sample.Limit > 0 && sample.Ratio >= 0.90 {
			s.bus.Post(bus.Notification{Level: "warn", Text: "Memory is above 90% of the limit. Run Restart Code Intelligence."})
		}
	})
	// Prime one sample so the status bar is not blank before the first tick.
	s.mem = runtimemem.Read()
	log.Printf("gio-ide %s started root=%q", version, project)
	return s, nil
}

func (s *shell) title() string {
	if s.root == "" {
		return "gio-ide"
	}
	return filepath.Base(s.root) + " — gio-ide"
}

// handlerTag is one Gio pointer and key target.
// The byte keeps each allocation at its own address. A zero-size value is
// always the same pointer, so every widget would share one handler and the
// first pressed widget would consume every click.
type handlerTag struct{ _ byte }

// tagFor returns the stable input tag for name.
func (s *shell) tagFor(name string) *handlerTag {
	if t, ok := s.tags[name]; ok {
		return t
	}
	t := &handlerTag{}
	s.tags[name] = t
	return t
}

func (s *shell) loop() {
	var ops op.Ops
	for {
		switch e := s.window.Event().(type) {
		case app.DestroyEvent:
			s.shutdownTerminals()
			s.shutdownIntel()
			if s.syn != nil {
				s.syn.Shutdown()
			}
			close(s.stop)
			if s.logf != nil {
				s.logf.Close()
			}
			os.Exit(0)
		case app.FrameEvent:
			start := time.Now()
			gtx := app.NewContext(&ops, e)
			s.frame(gtx)
			if d := time.Since(start); d > 100*time.Millisecond {
				noteSlowFrame(d)
			}
			e.Frame(gtx.Ops)
		}
	}
}

func (s *shell) frame(gtx layout.Context) {
	s.drain()
	s.syncCaret()
	tag := s.inputTag()
	if !s.focused || s.heldTag != tag {
		gtx.Execute(key.FocusCmd{Tag: tag})
		s.focused = true
		s.heldTag = tag
	}
	s.handleKeys(gtx)
	s.flushClipboard(gtx)
	s.publishEdits()
	s.layout(gtx)
}

func (s *shell) drain() {
	s.bus.Drain(func(e bus.Event) {
		switch ev := e.(type) {
		case bus.MemorySample:
			s.mem = runtimemem.Sample{Heap: ev.Heap, RSS: ev.RSS, Limit: ev.Limit, Ratio: ev.Ratio}
		case bus.ThemeChanged:
			s.applyTheme(ev.Name)
		case bus.Notification:
			s.notice = ev.Text
			log.Printf("gio-ide: %s", ev.Text)
		case bus.DocumentOpened:
			s.attachOpened(ev.DocID)
		case bus.DocumentSaved:
			s.onSaved(ev.DocID)
		case bus.FormatResult:
			s.onFormat(ev)
		case bus.OpenFileRequest:
			if ev.Line > 0 {
				s.pendingJump = bus.Location{Path: ev.Path, Line: ev.Line, Col: 1}
				s.pendingJumpOn = true
			}
			s.revealPath(ev.Path)
		case bus.SyntaxSpans:
			s.applySpans(ev)
		case bus.FoldRanges:
			s.applyFolds(ev)
		case bus.BracketPair:
			s.applyBracket(ev)
		case bus.FileListUpdated:
			s.noteFileList()
		case bus.SearchResultsBatch:
			for _, h := range ev.Results {
				s.searchHits = append(s.searchHits, search.Result{
					Path: h.Path, Line: h.Line, Col: h.Col, LineText: h.LineText,
					MatchStart: h.MatchStart, MatchEnd: h.MatchEnd,
				})
			}
		case bus.SearchDone:
			s.searchErr = ev.Err
			if ev.Err != "" {
				s.notice = ev.Err
			}
		case bus.IntelStateChanged:
			s.intel = ev.State
			s.intelDetail = ev.Detail
		case bus.DiagnosticsUpdated:
			s.onDiagnostics(ev)
		case bus.CompletionReady:
			s.onCompletion(ev)
		case bus.HoverReady:
			s.onHover(ev)
		case bus.LocationsReady:
			s.onLocations(ev)
		case bus.SymbolsReady:
			s.onSymbols(ev)
		case bus.DialogRequest:
			s.onDialog(ev)
		case bus.FileSystemChanged:
			if s.ih != nil && len(ev.Changes) > 0 {
				s.ih.FilesChanged(ev.Changes)
			}
			s.scheduleGitStatus()
		case bus.DocumentChanged:
			s.scheduleGutter(ev.DocID)
			s.noteGitDiffDoc(ev.DocID)
		case bus.GitGutterUpdated:
			s.onGutter(ev)
		case bus.GitDiffUpdated:
			s.onGitDiff(ev)
		case bus.SettingsChanged:
			cur := s.store.Current()
			if km, err := keymap.Load(xdg.KeymapPath(), keymap.Default()); err == nil {
				s.keys = km
				s.chord.Map = km
				s.chord.Clear()
			}
			if s.ih != nil {
				s.ih.Configure(cur.GoplsConfig())
			}
			s.rebuildProblems()
			s.applyTheme(ev.Theme)
		case bus.TerminalExited:
			s.onTerminalExit(ev)
		case bus.GitStatusUpdated:
			s.onGitStatus(ev)
		}
	})
}

func (s *shell) handleKeys(gtx layout.Context) {
	tag := s.inputTag()
	// The key target is not a pointer hit area. Without pass-through, this
	// unclipped event.Op covers the window and swallows presses on the tree.
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	pass.Pop()
	filters := s.keyFilters(tag)
	var batch []event.Event
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		batch = append(batch, ev)
	}
	// A printable key on Linux is a key event and an edit event.
	// Insert the edit once.
	ime := s.editorHasEdit(batch)
	for _, ev := range batch {
		switch e := ev.(type) {
		case key.Event:
			s.onKey(e, ime)
		case key.EditEvent:
			if v := s.activeView(); v != nil && s.focus == focusEditor && s.popup == "" && !s.markdownPreview() {
				v.ReplaceRunes(e.Range.Start, e.Range.End, e.Text)
			}
		case key.FocusEvent:
			s.focused = e.Focus
			if e.Focus && s.watchLimited && s.index != nil {
				s.watchLimited = false
				go func(ix workspace.Index) {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := ix.Refresh(ctx); err == nil {
						s.bus.Post(bus.FileListUpdated{Root: s.root, Count: len(ix.Files())})
					}
				}(s.index)
			}
			if !e.Focus {
				gtx.Execute(key.FocusCmd{Tag: tag})
				s.heldTag = tag
			}
		}
	}
}

func (s *shell) keyFilters(tag event.Tag) []event.Filter {
	mods := key.ModCtrl | key.ModAlt | key.ModShift | key.ModSuper | key.ModCommand
	names := []key.Name{
		key.NameTab, key.NameEscape, key.NameReturn, key.NameEnter,
		key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow,
		key.NameHome, key.NameEnd, key.NameDeleteBackward, key.NameDeleteForward,
		key.NamePageUp, key.NamePageDown, key.NameSpace,
		key.NameF1, key.NameF2, key.NameF3, key.NameF4, key.NameF5, key.NameF6,
		key.NameF7, key.NameF8, key.NameF9, key.NameF10, key.NameF11, key.NameF12,
	}
	out := []event.Filter{
		key.Filter{Focus: tag, Optional: mods},
		key.FocusFilter{Target: tag},
	}
	for _, n := range names {
		out = append(out, key.Filter{Focus: tag, Name: n, Optional: mods})
	}
	return out
}

// editorHasEdit reports whether batch contains text the editor will insert.
// An empty edit is a deletion and stays on the key path.
func (s *shell) editorHasEdit(batch []event.Event) bool {
	if s.focus != focusEditor || s.popup != "" || s.markdownPreview() || s.activeView() == nil {
		return false
	}
	for _, ev := range batch {
		e, ok := ev.(key.EditEvent)
		if ok && e.Text != "" {
			return true
		}
	}
	return false
}

// onKey handles one key.
// When ime is true, this frame also has an editor edit event.
// Do not insert that printable key a second time.
func (s *shell) onKey(e key.Event, ime bool) {
	st, ok := strokeFrom(e)
	if !ok {
		return
	}
	if s.popup != "" {
		if s.popupKey(st) {
			return
		}
	}
	ctx := s.keyContext()
	if ctx.TerminalFocus && !keymap.TerminalPassthrough(st) {
		s.writeTerminal(st)
		return
	}
	cmd, more := s.chord.Push(st, ctx, time.Now())
	if more || cmd == "" {
		if cmd == "" && !more && s.popup == "" {
			if s.gitDiffPath != "" && (s.focus == focusEditor || s.focus == focusTree) {
				if st.Key == "escape" && st.Mod == 0 {
					s.clearGitDiff()
					return
				}
				if s.diffKey(st) {
					return
				}
				if s.focus == focusEditor {
					return
				}
			}
			if s.focus == focusFind {
				s.findInput(st)
				return
			}
			if s.focus == focusEditor {
				if s.markdownPreview() {
					s.previewKey(st)
					return
				}
				if s.editorKey(st) {
					return
				}
				if text := insertText(st); text != "" && !ime {
					s.insertEditorText(text)
				}
			}
		}
		return
	}
	s.runCommand(cmd)
}

func (s *shell) keyContext() keymap.Context {
	v := s.activeView()
	return keymap.Context{
		EditorFocus:   s.focus == focusEditor && s.popup == "",
		TerminalFocus: s.focus == focusTerminal && s.popup == "",
		TreeFocus:     s.focus == focusTree && s.popup == "",
		PopupOpen:     s.popup != "",
		HasSelection:  v != nil && v.HasSelection(),
	}
}

// revealList keeps the command-palette selection on the visible page.
// listVisible is 0 until layout, so the page falls back to 12 rows.
func (s *shell) revealList() {
	vis := s.listVisible
	if vis < 1 {
		vis = 12
	}
	if s.listIndex < 0 {
		s.listIndex = 0
	}
	if s.listIndex < s.listScroll {
		s.listScroll = s.listIndex
	}
	if s.listIndex >= s.listScroll+vis {
		s.listScroll = s.listIndex - vis + 1
	}
	if s.listScroll < 0 {
		s.listScroll = 0
	}
}

// clampActionIndex limits Find Action to the filtered catalog.
// Other popups are left alone. A nil registry skips the clamp.
// An empty filter selects row 0.
func (s *shell) clampActionIndex() {
	if s.popup != "action" || s.cmds == nil {
		return
	}
	n := len(s.filteredCommands())
	if n == 0 {
		s.listIndex = 0
		return
	}
	if s.listIndex < 0 {
		s.listIndex = 0
	}
	if s.listIndex > n-1 {
		s.listIndex = n - 1
	}
}

func (s *shell) popupKey(st keymap.Stroke) bool {
	if st.Mod == keymap.ModCtrl|keymap.ModShift && st.Key == "a" && s.popup == "action" {
		s.closePopup()
		return true
	}
	if s.popup == "settings" && s.keyCapture != "" {
		if st.Key == "escape" {
			s.keyCapture = ""
			s.visualDirty = true
			return true
		}
		s.bindCaptured(st)
		return true
	}
	if st.Key == "escape" && st.Mod == 0 {
		s.closePopup()
		return true
	}
	// Settings is a button panel. Ctrl, Alt, Super, and a bound command reach
	// the chord map. Anything else must not type into the filter or hit Enter,
	// which would close the card.
	if s.popup == "settings" {
		const chord = keymap.ModCtrl | keymap.ModAlt | keymap.ModSuper
		if st.Mod&chord != 0 {
			return false
		}
		if s.keys != nil {
			if cmd, _ := s.keys.Lookup([]keymap.Stroke{st}, s.keyContext()); cmd != "" {
				return false
			}
		}
		return true
	}
	switch st.Key {
	case "backspace":
		if st.Mod == 0 && s.fieldCaret > 0 {
			runes := []rune(s.field)
			if s.fieldCaret > len(runes) {
				s.fieldCaret = len(runes)
			}
			runes = append(runes[:s.fieldCaret-1], runes[s.fieldCaret:]...)
			s.fieldCaret--
			s.field = string(runes)
			s.listIndex = 0
			s.listScroll = 0
			s.onPopupQuery()
		}
		return true
	case "delete":
		runes := []rune(s.field)
		if st.Mod == 0 && s.fieldCaret < len(runes) {
			runes = append(runes[:s.fieldCaret], runes[s.fieldCaret+1:]...)
			s.field = string(runes)
		}
		return true
	case "left":
		if s.fieldCaret > 0 {
			s.fieldCaret--
		}
		return true
	case "right":
		if s.fieldCaret < len([]rune(s.field)) {
			s.fieldCaret++
		}
		return true
	case "home":
		s.fieldCaret = 0
		return true
	case "end":
		s.fieldCaret = len([]rune(s.field))
		return true
	case "up":
		if s.listIndex > 0 {
			s.listIndex--
		}
		s.clampActionIndex()
		s.revealList()
		return true
	case "down":
		s.listIndex++
		s.clampActionIndex()
		s.revealList()
		return true
	case "enter":
		s.acceptPopup()
		return true
	}
	if txt := insertText(st); txt != "" {
		runes := []rune(s.field)
		if s.fieldCaret > len(runes) {
			s.fieldCaret = len(runes)
		}
		runes = append(runes[:s.fieldCaret], append([]rune(txt), runes[s.fieldCaret:]...)...)
		s.fieldCaret += len([]rune(txt))
		s.field = string(runes)
		s.listIndex = 0
		s.listScroll = 0
		s.onPopupQuery()
		return true
	}
	return false
}

func (s *shell) closePopup() {
	if s.popup == "dialog" && s.dialogReply != nil {
		s.dialogReply <- -1
		s.dialogReply = nil
	}
	s.popup = ""
	s.field = ""
	s.fieldCaret = 0
	s.listIndex = 0
	s.listScroll = 0
	s.keyCapture = ""
	s.keyScroll = 0
	if s.focus == focusPopup {
		s.focus = focusEditor
	}
}

func (s *shell) onPopupQuery() {
	switch s.popup {
	case "goto":
		s.updateGoto()
	case "symbol":
		if s.ih != nil {
			s.ih.WorkspaceSymbols(s.field)
		}
	}
}

func (s *shell) onDialog(ev bus.DialogRequest) {
	if s.dialogReply != nil {
		s.dialogReply <- -1
		s.dialogReply = nil
	}
	s.dialogReply = ev.Reply
	s.dialogActions = ev.Actions
	if len(s.dialogActions) == 0 {
		s.dialogActions = []string{"OK"}
	}
	s.dialogMsg = ev.Message
	s.notice = ev.Message
	s.openPopup("dialog")
}

func (s *shell) openPopup(kind string) {
	s.popup = kind
	s.field = ""
	s.fieldCaret = 0
	s.listIndex = 0
	s.listScroll = 0
	s.focus = focusPopup
	switch kind {
	case "goto":
		s.updateGoto()
	case "recent":
		s.loadRecent()
	}
}

func (s *shell) acceptPopup() {
	switch s.popup {
	case "action":
		items := s.filteredCommands()
		if len(items) == 0 {
			return
		}
		if s.listIndex >= len(items) {
			s.listIndex = 0
		}
		id := items[s.listIndex].ID
		s.closePopup()
		s.runCommand(id)
	case "unsaved":
		idx := s.listIndex
		s.closePopup()
		s.answerUnsaved(idx)
	case "goto":
		if len(s.gotoHits) == 0 {
			return
		}
		if s.listIndex >= len(s.gotoHits) {
			s.listIndex = 0
		}
		rel := s.gotoHits[s.listIndex]
		s.closePopup()
		s.openPath(filepath.Join(s.root, filepath.FromSlash(rel)))
	case "recent":
		if len(s.recent) == 0 {
			return
		}
		if s.listIndex >= len(s.recent) {
			s.listIndex = 0
		}
		path := s.recent[s.listIndex]
		s.closePopup()
		s.openPath(path)
	case "search":
		s.runProjectSearch(false)
		s.closePopup()
	case "replaceFiles":
		s.runProjectSearch(true)
		s.closePopup()
	case "commit":
		msg := s.field
		s.closePopup()
		s.doCommit(msg)
	case "renameScratch":
		name := s.field
		s.closePopup()
		s.renameScratch(name)
	case "moveScratch":
		rel := s.field
		s.closePopup()
		s.moveScratch(rel)
	case "branches":
		name := strings.TrimSpace(s.field)
		create := false
		if name == "" {
			if s.listIndex >= 0 && s.listIndex < len(s.gitBranches) {
				name = s.gitBranches[s.listIndex].Name
			}
		} else {
			create = true
			for _, b := range s.gitBranches {
				if b.Name == name {
					create = false
				}
			}
		}
		s.closePopup()
		if name != "" {
			s.doSwitch(name, create)
		}
	case "completion":
		s.acceptCompletion()
	case "usages", "symbol", "structure":
		s.acceptLocation(s.listIndex)
	case "dialog":
		i := s.listIndex
		if s.dialogReply != nil {
			s.dialogReply <- i
			s.dialogReply = nil
		}
		s.popup = ""
		s.field = ""
		s.fieldCaret = 0
		s.listIndex = 0
		s.focus = focusEditor
	default:
		s.closePopup()
	}
}

func (s *shell) filteredCommands() []commands.Command {
	q := strings.ToLower(strings.TrimSpace(s.field))
	var out []commands.Command
	for _, c := range s.cmds.All() {
		if c.Phase != "" && q == "" {
			continue
		}
		hay := strings.ToLower(c.Title + " " + c.ID)
		if q == "" || strings.Contains(hay, q) {
			out = append(out, c)
		}
	}
	return out
}

func (s *shell) runCommand(id string) {
	if fn := s.onCommand[id]; fn != nil {
		fn()
		return
	}
	if !s.cmds.Execute(id) {
		s.notice = "Unknown command " + id
	}
}

func (s *shell) insertEditorText(text string) {
	v := s.activeView()
	if v == nil {
		return
	}
	v.Type(text)
}

// applyTheme installs name when it is not already showing.
// A failed load leaves the current theme in place so the same name is tried again.
func (s *shell) applyTheme(name string) {
	if name == "" || name == s.themeName {
		return
	}
	th, err := loadTheme(name)
	if err != nil {
		s.notice = err.Error()
		return
	}
	s.themeName = name
	s.themeGen++
	s.theme = th
}

func loadTheme(name string) (*theme.Theme, error) {
	if name == "" || name == "dark" || name == "light" {
		return theme.ByName(name)
	}
	path := xdg.ConfigPath("themes", name+".toml")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("theme %q: %w", name, err)
	}
	return theme.Load(path)
}

func (s *shell) notify(text string) {
	s.bus.Post(bus.Notification{Level: "info", Text: text})
}
