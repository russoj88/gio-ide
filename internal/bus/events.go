package bus

// Event is a typed message from a worker to the UI, or between services.
type Event interface{ isEvent() }

type DocumentOpened struct {
	DocID   uint64
	Path    string
	Version int32
}

type DocumentChanged struct {
	DocID   uint64
	Version int32
	Path    string
}

type DocumentSaved struct {
	DocID uint64
	Path  string
}

// FormatResult is the formatted text for one document.
// Data is nil when Err is set. Token matches the request that produced it.
type FormatResult struct {
	DocID       uint64
	BaseVersion int32
	Token       int64
	Path        string
	Data        []byte
	Err         string
	SaveAfter   bool
}

type DocumentClosed struct {
	DocID uint64
	Path  string
}

// StyleSpan is one colored range inside a line. Start and End are byte columns.
type StyleSpan struct {
	Start, End int
	Style      string
}

type SyntaxSpans struct {
	DocID     uint64
	Version   int32
	FirstLine int
	Spans     [][]StyleSpan
}

type BracketPair struct {
	DocID   uint64
	Version int32
	Open    int
	Close   int
	OK      bool
}

// FoldRange is one collapsible region. Start and End are inclusive buffer
// lines. The header line Start stays visible. Suffix is drawn after the
// ellipsis when the range is collapsed, and is empty when there is nothing
// to show there.
type FoldRange struct {
	Start, End int
	Suffix     string
}

// FoldRanges is the full set of folds for one document version.
type FoldRanges struct {
	DocID   uint64
	Version int32
	Ranges  []FoldRange
}

type Diagnostic struct {
	Start, End int
	Line, Col  int
	Message    string
	Severity   int // 1 error, 2 warning, 3 info, 4 hint
	Source     string
}

type DiagnosticsUpdated struct {
	URI     string
	Version int32
	Items   []Diagnostic
}

type CompletionItem struct {
	Label       string
	Detail      string
	Insert      string
	Snippet     bool
	Kind        string
	TextEdits   []TextEdit
	ReplaceFrom int
	ReplaceTo   int
}

type TextEdit struct {
	Start, End int
	Text       string
}

type CompletionReady struct {
	ID      uint64
	DocID   uint64
	Version int32
	At      int
	Items   []CompletionItem
	Err     string
}

type Location struct {
	Path      string
	URI       string
	Line, Col int
	Offset    int
	EndLine   int
	EndCol    int
	EndOffset int
}

type LocationsReady struct {
	ID        uint64
	Kind      string // "definition" or "references"
	DocID     uint64
	Locations []Location
	Err       string
}

type HoverReady struct {
	ID       uint64
	DocID    uint64
	Version  int32
	Markdown string
	Err      string
}

type Symbol struct {
	Name      string
	Kind      string
	Path      string
	Line, Col int
	Container string
}

type SymbolsReady struct {
	ID      uint64
	Kind    string // "document" or "workspace"
	DocID   uint64
	Query   string
	Symbols []Symbol
	Err     string
}

type IntelStateChanged struct {
	State  string
	Detail string
}

type FileListUpdated struct {
	Root  string
	Count int
}

type FileChange struct {
	Path string
	Kind string // create, write, remove, rename
}

type FileSystemChanged struct {
	Changes []FileChange
}

type SearchHit struct {
	Path                 string
	Line, Col            int
	LineText             string
	MatchStart, MatchEnd int
}

type SearchResultsBatch struct {
	Results []SearchHit
}

type SearchDone struct {
	Err       string
	Truncated bool
}

type GitFile struct {
	Path          string
	Staging, Work byte
}

type GitStatusUpdated struct {
	Branch string
	Files  []GitFile
}

type GutterMark struct {
	Line int
	Kind byte // 'a', 'm', 'd'
}

type GitGutterUpdated struct {
	DocID uint64
	Path  string
	Lines []GutterMark
}

// GitDiffSpan is a highlighted byte range inside one side of a row.
type GitDiffSpan struct {
	Start int
	End   int
	Kind  byte
}

// GitDiffSide is one column of a side-by-side row.
// Line is 1-based, or 0 for a pad. Kind is ' ', '+', '-', '~', or 0.
type GitDiffSide struct {
	Line  int
	Text  string
	Kind  byte
	Spans []GitDiffSpan
}

// GitDiffRow is one aligned pair. Left is HEAD. Right is the current file.
type GitDiffRow struct {
	Left, Right GitDiffSide
}

// GitDiffUpdated is a diff for one status path.
// Seq drops a result that lost to a newer click or edit.
// Note is set when Rows is empty: binary, too large, no changes, or an error.
type GitDiffUpdated struct {
	Seq  uint64
	Path string
	Rows []GitDiffRow
	Note string
}

type TerminalDirty struct {
	Session uint64
}

type TerminalExited struct {
	Session uint64
	Err     string
}

type MemorySample struct {
	Heap  uint64
	RSS   uint64
	Limit int64
	Ratio float64
}

// SettingsChanged reports that the merged settings were reloaded.
// Theme is the effective theme name. The shell applies it when it differs
// from the theme already on screen.
type SettingsChanged struct {
	Theme string
}

type ThemeChanged struct {
	Name string
}

type Notification struct {
	Level string // info, warn, error
	Text  string
}

// DialogRequest is posted by the intel client for ShowMessageRequest.
// The UI answers on Reply. The waiter uses the call context.
type DialogRequest struct {
	Title   string
	Message string
	Actions []string
	Reply   chan<- int // index, or -1 on dismiss
}

type OpenFileRequest struct {
	Path   string
	Line   int
	Select bool
}

func (DocumentOpened) isEvent()     {}
func (DocumentChanged) isEvent()    {}
func (DocumentSaved) isEvent()      {}
func (FormatResult) isEvent()       {}
func (DocumentClosed) isEvent()     {}
func (SyntaxSpans) isEvent()        {}
func (BracketPair) isEvent()        {}
func (FoldRanges) isEvent()         {}
func (DiagnosticsUpdated) isEvent() {}
func (CompletionReady) isEvent()    {}
func (LocationsReady) isEvent()     {}
func (HoverReady) isEvent()         {}
func (SymbolsReady) isEvent()       {}
func (IntelStateChanged) isEvent()  {}
func (FileListUpdated) isEvent()    {}
func (FileSystemChanged) isEvent()  {}
func (SearchResultsBatch) isEvent() {}
func (SearchDone) isEvent()         {}
func (GitStatusUpdated) isEvent()   {}
func (GitGutterUpdated) isEvent()   {}
func (GitDiffUpdated) isEvent()     {}
func (TerminalDirty) isEvent()      {}
func (TerminalExited) isEvent()     {}
func (MemorySample) isEvent()       {}
func (SettingsChanged) isEvent()    {}
func (ThemeChanged) isEvent()       {}
func (Notification) isEvent()       {}
func (DialogRequest) isEvent()      {}
func (OpenFileRequest) isEvent()    {}
