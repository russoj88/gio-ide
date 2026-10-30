# gio-ide user manual

This manual is the procedure for every command and surface in the current Linux build. A person follows **Do**, **See**, and **Pass** in the window. An agent follows those plus **Agent**, the [protocol](#13-agent-protocol), and the [smoke list](#14-smoke-list).

Quoted strings are the text the program paints or logs. Command ids are the values in `internal/commands`. Default strokes are the values in `keymap.defaultRows`.

Headless tests do not call `app.Run`. Closing a real window runs `os.Exit(0)`. Drive a partial shell or an `editor.View` with `internal/harness.Desktop`, and set `XDG_CONFIG_HOME` to a temp directory before anything writes settings or the keymap.

## Contents

1. [Run the editor](#1-run-the-editor)
2. [Screen map](#2-screen-map)
3. [Project tree and opening files](#3-project-tree-and-opening-files)
4. [Editing](#4-editing)
5. [Find and replace](#5-find-and-replace)
6. [Save, close, and splits](#6-save-close-and-splits)
7. [Navigation and code intelligence](#7-navigation-and-code-intelligence)
8. [Git](#8-git)
9. [Terminal](#9-terminal)
10. [Find Action, Settings, keymap, and theme](#10-find-action-settings-keymap-and-theme)
11. [Command catalog](#11-command-catalog)
12. [Settings keys](#12-settings-keys)
13. [Agent protocol](#13-agent-protocol)
14. [Smoke list](#14-smoke-list)

## 1. Run the editor

**Do.** On Linux, install the Gio libraries from the README apt list, use Go 1.27.2, and export `CGO_ENABLED=1`. Then:

```bash
go run ./cmd/gio-ide /path/to/module
```

**See.** A window at 1280×800 dp (minimum 720×480). The title is `gio-ide` with no argument, or `<folder base> — gio-ide` when a path is open. The left Project panel starts open. The bottom panel is closed. The status bar reads branch `—` until a repo is open, caret `1:1`, `0 errors  0 warnings`, an intel cell, a heap cell, and `gio-ide <version>`.

**Pass.**

| Invocation | Result |
|---|---|
| `-version` | Prints `gio-ide <version>` and returns. `go run` without ldflags prints `gio-ide dev`. |
| A flag parse error | Exit 2. |
| A path that `os.Stat` rejects | Exit 2, and stderr starts with `gio-ide:`. |
| `app.Run` returns an error | Exit 1. |
| A directory argument | That directory is the workspace. |
| A file argument | That file opens, and its parent is the workspace. |
| No argument | Empty window. Project body is `Open a folder from the command line to see its files.` |
| Extra arguments | Only the first path is used. |

`go` on `PATH` is required for useful code intelligence. The editor still opens when `go` is missing. With a folder open, the welcome notice is `The go command is not on PATH. Editing still works. Code intelligence needs Go.`

**Agent.** Build UI tests with `CGO_ENABLED=1`. Do not call `app.Run` or `newShell` under `go test`. Construct the `*shell` fields the assertion needs. Process messages land in `$XDG_STATE_HOME/gio-ide/logs/gio-ide.log` (default `~/.local/state/gio-ide/logs/gio-ide.log`). A notice is also `log.Printf("gio-ide: %s", text)`.

Config files:

| File | Path |
|---|---|
| User settings | `$XDG_CONFIG_HOME/gio-ide/settings.toml` (default `~/.config/gio-ide/settings.toml`) |
| Keymap | `$XDG_CONFIG_HOME/gio-ide/keymap.toml` |
| Custom theme | `$XDG_CONFIG_HOME/gio-ide/themes/<name>.toml` |
| Recent files | `$XDG_STATE_HOME/gio-ide/recent.toml` (50 paths) |
| Project settings | `<root>/.gio-ide/settings.toml` |

Load order is defaults, then the user file, then the project file. The project file wins. The Settings card writes changed keys to the user file only, then reloads both layers. The app does not write the project file.

Where a message appears:

- `s.notice` is painted on the welcome page in `diag.warning`, and only there.
- With a buffer open, the same text is in the log and in `s.notice`. It is not drawn in the chrome.
- A gopls `ShowMessageRequest` opens a card titled `Code intelligence`. The first line of the message sits above the action rows. Enter runs the selected row. Escape closes the card.
- A read-only buffer paints `This file is binary or not valid UTF-8. It is read-only.` inside the editor.
- The status bar is the place to read branch, caret, diagnostic counts, intel state, heap, and version.

## 2. Screen map

There is no menu bar. From the left, at the default size and a scale of 1 px per dp:

| Region | Size | What it shows |
|---|---|---|
| Tool strip | 40 dp wide | **P** Project, **G** Git, **!** Problems. Run and Debug are not drawn. |
| Left panel | 260 dp, starts at x=41 | Project, Git, or Problems. Starts open on Project. |
| Editor | the rest | The active buffer, or the welcome page when that leaf has no buffer. |
| Bottom panel | closed; 180 dp for Problems and Search. The terminal starts at 240 dp. | Problems, Search, or Terminal. Drag the bar above the terminal to resize it. Click a terminal tab to switch. `x` closes that tab. `+` or Ctrl+T opens another. |
| Status bar | one row | Not clickable. |
| Popup | card 560×420 dp, Settings 520 tall, centered | Dim overlay, alpha 120. On 1280×800 the Settings origin is (360, 140). |

**Do.** Alt+1 or click **P** (`view.projectTool`). Alt+9 or click **G** (`view.gitTool`). Alt+6 or click **!** (`view.problemsTool`).

**See.** P and G toggle. The panel closes when that tool is already open, and tree focus returns to the editor. Problems opens the left list and the bottom panel together, and a second Alt+6 leaves them open. The active strip button uses `ui.selection`.

**Pass.** After the P click, `leftOpen` is true and `leftTool` is `"project"`. The P center at scale 1 is `((40-1)/2, (40-1)/2)`, which is (19, 19).

**Agent.** `TestLayoutOpensProjectTool` in `internal/app/layout_test.go` clicks that point. Tree clicks need `handleKeys` in the same frame. That key op is wrapped in `pointer.PassOp`. An unclipped key op covers the window and swallows the row.

Welcome page, when the focused leaf has no buffer: the folder base name and full path when a folder is open, otherwise the word `gio-ide`, then the tagline `A Go editor with completion, navigation, a terminal, and Git built in.`, then three hints:

- `Find Action  Ctrl+Shift+A`
- `Go to File  Ctrl+Shift+N`
- `Terminal  Alt+F12`

A non-empty `s.notice` is an extra line under those hints.

There is no tab strip. `layoutLeaf` draws the active buffer only. Several buffers can be open. Alt+Right (`nav.nextTab`) and Alt+Left (`nav.prevTab`) change which one is visible. The status bar does not name the file. Check the active document path.

Popup keys, other than Settings: Escape closes. Backspace, Delete, Left, Right, Home, and End edit the field. Up and Down move the row. Enter accepts. A printable key appends. There is no click-outside dismiss. Palette rows, search hits, git rows, and problem rows are not click targets. Terminal tabs and the terminal + are click targets. The unsaved dialog rows and the Settings controls are click targets.

## 3. Project tree and opening files

**Do.** Open a folder from the command line. Click a directory row to expand it. Click a file row to open it. Or press Ctrl+Shift+N (`nav.goToFile`) and Enter on a row.

**See.** Directories start collapsed. A directory name is dim, with a drawn triangle (the font has no disclosure glyph). Each row has a drawn mark after that slot: a folder, or a file mark chosen from the name. The mark is 16 dp and uses existing theme colors. A file name uses `ui.foreground`. Indent is 14 dp per depth. Row height is 22 dp. The wheel scrolls 3 rows. With an index and no rows expanded, the first row sits near y=30. The hit target is the full row, left aligned.

**Pass.**

| State | On screen |
|---|---|
| No workspace | `Open a folder from the command line to see its files.` in dim type, above the tree. |
| Workspace, empty index | The root path, in dim type, above the tree. |
| Workspace with files | The Project title and the tree. The root path is not repeated. |
| Directory click | `openDirs[path]` flips, focus stays on the tree, and the new rows paint in that frame. |
| Same path again within 300 ms | The folder stays as the first click left it. `treeClickGap` uses `time.Now`. |
| File click | That file is the active buffer and focus is the editor. |
| Path already open | That buffer is focused. The file is not loaded again. |

The index skips `files.exclude` (default `node_modules/`) and gitignore patterns. A tracked path is still listed. `files.watch` defaults to true. When inotify hits its limit, the welcome notice is `File watching stopped (fs.inotify.max_user_watches). The tree rescans when the window is focused.`

A NUL in the first 8192 bytes, or bytes that are not valid UTF-8, opens read-only. The editor paints `This file is binary or not valid UTF-8. It is read-only.` Typing does nothing.

Opening also happens from Recent Files, the launch path, and `bus.OpenFileRequest`. Each open is recorded in the recent list.

Scratches live in `$XDG_STATE_HOME/gio-ide/scratches` (default `~/.local/state/gio-ide/scratches`). Every project shares that directory. The project tree lists them under a `Scratches` folder, after the project rows. The folder starts collapsed. Its row is not a filesystem path. A click toggles it with the same 300 ms gap as other directories. A scratch file row uses the absolute path. Clicking it opens that path and focuses the editor. It is not joined onto the workspace root.

Ctrl+Alt+N (`file.newScratch`) creates a Go scratch and opens it. The buffer is `package main`, a blank line, then an empty `func main()`. New Scratch Buffer (`file.newScratchBuffer`), New Scratch File from Selection (`file.newScratchFromSelection`), Rename Scratch (`file.renameScratch`), and Move Scratch to Project (`file.moveScratch`) are unbound Find Action commands. There is no general New File command. Rename of a project file is still the phase 3 command `refactor.rename`. Running it logs `Rename is not available in v1`.

Closing the last view of a scratch whose buffer is empty or only whitespace deletes the file and closes the tab. If the file is already gone, the tab still closes. Any other delete error is the notice and the tab stays. A non-empty dirty scratch uses Save / Don't Save / Cancel. A non-empty clean scratch closes and stays on disk.

Save of a scratch does not run go fix or goimports. It runs gofumpt only when `editor.gofumpt_on_save` is on and the file is `.go`. Completion and diagnostics do not apply until the file is moved into the project.

**Agent.** Put nodes on `s.tree` when the test does not need disk I/O. For a real open, set `s.root` to a temp directory that contains the file, call `clickTree(rel)`, then `drain` so `DocumentOpened` attaches the tab. Pixel tests probe x=90 from y=30 downward until `openDirs` changes, as in `internal/app/tree_click_test.go`. The 300 ms gap is covered with `synctest` in `internal/app/project_test.go`.

## 4. Editing

**Do.** Click the buffer. Type. The keys below apply while the editor is focused, no popup is open, and the find bar is not focused.

**See.** Go Mono, 14 px, line height 1.3, tab width 4. The face is the built-in Go collection. `editor.font_family` is stored and unread. Each visible line is one row. A collapsed range hides the lines after its header through the end line, so those lines are not rows. `editor.soft_wrap` is stored and unread. The caret is a 2 px bar, drawn when the view is focused. The caret line keeps its highlight when focus is elsewhere. Only the primary caret is painted. Extra carets stay invisible until their selection is non-empty. Every non-empty selection is at least 2 px wide.

**Pass.** Editor keys that are not commands:

| Action | Keys | Result |
|---|---|---|
| Insert | a printable key | Every selection is replaced. The caret sits after the insert. Shift selects the uppercase letter. |
| Newline | Enter | Inserts a line feed. |
| Indent | Tab | Inserts a tab. With Insert spaces on, inserts spaces to the next `editor.tab_width` stop, counted in byte columns. |
| Delete backward or forward | Backspace, Delete | Deletes one rune, or the whole selection. At the buffer edge, the text stays. |
| Move by rune | Left, Right, Shift extends | One UTF-8 rune. |
| Move by line | Up, Down, Shift extends | One visible row. Folded lines are skipped. The preferred byte column is kept and clamped on a shorter line. Vertical moves do not change that preferred column. |
| Line edges | Home, End, Shift extends | The line start, or the line end with the newline excluded. |
| Document edges | Ctrl+Home, Ctrl+End | Offset 0, or the end. |
| Page | Page Up, Page Down | Repeats up or down for one viewport of visible rows. Folded lines are skipped. Before the first layout that count is 40. |
| Wheel | vertical | Moves 3 visible rows per event and stops when the last line is still in the viewport. Folded lines are skipped. A horizontal wheel changes `scrollX`, clamped at 0. |

Alt or Super on an editor key is ignored. Ctrl, Alt, or Super plus a character is not inserted. Ctrl+Left and Ctrl+Right move one rune. A second primary press in the text selects a word of letters, digits, and `_` when the gap is greater than zero and at most 200 ms. There is no Ctrl+click caret.

Mouse: a primary press places the caret and focuses the editor. Shift extends the primary selection and drops extra carets. A drag with the button held extends that selection. Release ends the drag. Click the gutter triangle to toggle that fold. Click the ellipsis to expand. A click in the text on a hidden line opens the folds that hide it. A gutter toggle does not expand other folds.

When the file is taller than the viewport, a scrollbar is drawn on the right, over the text. Clicking the track or dragging the thumb scrolls by visible rows, so folded lines are skipped. The last line stays in the viewport, so the view does not scroll into empty space below the file. A file that fits has no scrollbar and ignores a scroll that would move it.

Undo grouping: typing merges into the previous undo step when the previous edit was typing, it was within 1 second, the caret is adjacent, and the batch is a single edit. A caret move starts a new step. Multi-cursor inserts are several edits, so they do not merge. The buffer keeps 500 undo records. Paste, cut, duplicate, delete-line, comment, and replace are each their own step.

Ctrl+Z (`edit.undo`) and Ctrl+Shift+Z (`edit.redo`) put one caret at the end of the byte range that differs, then reveal that line. Reveal opens any fold that hides the line. Undo does not restore which ranges are collapsed. Undo on a clean buffer returns false and leaves the caret and the scroll line where they are.

CRLF files load as LF and save as CRLF.

Show whitespace (Settings, or `editor.show_whitespace`) draws `→` for a tab and `·` for a space in `editor.whitespace`. A raw tab is not shaped, because Go Mono has no glyph for U+0009. Other C0 controls and DEL take no cell.

Command edits:

| Command | Default | Result |
|---|---|---|
| `edit.selectAll` | `ctrl+a` | One selection covering the buffer. |
| `edit.copy` | `ctrl+c` | The primary selection is written to the clipboard. An empty selection does nothing. |
| `edit.cut` | `ctrl+x` | Cuts every non-empty selection. A read-only buffer leaves the text in place. |
| `edit.paste` | `ctrl+v` | Inserts the clipboard text at every selection. |
| `edit.duplicateLine` | `ctrl+d` | Inserts a copy of each selected line, one undo step. A collapsed header in the selection is expanded first, so the hidden lines are included. The caret then moves to the end of the buffer. |
| `edit.deleteLine` | `ctrl+y` | Removes each selected line, including its newline, one undo step. A collapsed header in the selection is expanded first, so the hidden lines are included. |
| `edit.toggleLineComment` | `ctrl+/` | Adds `// ` at the first non-space, or removes a `//` and one following space when every selected line already has one. Hidden lines are not commented. The caret stays. This matches Go comments in every file type. |
| `edit.fold` | `ctrl+-` | Collapses the range that starts on the caret line. If that range is already collapsed, collapses the next outer range that contains the line. |
| `edit.unfold` | `ctrl+=` | Expands the innermost collapsed range that starts on the caret line or hides it. On a US keyboard `ctrl+=` is the equal key. Fold All is `ctrl+shift+=`, the key a US keyboard labels as Plus, not Unfold. |
| `edit.foldAll` | `ctrl+shift+-` | Collapses every range. |
| `edit.unfoldAll` | `ctrl+shift+=` | Expands every range. On a US keyboard this chord is the key labeled Plus. |
| `edit.addNextOccurrence` | `alt+j` | An empty caret first selects `[A-Za-z0-9_]+`. The next exact copy is then selected and becomes primary. |
| `edit.selectAllOccurrences` | `ctrl+alt+shift+j` | Every non-overlapping copy of the primary selection. |
| `edit.unselectOccurrence` | `alt+shift+j` | Drops the primary selection. The last caret collapses to its head. |
| `edit.addCaret` | unbound | An empty caret on the next visible row at the preferred byte column. Folded lines are skipped. Run it from Find Action. It becomes the only painted caret. |
| `edit.highlight` | `ctrl+alt+h` | Marks the primary selection. An empty selection marks the caret line, from that line start through the next line start, or the buffer end on the last line. The same start, end, and kind is removed when the command runs again. |
| `edit.highlightLines` | `ctrl+alt+shift+h` | Marks every full line the primary selection touches. A selection that ends exactly at the start of a later line does not include that line. An empty selection marks the caret line. |

A mark is a persistent byte range on the open document. It is not the selection and it is not syntax color. It stays when the caret moves. Both splits of one file show the same marks. Marks are not saved. The last close of the document drops them.

Highlight Error (`edit.highlightError`) is unbound. It uses the same range rule as Highlight. Highlight Smell (`edit.highlightSmell`) is unbound. It uses the same range rule as Highlight. The same start, end, and kind is removed when the command runs again. Clear Highlights (`edit.clearHighlights`) is unbound. It removes every mark on the document.

A mark moves with the bytes. An insert inside the mark makes it longer. An edit that covers the mark removes it. An edit before the mark shifts it. Undo and redo follow the same rules.

The command does not change the text or the selection. A read-only buffer can take a mark. No open file does nothing.

The mark is a fill behind the glyphs and behind the selection. A line fully inside the mark fills across the text area. A partial range fills only its cells.

Highlight and Highlight Lines are note marks. Note uses `editor.findMatch`. Error uses `diag.error`. Smell uses `diag.warning`. The fill alpha is 80. Syntax color does not change.

A collapsed fold tints its ellipsis when a mark overlaps the hidden lines. The selection still paints over the mark.

Git gutter, when the folder is a repo and `git.gutter` is true: 150 ms after an edit, a 4 dp bar on the left of the line. Added is RGB (98, 150, 85), modified is (64, 120, 200), deleted is (200, 80, 80). Those literals are in `editor.View.Layout`. The theme keys `gutter.added`, `gutter.modified`, and `gutter.deleted` color nothing on the bar. Line numbers use `gutter.foreground` on `gutter.background`. A pure deletion is drawn on the previous line. The diff itself stops at 200 ms. The fold column is always reserved after that stripe and before the line numbers, so the numbers do not jump when a file has no folds. A disclosure triangle in that column points down when the range is open and right when it is collapsed. A collapsed header keeps its buffer line number, so the numbers have gaps. After the header text, `gutter.foreground` paints an ASCII ellipsis ` ... ` and a suffix. The suffix is the closing bracket or end tag (`}`, `)`, `]`, `</div>`). Indent folds and line-comment groups have an empty suffix. A block comment uses `*/` when that is the node's last token.

Code folding collapses a structural range. The header line stays visible. Hidden lines are the ones after the header through the end line, inclusive.

Go folds bracket pairs `()` `{}` `[]` that span two or more lines, multi-line block comments, and adjacent `//` comments with no blank line between them. `if` and `else` fold separately when gofmt puts `} else {` on one line.

HTML folds matching start and end tags. The suffix is the end tag as written. Void tags (`area`, `base`, `br`, `col`, `embed`, `hr`, `img`, `input`, `link`, `meta`, `source`, `track`, `wbr`) and self-closing tags do not fold. Inside `script` and `style`, braces fold too.

JavaScript and JSON fold brace and bracket ranges. Brackets inside strings, comments, and template literals are ignored. JSON has no comments.

Other opened files use indentation folds: YAML, SQL, Dockerfile, env, go.mod, go.sum, plain text, and Markdown source. A header is a non-empty line whose next non-empty line is indented further. The range runs through the last following line that is indented more than the header. One space or one tab is enough to nest. Markdown preview does not fold.

Fold state is per view. Splitting a view copies which ranges are collapsed. It is not saved, and undo does not restore it. A new view starts expanded. Go to a hidden line, by a click in the text, Reveal, or a jump, opens the folds that hide it. A gutter toggle does not expand other folds.

The caret blink period is `editor.cursor_blink_ms` (default 500). Half the period is on and half is off. Zero stays lit. Moving the caret restarts the phase.

**Agent.** Text and caret checks call the view directly:

```go
v := editor.NewView(doc)
v.Click(0, 2, false) // line, byte column, extend
v.Type("XYZ")
```

Assertions in `internal/editor/edit_test.go` and `extra_test.go` use `CaretPos`, `ScrollLine`, `SelectedText`, `SelectionBounds`, `Selections`, and `doc.Snapshot().Bytes()`. Pixel hits use `hitCell` in `internal/editor/layout_test.go`: gutter width + 8 dp pad + the center of the cell, after one `Step` so the cell size is known. A drag is `Desktop.Drag`, which queues `pointer.Move` with the button held. `TestDragSelectsText` selects `hello world`. `TestUndoStaysOnTheEdit` is the check that undo stays on the edit.

Through the shell, focus the editor tag, leave `popup` empty, and call `handleKeys` in the frame:

```go
d.Focus(s.inputTag())
d.Press(key.Name("a"), 0)     // v.Type("a")
d.Press(key.NameLeftArrow, 0) // MoveLeft
d.Edit(0, 0, "hi")            // ReplaceRunes when focus is the editor
d.Queue(key.Event{Name: "A", State: key.Press}, key.EditEvent{Text: "a"}) // one "a"
```

One printable key on a Linux window is a key event and an edit event in the same frame. The editor inserts the edit text once. `Press` inserts the character at the caret. `Press` without `Focus` is dropped. The shell also needs `s.keys = keymap.Default()`, a `keymap.Resolver`, and `s.bindCommands()` before a chord does anything.

## 5. Find and replace

### In the file

**Do.** Ctrl+F (`edit.find`) or Ctrl+R (`edit.replace`). Type the query. Enter, F3 (`edit.findNext`), or Shift+F3 (`edit.findPrev`) searches. Escape closes the bar. With Replace open, Tab switches between the two fields. Click `.*` for regex. Enter on the replace field replaces one match. Click `All` to replace every match.

**See.** A bar above the editor. The query line is `Find  ` plus a caret. The regex button is labeled `.*` and uses `ui.selection` while regex is on. Replace adds a `Replace  ` line and an `All` button. A single-line selection is copied into the query when the bar opens. The popup kinds `find` and `replace` are not opened by these commands. Their body, if a test sets `popup` to one of them, is `This action is not in this version.`

**Pass.** Each search puts the caret back at the position recorded when the bar opened, then selects the next match after that position. A second Enter or F3 selects that same match again. `editor.View.FindNext` on its own walks forward. The bar reset is what the window does.

Literal search is case-sensitive. An empty query does nothing. A zero-length regex match is skipped. A bad pattern stores `err.Error()` in `s.notice`. There is no whole-word switch and no case switch. The note mark uses `editor.findMatch`. The hit is the normal selection.

Replace one replaces the primary selection when it matches the query, as one undo step, then selects the next match from the new caret. Replace all reports `<n> replaced` and moves the caret to the end of the buffer. In regex mode, `$1` and the other `Regexp.Expand` tokens expand. `TestFindReplace` calls `View.FindNext` directly, so it walks. A window test that uses the bar expects the same match twice.

**Agent.** Call `openFind(false)` or `openFind(true)`, set `findQuery`, and call `applyFind(1)` twice. Compare `SelectionBounds` after each call. They match.

### In the project

**Do.** Ctrl+Shift+F (`search.inFiles`). Type a pattern. Enter.

**See.** Popup title `Search in Files`. The row reads `Enter runs the search. Results open in the bottom panel.` The bottom title is `Search`. Before any run the body is `No matches yet. Ctrl+Shift+F searches the project.`

**Pass.** An empty pattern does nothing. With no folder, `s.notice` is `Open a folder before searching`. A run is a case-insensitive literal search. `Regex`, whole word, and case-sensitive stay false on that query. Exclude comes from `search.exclude` (default `vendor/`). The engine is `builtin` unless `search.engine` is `rg`.

The bottom body becomes `<n> matches  <path>:<line>  <text>` for the first hit only (`formatSearchHits`). Hits are not clickable. Builtin search skips a file larger than 8 MiB, a file with a NUL in the first 8 KiB, and a line that is not valid UTF-8. Results arrive in batches of 200. Line and column are 1-based byte columns. `search.max_results` (default 20000) is stored and unread. A missing `rg`, or a bad pattern for that engine, replaces the body with the error and sets `s.notice`.

**Do.** Ctrl+Shift+R (`search.replaceInFiles`).

**See.** The same popup title. The row reads `Pattern, then Tab is not required: type pattern<tab>replacement, then Enter.`

**Pass.** The replacement split is a raw tab byte in the field (`strings.IndexByte(pattern, '\t')`). The Tab key is handled as a motion key and does not insert that byte, so a person typing in the popup searches the pattern and writes an empty replacement. A test sets `s.field` to `"old\tnew"` and calls `runProjectSearch(true)`. The write is `strings.ReplaceAll` on each regular file inside the workspace, skipping a `vendor/` prefix, a symlink, a path that resolves outside the workspace, files over 8 MiB, and a NUL in the first 8 KiB. Open buffers are left as they were, including dirty ones. The notice is `<n> replacements written`.

**Agent.** `TestBuiltinSearch` and `TestSearchMatchesRg` in `internal/search/search_test.go` cover the engines. The panel assertion reads `s.searchHits` and the formatted first line.

## 6. Save, close, and splits

**Do.** Ctrl+S (`file.save`). Ctrl+F4 (`file.close`). Save All, Split Right, and Split Down have no default key. Run them from Find Action (`file.saveAll`, `view.splitRight`, `view.splitDown`).

**See.** Save shows nothing on the chrome when a file is open. The log line is the notice when there is one. Close of the last view of a changed buffer opens a card titled `Save changes?` with `<path> has unsaved changes.` and the rows Save, Don't Save, and Cancel.

**Pass.**

| Action | Result |
|---|---|
| Save, dirty and writable | Writes a new file in the same directory and renames it over the path. An existing regular file keeps its mode. A missing path is created as mode 0644. A symlink is replaced and is not opened. A second save of a clean buffer does not write. |
| Save, no buffer | Notice `Nothing to save`. |
| Save, read-only | The read-only notice. |
| Save All | Each dirty writable buffer once. None dirty: `Nothing to save`. |
| Close, no buffer | Notice `No editor tab`. |
| Close, last view of a dirty buffer | Popup `unsaved`. Save writes, then closes when the buffer is clean. Don't Save drops it. Cancel leaves it. Escape cancels. An empty or whitespace-only scratch is deleted instead. |
| Close, last view of an empty scratch | Deletes the file and closes the tab. No popup. Whitespace-only counts as empty. A missing file still closes. Any other delete error leaves the tab and notifies. |
| Close, another view of that buffer remains | The view closes with no popup. |
| Split with a buffer open | A fixed 50/50 of the same document. Carets are separate. Text is shared. |
| Split with nothing open | Notice `Open a file before splitting`. |

A `.go` save with `editor.gofix_on_save`, `editor.goimports_on_save`, or `editor.gofumpt_on_save` on runs those tools off the UI goroutine before the temp-file write. All three on runs go fix, then goimports, then gofumpt. A failure still writes the original bytes and notifies. If the buffer changes while the tools run, the save is dropped and the tab is not closed. gofumpt does not run the `go` command. go fix and goimports do. go fix rewrites only the saved file. The three settings default off. A scratch does not run go fix or goimports. It runs gofumpt only when `editor.gofumpt_on_save` is on and the path is `.go`.

There is no splitter drag handle. An empty side collapses.

**Agent.** `TestSplitCloseAndUnsaved` in `internal/app/docs_test.go` opens the unsaved popup and chooses Don't Save. Call `cmdSave` and compare the file bytes. CRLF round-trip is `TestCRLFAndBinary` in `internal/buffer/buffer_test.go`: `a\r\nb\r\n` plus an insert of `c` at the start saves `ac\r\nb\r\n`.

## 7. Navigation and code intelligence

**Do.** With a folder open and `intel.enabled` true (the default), wait until the status cell reads `intel Ready`. Then use the keys in the table.

**See.** The cell starts as `intel stopped` on a hand-built shell. `startIntel` sets `intel off` when there is no folder or intel is disabled, and `intel Starting` when it starts. Later text comes from the host, including `Ready` and `Restarting`.

**Pass.**

| Command | Default | Result |
|---|---|---|
| `nav.goToFile` | `ctrl+shift+n` | Up to 50 matching paths. Enter opens `<root>/<rel>`. Empty list text: `No matching files`. |
| `nav.recentFiles` | `ctrl+e` | Paths from the recent file. Enter opens one. Empty: `No recent files`. |
| `nav.goToSymbol` | `ctrl+alt+shift+n` | Workspace symbols. The query updates as you type. Enter jumps. Empty: `No symbols`. |
| `nav.fileStructure` | `ctrl+f12` | Symbols of the active Go buffer, queried once. Typing does not filter the list. |
| `nav.goToDeclaration` | `ctrl+b` | Jumps to the first definition and pushes Back. No list. |
| `nav.findUsages` | `alt+f7` | Rows `path:line:col`. Enter jumps. Empty: `No usages`. |
| `nav.back` | `ctrl+alt+left` | Returns to the previous jump. An empty stack does nothing. |
| `nav.forward` | `ctrl+alt+right` | The opposite direction. |
| `edit.completion` | `ctrl+space` | Rows are the label plus detail. Enter inserts the item as one undo step. Snippet placeholders are flattened. Empty: `No completions`. |
| `edit.quickDoc` | `ctrl+q` | The first line of the hover markdown is stored in `s.notice`. No hover: `No documentation`. |
| `nav.goToLine` | `ctrl+g` | Title `Go to Line`. Body `This action is not in this version.` Enter closes the card. `line:col` stays. |
| `nav.nextError` | `f2` | Notice `nav.nextError is ready once a file is open`. The handler stays after a file is open. |
| `nav.prevError` | `shift+f2` | Notice `nav.prevError is ready once a file is open`. |
| `intel.restart` | unbound | Before start: `Code intelligence is not running yet`. After start: the cell reads `intel Restarting`, then `intel Ready`. |

Before `startIntel`, the intel-backed commands keep the notice `<id> is ready once a file is open`.

Syntax color runs for Go, go.mod, go.sum, YAML, JSON, Dockerfiles, env files, JavaScript, HTML, and SQL. Other files stay `editor.foreground`. Go uses tree-sitter. The other languages are scanned. A Dockerfile is `Dockerfile`, `Dockerfile.*`, or `*.dockerfile`. An env file is `.env`, `.env.*`, or `*.env`. JavaScript is `.js`, `.mjs`, or `.cjs`. HTML is `.html` or `.htm`. YAML is `.yaml` or `.yml`, including Compose files. JSON is `.json`. SQL is `.sql`, `.pgsql`, or `.psql`, highlighted as PostgreSQL. Block comments nest. Dollar-quoted text is scanned again, so keywords inside it stay colored. Bracket matching stays on Go. Go captures map to theme keys:

| Capture | Theme key | Dark |
|---|---|---|
| `function`, `function.method`, `function.builtin` | `syntax.function` | `#56a8f5` |
| `keyword` | `syntax.keyword` | `#cf8e6d` |
| `comment` | `syntax.comment` | `#7a7e85` |
| `string`, `escape` | `syntax.string` | `#6aab73` |
| `number` | `syntax.number` | `#2aacb8` |
| `constant`, `constant.builtin` | `syntax.constant` | `#c77dbb` |
| `type` | `syntax.type` | `#bcbec4` |
| `property` | `syntax.property` | `#c77dbb` |
| `operator` | `syntax.operator` | `#bcbec4` |

`@variable` and unknown captures become foreground and are dropped. Higher priority wins on a byte. The visible window requested each frame runs from 20 lines above the scroll line through 80 lines below it. A caret on `(){}[]` paints both ends of the matching tree-sitter node with `editor.bracketMatch` (dark `#3b514d`).

Problems (Alt+6) lists up to 80 `basename: message` rows on the left. They are not clickable. The bottom body is `No problems.` or the joined lines. Status counts use `%d errors  %d warnings`. Severity at or below 1 counts as an error. Settings warnings count too, and each one is also a Problems row `key: message`.

`intel.semantic_tokens` is stored. The app does not request semantic tokens. The five timeouts below are passed into the host when they are greater than 0. `[intel.gopls]` accepts `staticcheck` (bool, default false) and `directoryFilters` (default `["-node_modules"]`) and sends them on start and on settings reload.

`runtime.memory_limit` is applied once at startup. `auto` and `off` parse as no extra limit from this setting. `auto` uses `max(physical RAM/2, 2 GiB)`. `off` leaves the limit unchanged. A size such as `6GiB` is accepted. `GOMEMLIMIT` wins, and Apply then does not call `SetMemoryLimit`. A reload does not apply the limit again. At or above 90% the notice is `Memory is above 90% of the limit. Run Restart Code Intelligence.`

**Agent.** Declaration, usages, completion, hover, and symbols need a real module and `go` on `PATH`. The fixture module is `testdata/fixture`. Wait until `s.intel` is `Ready` before those three checks in the smoke list. Go to Line is `openPopup` kind `line`, then Enter, then the same `CaretPos`. Syntax goldens are `internal/syntax/testdata/fixture.go` against `fixture.golden`. `GIO_IDE_UPDATE_GOLDEN=1` rewrites that file. That golden is spans (`line:start:end:style`), not a screenshot.

## 8. Git

**Do.** Alt+9 (`view.gitTool`). Ctrl+K (`git.commit`) types a message and Enter commits. Ctrl+Shift and the backtick key (`git.branches`) switches or creates a branch. `git.refreshStatus` has no default key.

**See.** The left panel title is `Git`.

**Pass.**

| State | On screen |
|---|---|
| Not a repo, or no folder | `This folder is not a Git repository.` The same sentence is the notice if you commit or switch anyway. |
| Clean repo | `<branch>  clean` |
| Dirty repo | Header `Git  <branch>`, then one row per path: two status bytes and the path (` M`, `??`, `A `). The list scrolls with the wheel. A click highlights the row and shows that file's diff. |
| Status bar | The branch name, or `—` when no repo is open. |

Status refreshes after a commit, on file-system events, and on `git.refreshStatus`. The wait is `git.status_debounce_ms` (default 300). A value of 0 or less is treated as 300.

Commit popup title `Commit`. The prompt is `Commit message, then Enter. Changed files are staged.` When a hook is executable or `commit.gpgsign` is set, the prompt is replaced by `gio-ide does not run Git hooks or sign commits. Use the terminal for git commands that need them.` Enter stages every status path, then commits. An empty message surfaces `commit message is empty`. Success logs `Committed`. The commit is unsigned, and hooks are not run. Escape closes the card.

Branches popup title `Branches`. The current name ends with `  *`. An empty list reads `No branches yet. Type a name and press Enter.` Enter with an empty field checks out the selected row. A typed name checks out that branch, or creates it at HEAD when the name is new. Force is never set. Dirty editor buffers log `Save files before switching branches` and the branch stays. A dirty worktree returns `worktree has local changes`.

A click replaces the focused editor with a side-by-side diff of HEAD against the current file. The left column is titled `HEAD` and the right column is titled `Working tree`. The two columns scroll together. Other splits keep their buffers. With no file open, the diff replaces the welcome page. The current file is the open buffer when that path is open, and the worktree file otherwise. An untracked file is all additions on the right. A deleted file is all removals on the left. A changed line is shown on both sides, and the words that differ are marked darker than the rest of the line. A file that is both staged and unstaged shows the net result against HEAD. The pinned title is the path. **Close** at the top right, and Escape, clear the diff and return to the buffer. Opening a file from the project tree clears it too. Arrow keys, page, home, and end scroll the diff. Typing does not change the buffer while the diff is showing. A binary file reads `Binary files differ`. A file over 1 MiB reads `File is too large to diff`. An empty diff reads `No changes`. Added lines are green and removed lines are red, the same colors as the gutter bars.

`internal/git` can unstage. No command calls it. There is no push, pull, or discard in the window.

**Agent.** Use a temp repo. After an edit, drain until `gitFiles` contains the path. `doCommit` with a non-empty `field` clears the status. `Switch` with `Create: true` changes `s.branch` after the next status refresh. Assert the hook prompt by setting a hook executable and reading `s.gitNotice`, and assert that the hook script did not run. A click on a row sets `s.gitDiffPath`. Drain until `s.gitDiffRows` is non-empty for a text change, or `s.gitDiffNote` is set (`Binary files differ`, `File is too large to diff`, `No changes`, or an error). Close and Escape clear `s.gitDiffPath`.

## 9. Terminal

**Do.** Alt+F12 (`view.terminal`). Type a command. Alt+F12 again while the terminal is focused.

**See.** The terminal starts at 240 dp. The bar above it is a resize handle. Drag up to make the panel taller, or down to make it shorter. The panel stays at least 80 dp, and the editor keeps at least 120 dp. The handle draws in the caret color while the pointer is on it. The tab strip is 30 dp on `ui.tabInactive`. A tab title is the OSC title, or `terminal`. Inactive tabs use dim text. The current tab uses `ui.tabActive`, full foreground, and `ui.accent` along its top and bottom. An `x` on the right of each tab closes that session. A `+` after the tabs starts another session. A shell that exits with a failure paints through the notice as `terminal: …`. Closing a tab does not. Text is a pixel grid: each glyph is rasterized once and blitted on whole pixels. The cursor is a block the size of the cell.

**Pass.** The first show starts a PTY when none exists. The shell is `terminal.shell` when that string is non-empty, otherwise `$SHELL`, otherwise `/bin/sh`. The directory is the workspace, or the process cwd. A second Alt+F12 while focus is the terminal hides the panel and returns focus to the editor.

While the terminal is focused, keys go to the PTY except this set: Alt+F12, Alt+1, Alt+6, Alt+9, Ctrl+Shift+A, Ctrl+Shift+N, Ctrl+Shift+C, Ctrl+Shift+V, Ctrl+T. Ctrl+C is the interrupt. Ctrl+D goes to the shell. On an empty line the shell exits, that tab closes without a notice, and the last tab hides the panel and returns focus to the editor. The same close happens when the session ends for any other reason. Shift+Page Up and Shift+Page Down scroll one page of the grid. Copy is Ctrl+Shift+C (`terminal.copy`). With no selection the notice is `Nothing selected in the terminal`. Paste is Ctrl+Shift+V (`terminal.paste`). A drag that reaches another cell selects those cells. A click clears the selection. The wheel scrolls 3 lines. Bracketed paste is used when the PTY has turned it on.

Ctrl+T (`terminal.new`) starts another session from the editor or from a focused terminal. The `+` runs the same command. `terminal.close` has no default key. The `x` on a tab closes that session. Close of the last session hides the panel. Scrollback defaults to 10000 lines. A value of 0 or less is treated as 10000. ANSI-16 colors are the fixed table in `internal/terminal/widget.go`. Default foreground, background, cursor, and selection come from the `terminal.*` theme keys.

**Agent.** A PTY test waits on the session output. It does not sleep. Do not call `terminal.Start` or `newTerminal` from a test. Focus checks read `s.focus` and `s.bottomOpen` around `view.terminal`. Copy and paste go through `terminal.copy` and `terminal.paste` with the clipboard event the harness already feeds. `termH` is the terminal panel height in pixels. Zero means the 240 dp default. `termSashY` is the handle's window Y from the previous frame. Press on that bar, then move with the primary button held. Moving up increases `termH`. A click in the grid leaves `Selection` false. Press, then move onto another cell, and `Selection` is set. `onTerminalExit` drops the tab whose `Term.ID` matches `Session`. `Close` and a PTY read of EIO post that event with an empty `Err`, so the notice stays clear. A non-zero wait status still fills `Err`. Ctrl+T is `terminal.new` and is in `TerminalPassthrough`. The `+` runs `terminal.new`. `termTabLeft` and `termPlusLeft` are strip-local x positions from the latest frame. In the full window the strip starts after the 40 dp tool strip when the left panel is closed. The resize bar and the `+` use different pointer tags; a zero-size tag would alias them. The face is DejaVu Sans Mono when those four files are installed, otherwise Liberation Mono, otherwise Go Mono.

## 10. Find Action, Settings, keymap, and theme

### Find Action

**Do.** Ctrl+Shift+A (`app.findAction`). Type a title or an id. Up and Down move. Enter runs the row. Ctrl+Shift+A or Escape closes.

**See.** Title `Find Action`. Each row is the title, then spaces, then the id. The list keeps the selection on the page with a first-visible-row index. Rows are not clickable. The wheel scrolls the list.

**Pass.** An empty query hides every command whose phase is non-empty. A query that matches one of those shows the title plus `  (later)`. Enter on it logs `<Title> is not available in v1`. A v1 command runs.

**Agent.** `TestActionPaletteRevealFollowsSelection` drives `popupKey` with Up and Down and reads `listScroll`. Filtering reads `filteredCommands()`.

### Settings

**Do.** Ctrl+Alt+S (`app.settings`). Click − or + , or the toggle on the right of a row. Escape closes.

**See.** Title `Settings`. Plain keys, including Enter, do nothing. Ctrl, Alt, and Super chords still run. The card does not toggle closed when the command runs again.

**Pass.** The click updates the value, repaints in the same frame, and writes the changed key to the user file. The project file is reloaded and still wins.

| Row | Control | Range | Default | Key |
|---|---|---|---|---|
| Font size | − / + by 1 | 8–72 | 14 | `editor.font_size` |
| Cursor blink | − / + by 50 | 0–2000, and 0 is labeled `steady` | 500 | `editor.cursor_blink_ms` |
| Line height | − / + by 0.1 | 1–2.5 | 1.3 | `editor.line_height` |
| Tab width | − / + by 1 | 1–16 | 4 | `editor.tab_width` |
| Terminal font | − / + by 1 | 8–32 | 13 | `terminal.font_size` |
| Indent | toggle | `Tabs` or `Spaces` | Tabs (`insert_spaces` false) | `editor.insert_spaces` |
| Whitespace | toggle | `Hidden` or `Shown` | Hidden | `editor.show_whitespace` |
| gofumpt | toggle | `Off` or `On` | Off | `editor.gofumpt_on_save` |
| goimports | toggle | `Off` or `On` | Off | `editor.goimports_on_save` |
| go fix | toggle | `Off` or `On` | Off | `editor.gofix_on_save` |
| Theme | toggle | `Dark` or `Light` | `dark` | `ui.theme` |

The font-size plus control is the rightmost 36 px of the first row. `TestSettingsFontSizePersists` clicks around (888, 196) and expects font size 15 in memory and `font_size = 15` in the user file.

Hand-edited TOML reloads 100 ms after a change in the user or project settings directory. Integers must be ≥ 0. Floats must be > 0. A bad value keeps the default and adds a Problems row. Unknown keys warn and are ignored. `search.engine` must be `builtin` or `rg`. `ui.theme` may be any non-empty string. `dark` and `light` are built in. Any other name loads `themes/<name>.toml`, which inherits `base` `dark` or `light`. A failed load keeps the theme already on screen and sets `s.notice`.

The keymap block under the rows lists every command whose phase is empty. The shortcut shows the stroke, or `Unbound`. Clicking it shows `Press a key...`. The next stroke becomes the only binding for that command, is removed from any other command, and is written to `keymap.toml` immediately. A displaced command logs `<stroke> was <title>`. Escape cancels the capture and writes nothing. A row Reset appears when that command differs from the default, and it restores that command. The header Reset restores `keymap.Default()` and saves. Both write immediately.

`app.keymap` does not open this card. It ensures the config directory exists, logs `Opening <path>`, and starts `xdg-open` on `keymap.toml`. If `xdg-open` fails to start, the notice is `Open <path> in an editor`. A directory error is that error's text.

`keymap.toml` shape:

```toml
[[unbind]]
keys = "ctrl+s"

[[bind]]
keys = "ctrl+alt+s"
command = "file.save"
when = ""
```

Sequences are space-separated strokes, for example `alt+g b`. A stroke is `ctrl`, `alt`, `shift`, or `super`, plus a key. A trailing `+` is the plus key (`ctrl++`). `when` allows identifiers, `!`, and `&&`. The identifiers are `editorFocus`, `terminalFocus`, `treeFocus`, `popupOpen`, and `hasSelection`. Every default binding has an empty `when`, which evaluates as true. An empty diff writes an empty file, and the next load is the defaults. The chord timeout is 1.5 s. No default binding is a chord. Editing `keymap.toml` applies on the same directory watch as `settings.toml`, with no restart.

`view.toggleTheme` flips the painted theme between `light` and `dark` and does not write the file. A stored name other than those two becomes `light` on screen, and the user file keeps the custom name (`TestToggleThemeLeavesCustomName`).

Built-in dark, from `internal/theme/theme.go`: editor background `#1e1f22`, foreground `#bcbec4`, caret `#ced0d6`, accent `#3574f0`, selection `#214283`. Built-in light: editor background `#ffffff`, foreground `#080808`, gutter background `#f7f8fa`. Chrome font sizes are fixed `unit.Sp` values (13, 12, and 14). `ui.ui_font_size` is stored and unread. The UI face is Go.

**Agent.** `s.onCommand["app.settings"]()` or `s.popup = "settings"`. Without pixels, `s.adjustSetting("set:font+")`. Key capture is `TestSettingsKeymapCaptureRebinds` in `internal/app/keymap_ui_test.go`: it uses a raw router, presses one stroke, and reads `keymap.toml`. `TestSettingsPopupLetsChordsThrough` checks that a chord still runs while the card is open.

### About

**Do.** Run `app.about` from Find Action.

**See.** Four lines: `gio-ide <version>`, `Go code editor for Linux.`, `In-process gopls, tree-sitter, go-git.`, and `No plugins.`

**Pass.** Enter closes the card.

## 11. Command catalog

One row per id in `commands.Catalog`. Stroke text is the default binding. `unbound` means the id is absent from `defaultRows`.

### File

| ID | Title | Stroke | Result |
|---|---|---|---|
| `file.save` | Save | `ctrl+s` | Writes the active dirty writable buffer. Clean: no write. None: `Nothing to save`. |
| `file.saveAll` | Save All | unbound | Writes every dirty writable buffer. None: `Nothing to save`. |
| `file.close` | Close | `ctrl+f4` | Closes the active buffer, or opens `Save changes?` for the last dirty view. The last view of an empty or whitespace scratch deletes the file instead. None: `No editor tab`. |
| `file.newScratch` | New Scratch File | `ctrl+alt+n` | Creates a Go scratch (`package main`, a blank line, empty `func main()`) and opens it. |
| `file.newScratchBuffer` | New Scratch Buffer | unbound | Creates an empty text scratch and opens it. |
| `file.newScratchFromSelection` | New Scratch File from Selection | unbound | Writes the primary selection into a new scratch, with no wrapper. Extension `.go` when the active path ends in `.go`, otherwise `.txt`. No view or an empty selection notifies `Select text to make a scratch file`. |
| `file.renameScratch` | Rename Scratch | unbound | Asks for a new base name. Enter renames the file and keeps the buffer, including unsaved edits. The same name is a no-op. Escape cancels. A project file notifies `The current file is not a scratch`. |
| `file.moveScratch` | Move Scratch to Project | unbound | Asks for a relative path in the open folder. Enter moves the file there. A `.go` file then gets code intelligence. A path outside the folder is refused. No folder notifies `Open a folder before moving a scratch`. A project file notifies `The current file is not a scratch`. |

### Navigation

| ID | Title | Stroke | Result |
|---|---|---|---|
| `nav.goToFile` | Go to File | `ctrl+shift+n` | Popup of up to 50 paths. Enter opens one. |
| `nav.goToSymbol` | Go to Symbol | `ctrl+alt+shift+n` | Workspace symbols. Enter jumps. |
| `nav.fileStructure` | File Structure | `ctrl+f12` | Document symbols for the active Go buffer. Enter jumps. |
| `nav.goToDeclaration` | Go to Declaration | `ctrl+b` | Jumps to the first definition and pushes Back. |
| `nav.findUsages` | Find Usages | `alt+f7` | Popup of `path:line:col`. Enter jumps. |
| `nav.back` | Back | `ctrl+alt+left` | Previous jump, or no change on an empty stack. |
| `nav.forward` | Forward | `ctrl+alt+right` | Next jump, or no change on an empty stack. |
| `nav.goToLine` | Go to Line | `ctrl+g` | Milestone popup. Enter closes it. The caret stays. |
| `nav.recentFiles` | Recent Files | `ctrl+e` | Recent paths. Enter opens one. |
| `nav.nextError` | Next Error | `f2` | Notice `nav.nextError is ready once a file is open`. |
| `nav.prevError` | Previous Error | `shift+f2` | Notice `nav.prevError is ready once a file is open`. |
| `nav.nextTab` | Next Tab | `alt+right` | Shows the next open buffer in the focused leaf. |
| `nav.prevTab` | Previous Tab | `alt+left` | Shows the previous open buffer. |

### Edit

| ID | Title | Stroke | Result |
|---|---|---|---|
| `edit.find` | Find | `ctrl+f` | Opens the find bar and selects the first match after the open-time caret. |
| `edit.replace` | Replace | `ctrl+r` | Opens the find bar with a replace field. |
| `edit.findNext` | Find Next | `f3` | Searches again from the open-time caret. Opens the bar first when it is closed. |
| `edit.findPrev` | Find Previous | `shift+f3` | Same restart, in the previous direction. |
| `edit.completion` | Completion | `ctrl+space` | Completion popup. Enter inserts the item. |
| `edit.quickDoc` | Quick Documentation | `ctrl+q` | First hover line in `s.notice`. |
| `edit.addNextOccurrence` | Add Next Occurrence | `alt+j` | Selects the next copy of the word or selection. |
| `edit.unselectOccurrence` | Unselect Occurrence | `alt+shift+j` | Drops the primary caret. |
| `edit.selectAllOccurrences` | Select All Occurrences | `ctrl+alt+shift+j` | Selects every copy. |
| `edit.addCaret` | Add Caret | unbound | Adds an empty caret on the next visible row. Folded lines are skipped. |
| `edit.duplicateLine` | Duplicate Line | `ctrl+d` | Duplicates the selected lines. A collapsed header in the selection expands so the hidden lines are included. The caret moves to the end of the buffer. |
| `edit.deleteLine` | Delete Line | `ctrl+y` | Deletes the selected lines. A collapsed header in the selection expands so the hidden lines are included. |
| `edit.toggleLineComment` | Toggle Line Comment | `ctrl+/` | Toggles a `// ` line comment. Hidden lines are not commented. |
| `edit.fold` | Fold | `ctrl+-` | Collapses the range that starts on the caret line. If that range is already collapsed, collapses the next outer range that contains the line. |
| `edit.unfold` | Unfold | `ctrl+=` | Expands the innermost collapsed range that starts on the caret line or hides it. On a US keyboard `ctrl+=` is the equal key. Fold All is `ctrl+shift+=`, the key a US keyboard labels as Plus, not Unfold. |
| `edit.foldAll` | Fold All | `ctrl+shift+-` | Collapses every range. |
| `edit.unfoldAll` | Unfold All | `ctrl+shift+=` | Expands every range. On a US keyboard this chord is the key labeled Plus. |
| `edit.undo` | Undo | `ctrl+z` | Reverts one step. The caret sits at the end of the change. |
| `edit.redo` | Redo | `ctrl+shift+z` | Reapplies one step the same way. |
| `edit.copy` | Copy | `ctrl+c` | Copies the primary selection. |
| `edit.cut` | Cut | `ctrl+x` | Cuts every non-empty selection. |
| `edit.paste` | Paste | `ctrl+v` | Pastes at every selection. |
| `edit.selectAll` | Select All | `ctrl+a` | Selects the whole buffer. |
| `edit.highlight` | Highlight | `ctrl+alt+h` | Marks the primary selection. An empty selection marks the caret line. The same start, end, and kind is removed when the command runs again. |
| `edit.highlightLines` | Highlight Lines | `ctrl+alt+shift+h` | Marks every full line the primary selection touches. A selection that ends exactly at the start of a later line does not include that line. An empty selection marks the caret line. |
| `edit.highlightError` | Highlight Error | unbound | Same range rule as Highlight, drawn with `diag.error`. |
| `edit.highlightSmell` | Highlight Smell | unbound | Same range rule as Highlight, drawn with `diag.warning`. |
| `edit.clearHighlights` | Clear Highlights | unbound | Removes every mark on the document. |

### Search

| ID | Title | Stroke | Result |
|---|---|---|---|
| `search.inFiles` | Search in Files | `ctrl+shift+f` | Project search. The bottom panel shows the first hit's summary line. |
| `search.replaceInFiles` | Replace in Files | `ctrl+shift+r` | Literal replace on disk when the field contains a tab byte. |

### View

| ID | Title | Stroke | Result |
|---|---|---|---|
| `view.splitRight` | Split Right | unbound | Side-by-side views of the active document. |
| `view.splitDown` | Split Down | unbound | Stacked views of the active document. |
| `view.projectTool` | Project | `alt+1` | Toggles the left Project panel. |
| `view.problemsTool` | Problems | `alt+6` | Opens Problems on the left and the bottom. Stays open. |
| `view.gitTool` | Git | `alt+9` | Toggles the left Git panel. |
| `view.terminal` | Terminal | `alt+f12` | Shows and focuses the terminal, or hides it when it already has focus. |
| `view.toggleTheme` | Toggle Theme | unbound | Flips the painted theme between `light` and `dark`. The user file stays. |

### Git

| ID | Title | Stroke | Result |
|---|---|---|---|
| `git.commit` | Commit | `ctrl+k` | Stages every status path and commits the message. |
| `git.branches` | Branches | ctrl+shift+backtick | Checks out the selected or typed branch, or creates a missing name. |
| `git.refreshStatus` | Refresh Git Status | unbound | Schedules a status reload after the debounce. |

The Branches source stroke is ctrl, shift, and the backtick key. In `defaultRows` that string is:

```text
ctrl+shift+`
```

### Terminal

| ID | Title | Stroke | Result |
|---|---|---|---|
| `terminal.copy` | Terminal Copy | `ctrl+shift+c` | Copies the terminal selection. |
| `terminal.paste` | Terminal Paste | `ctrl+shift+v` | Writes the clipboard to the PTY. |
| `terminal.new` | New Terminal | `ctrl+t` | Starts another PTY tab. The `+` on the tab strip runs it too. |
| `terminal.close` | Close Terminal | unbound | Closes the active PTY. The `x` on a tab closes that session. The last one hides the panel. |

### Intel and app

| ID | Title | Stroke | Result |
|---|---|---|---|
| `intel.restart` | Restart Code Intelligence | unbound | Restarts the host. The status cell goes through `Restarting` to `Ready`. |
| `app.findAction` | Find Action | `ctrl+shift+a` | Toggles the command palette. |
| `app.settings` | Settings | `ctrl+alt+s` | Opens the Settings card. |
| `app.keymap` | Keymap | unbound | Opens `keymap.toml` in the external opener. |
| `app.about` | About | unbound | Shows `gio-ide <version>`. |

### Code

These commands are unbound. Find Action can run them. They do not read `editor.gofix_on_save`, `editor.gofumpt_on_save`, or `editor.goimports_on_save`.

| ID | Title | Stroke | Result |
|---|---|---|---|
| `code.reformat` | Reformat Code | unbound | Runs embedded gofumpt on the active `.go` buffer and does not save. A clean result is quiet. A change is one undo step and leaves the file dirty. A non-`.go` buffer notifies that format applies to Go files. Read-only keeps its notice. |
| `code.organizeImports` | Optimize Imports | unbound | Runs embedded goimports on the active `.go` buffer and does not save. A clean result is quiet. A change is one undo step and leaves the file dirty. A non-`.go` buffer notifies that format applies to Go files. Read-only keeps its notice. goimports runs the `go` command. |

### Later

These ids are unbound. Find Action shows them only while the query matches, with `  (later)` on the title. Running one logs `<Title> is not available in v1`. The breakpoint gutter column width is 0.

| ID | Title | Phase |
|---|---|---|
| `debug.launch` | Debug: Start | 2 |
| `debug.continue` | Debug: Continue | 2 |
| `debug.next` | Debug: Step Over | 2 |
| `debug.stepIn` | Debug: Step Into | 2 |
| `debug.stepOut` | Debug: Step Out | 2 |
| `debug.pause` | Debug: Pause | 2 |
| `debug.stop` | Debug: Stop | 2 |
| `refactor.rename` | Rename | 3 |
| `refactor.extract` | Extract | 3 |
| `refactor.codeAction` | Code Action | 3 |
| `run.run` | Run | 3 |
| `run.test` | Run Tests | 3 |
| `test.run` | Test | 3 |
| `test.debug` | Debug Test | 3 |

Editor keys that are not in this catalog (arrows, Home, End, Page Up, Page Down, Backspace, Delete, Enter, Tab, and printable characters) are in [Editing](#4-editing).

## 12. Settings keys

Types and defaults are `settings.Defaults`. The Settings card edits the eleven keys in [Settings](#settings). Everything else is TOML only.

Keys the program reads:

| Key | Type | Default | What reads it |
|---|---|---|---|
| `editor.font_size` | float | `14` | Editor style. Values ≤ 0 paint as 14. Card clamp 8–72. |
| `editor.line_height` | float | `1.3` | Editor style. Values ≤ 0 paint as 1.3. Card clamp 1–2.5. |
| `editor.tab_width` | int | `4` | Tab display and the Insert-spaces stop. Values ≤ 0 paint as 4. Card clamp 1–16. |
| `editor.insert_spaces` | bool | `false` | Tab inserts spaces when true. |
| `editor.show_whitespace` | bool | `false` | Draws `→` and `·`. |
| `editor.gofix_on_save` | bool | `false` | Save of a `.go` file. Runs `go fix` on that file only. |
| `editor.gofumpt_on_save` | bool | `false` | Save of a `.go` file. `code.reformat` uses the same embedded gofumpt and does not read this bool. |
| `editor.goimports_on_save` | bool | `false` | Save of a `.go` file. `code.organizeImports` uses the same embedded goimports and does not read this bool. |
| `editor.cursor_blink_ms` | int | `500` | Full blink period. 0 stays lit. |
| `ui.theme` | string | `dark` | `dark`, `light`, or `themes/<name>.toml`. |
| `intel.enabled` | bool | `true` | False or no folder: `intel off`, and the pre-start command notices stay. |
| `intel.completion_timeout_ms` | int | `1500` | Host timeout when > 0. |
| `intel.hover_timeout_ms` | int | `1500` | Host timeout when > 0. |
| `intel.definition_timeout_ms` | int | `3000` | Host timeout when > 0. |
| `intel.references_timeout_ms` | int | `10000` | Host timeout when > 0. |
| `intel.symbols_timeout_ms` | int | `5000` | Host timeout when > 0. |
| `intel.gopls.staticcheck` | bool | `false` | Passed to the host on start and on reload. |
| `intel.gopls.directoryFilters` | string array | `["-node_modules"]` | Passed to the host on start and on reload. |
| `runtime.memory_limit` | string | `auto` | Once at startup. `auto`, `off`, or a size. |
| `search.engine` | string | `builtin` | `builtin` or `rg`. |
| `search.exclude` | string array | `["vendor/"]` | Project search exclude. |
| `files.exclude` | string array | `["node_modules/"]` | Added to the workspace ignore list. |
| `files.watch` | bool | `true` | False skips the file watcher. |
| `terminal.shell` | string | `""` | Empty means `$SHELL`, then `/bin/sh`. |
| `terminal.scrollback_lines` | int | `10000` | PTY scrollback. Values ≤ 0 become 10000. |
| `terminal.font_size` | float | `13` | Terminal face size. Values ≤ 0 paint as 13. Card clamp 8–32. |
| `git.gutter` | bool | `true` | False skips gutter diffs. |
| `git.status_debounce_ms` | int | `300` | Status debounce. Values ≤ 0 become 300. |

Keys that are stored and unread by the widgets:

| Key | Type | Default |
|---|---|---|
| `editor.font_family` | string | `Go Mono` |
| `editor.soft_wrap` | bool | `false` |
| `ui.ui_font_size` | float | `13` |
| `intel.semantic_tokens` | bool | `false` |
| `search.max_results` | int | `20000` |

A project value overrides the user value for the same key. Changing the card updates the user file only.

## 13. Agent protocol

Call `harness.Desktop` from the test goroutine. `New` rejects a non-positive size, uses 1 px per dp and 1 px per sp, and starts the clock at 2026-01-01 00:00:00 UTC.

| Rule | What to do |
|---|---|
| Frame | `Step` builds a `layout.Context`, runs `Layout`, then `router.Frame`. Read `WakeupTime` once per step and keep the value. The call clears the flag. |
| Click | `Click(at)` is a primary press and release, then a frame. |
| Drag | `Drag(from, to)` is press, `pointer.Move` with the button held, then release. The router reports the move as `pointer.Drag`. A queued `pointer.Drag` panics. |
| Pointer time | `pointer.Event.Time` is `now.Sub(epoch)` as a `time.Duration`. |
| Keys | `Press` and `Edit` do nothing until `Focus(tag)`. The widget registers `key.FocusFilter`, or the next frame drops focus. |
| Shell keys | `Layout` calls `handleKeys` before `layout`. Tree rows need that op inside `pointer.PassOp`. |
| Screenshot | `Image` needs a surfaceless GPU. Skip on error. Behavior checks do not need a picture. |
| Clock | `Advance` changes the next frame and does not draw one. `treeClickGap` uses the wall clock, so a double-click test belongs in `synctest`. |

Prefer a direct call when the check is text, a caret, or a popup kind: `clickTree`, `openPopup("settings")`, `View.Type`, `View.Undo`, `adjustSetting`. Use pixels for the tool strip, tree rows, Settings buttons, cell hits, and drag selection.

Recipes, each at 1280×800:

**Open the project tool.** `d.Layout` calls `s.layout`. `d.Click(image.Pt(19, 19))`. Expect `leftOpen` and `leftTool == "project"`. Same test: `internal/app/layout_test.go`.

**Expand a directory.** `d.Layout` calls `s.handleKeys` then `s.layout`. After one `Step`, click `image.Pt(90, y)` for y from 30 to 120 step 2 until `openDirs["internal"]` is set. Same test: `internal/app/tree_click_test.go`.

**Open a file.** Set `root`, `bus`, `docs`, and a tree node whose `Path` is the relative file. `s.clickTree("a.go")` then `s.drain()`. The tab is attached when `DocumentOpened` is handled. A pixel open uses the same x=90 probe on a file row.

**Type a letter.** Editor-only: `v.Type("a")`. Through the shell: `d.Focus(s.inputTag())` and `d.Press(key.Name("a"), 0)` with `handleKeys` in the frame, `popup` empty, and focus on the editor.

**Bump font size.** Open Settings, `d.Layout` calls `handleKeys` and `layout`, click around `image.Pt(888, 196)`. Expect `Editor.FontSize == 15`, `shownFont == 15`, and `font_size = 15` in the user file. Same test: `internal/app/settings_panel_test.go`. Direct call: `s.adjustSetting("set:font+")`.

**Capture one keymap stroke.** `adjustSetting("km:edit.undo")`, then deliver one key to the capture. Expect `keymap.toml` to contain that stroke and `edit.undo`. Same test: `internal/app/keymap_ui_test.go`.

Existing tests to extend rather than re-derive: `internal/harness/harness_test.go`, `internal/app/layout_test.go`, `internal/app/tree_click_test.go`, `internal/app/settings_panel_test.go`, `internal/app/keymap_ui_test.go`, `internal/app/docs_test.go`, `internal/app/project_test.go`, `internal/editor/edit_test.go`, `internal/editor/layout_test.go`.

## 14. Smoke list

Run this against a temp git repo that contains two `.go` files, one unstaged edit, and a second branch. Record Pass or the actual text when a step stops.

1. The tree lists the first file. Opening it shows that buffer. The status caret is `1:1` until you click or type.
2. Type a word, undo, and redo. After undo the caret is at the end of the restored edit, on that line.
3. Save. A second save leaves the file bytes unchanged. Change the buffer again, close it, and choose Don't Save. The buffer is gone and the file on disk is the saved copy.
4. Open Find on a buffer with two copies of a word. The first Enter selects the first copy after the caret from when the bar opened. The second Enter keeps that same selection.
5. Go to File opens the other file. Alt+Left shows the first buffer again. With `go` on `PATH` and `intel Ready`, Go to Declaration moves the caret, Find Usages lists `path:line:col`, and Completion inserts the chosen label.
6. The Git panel shows the dirty path. Commit with a message clears the list to `<branch>  clean`. Creating a branch and switching to it changes the status branch cell. A hook script in that repo does not run. The commit prompt tells you to use the terminal for hooks.
7. Alt+F12 shows a shell prompt in the workspace directory. Alt+F12 while that panel is focused hides it.
8. Settings font size plus writes `font_size = 15` into the user file. `view.toggleTheme` flips the painted background between `#1e1f22` and `#ffffff` and leaves a custom `ui.theme` string in the user file unchanged.
9. Find Action filtered to `Rename` shows `Rename  (later)` and `refactor.rename`. Enter logs `Rename is not available in v1`.
10. Go to Line closes on Enter and the caret is unchanged. F2 logs `nav.nextError is ready once a file is open`.
11. Fold an if with Ctrl+-. The body is not on screen. The header shows ` ... }`. Unfold with Ctrl+= brings it back. Up and Down skip hidden lines.
